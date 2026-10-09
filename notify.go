package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

var validChannels = []string{"app", "sms", "whatsapp"}

func cleanChannels(s string) string {
	var out []string
	for _, c := range strings.Split(s, ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		if slices.Contains(validChannels, c) && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

// sender delivers a text message to a phone number over one external channel.
// It returns the final status ("sent" or "simulated").
type sender interface {
	Name() string
	Send(ctx context.Context, to, text string) (string, error)
}

type deliveryJob struct {
	id      int64
	channel string
	to      string
	text    string
}

type Notifier struct {
	a    *App
	jobs chan deliveryJob
	sms  sender
	wa   sender
}

func newNotifier(a *App) *Notifier {
	n := &Notifier{a: a, jobs: make(chan deliveryJob, 2000)}
	hc := &http.Client{Timeout: 20 * time.Second}
	cfg := a.cfg
	switch cfg.SMSProvider {
	case "notifylk":
		n.sms = &notifyLK{hc, cfg}
	case "twilio":
		n.sms = &twilio{hc, cfg, false}
	case "webhook":
		n.sms = &smsWebhook{hc, cfg}
	default:
		n.sms = logSender{"sms"}
	}
	switch cfg.WAProvider {
	case "meta":
		n.wa = &metaWhatsApp{hc, cfg}
	case "twilio":
		n.wa = &twilio{hc, cfg, true}
	default:
		n.wa = logSender{"whatsapp"}
	}
	return n
}

// run starts delivery workers, re-queues anything left over from a previous run and
// returns once the workers have stopped.
func (n *Notifier) run(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-n.jobs:
					n.deliver(ctx, j)
				}
			}
		})
	}
	rows, err := n.a.db.Query(`SELECT id,channel,recipient,message FROM deliveries WHERE status='queued' ORDER BY id`)
	if err != nil {
		log.Printf("requeue: %v", err)
		return
	}
	var pending []deliveryJob
	for rows.Next() {
		var j deliveryJob
		if rows.Scan(&j.id, &j.channel, &j.to, &j.text) == nil {
			pending = append(pending, j)
		}
	}
	rows.Close()
	for _, j := range pending {
		n.enqueue(j)
	}
}

func (n *Notifier) enqueue(j deliveryJob) {
	select {
	case n.jobs <- j:
	default:
		go func() { n.jobs <- j }()
	}
}

func (n *Notifier) senderFor(channel string) sender {
	if channel == "whatsapp" {
		return n.wa
	}
	return n.sms
}

func (n *Notifier) deliver(ctx context.Context, j deliveryJob) {
	status, err := n.senderFor(j.channel).Send(ctx, j.to, j.text)
	errMsg := ""
	if err != nil {
		status, errMsg = "failed", err.Error()
		log.Printf("%s delivery %d to %s failed: %v", j.channel, j.id, j.to, err)
	}
	n.a.db.Exec(`UPDATE deliveries SET status=?, error=?, sent_at=? WHERE id=?`, status, errMsg, now(), j.id)
}

// sendDirect sends one message synchronously (used for provider tests).
func (n *Notifier) sendDirect(ctx context.Context, channel, to, text string) (string, error) {
	if channel != "sms" && channel != "whatsapp" {
		return "", errors.New("channel must be sms or whatsapp")
	}
	if strings.TrimSpace(to) == "" {
		return "", errors.New("phone number is required")
	}
	return n.senderFor(channel).Send(ctx, to, text)
}

// channelsFor returns the configured channels for an automated category.
func (n *Notifier) channelsFor(category string) []string {
	v := cleanChannels(n.a.setting("channels_" + category))
	if v == "" {
		return []string{"app"}
	}
	return strings.Split(v, ",")
}

// Notify sends a message to each user over the requested channels. The in-app
// notification is stored immediately; SMS and WhatsApp are queued for the workers.
// It returns the number of distinct recipients.
func (n *Notifier) Notify(category, title, body string, userIDs []int64, channels []string) (int, error) {
	seen := map[int64]bool{}
	var ids []int64
	for _, id := range userIDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	school := n.a.setting("school_name")
	text := fmt.Sprintf("%s\n%s\n- %s", title, body, school)

	rows, err := n.a.db.Query(`SELECT id, phone, whatsapp FROM users WHERE active=1 AND id IN (`+placeholders(len(ids))+`)`, int64Args(ids)...)
	if err != nil {
		return 0, err
	}
	type rcpt struct {
		id        int64
		phone, wa string
	}
	var list []rcpt
	for rows.Next() {
		var r rcpt
		if err := rows.Scan(&r.id, &r.phone, &r.wa); err == nil {
			list = append(list, r)
		}
	}
	rows.Close()

	ts := now()
	var jobs []deliveryJob
	tx, err := n.a.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, r := range list {
		for _, ch := range channels {
			switch ch {
			case "app":
				if _, err := tx.Exec(`INSERT INTO notifications(user_id,category,title,body,created_at) VALUES(?,?,?,?,?)`, r.id, category, title, body, ts); err != nil {
					return 0, err
				}
				tx.Exec(`INSERT INTO deliveries(user_id,channel,recipient,category,title,message,status,created_at,sent_at) VALUES(?,?,?,?,?,?,?,?,?)`,
					r.id, "app", "in-app", category, title, body, "delivered", ts, ts)
			case "sms", "whatsapp":
				to := r.phone
				if ch == "whatsapp" && r.wa != "" {
					to = r.wa
				}
				if strings.TrimSpace(to) == "" {
					tx.Exec(`INSERT INTO deliveries(user_id,channel,recipient,category,title,message,status,error,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
						r.id, ch, "", category, title, text, "skipped", "no phone number on file", ts)
					continue
				}
				res, err := tx.Exec(`INSERT INTO deliveries(user_id,channel,recipient,category,title,message,status,created_at) VALUES(?,?,?,?,?,?,?,?)`,
					r.id, ch, to, category, title, text, "queued", ts)
				if err != nil {
					return 0, err
				}
				did, _ := res.LastInsertId()
				jobs = append(jobs, deliveryJob{did, ch, to, text})
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, j := range jobs {
		n.enqueue(j)
	}
	return len(list), nil
}

// normalizePhone converts local numbers like 077 123 4567 into international digits (94771234567).
func normalizePhone(p, cc string) string {
	var b strings.Builder
	for _, c := range p {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	d := b.String()
	switch {
	case strings.HasPrefix(strings.TrimSpace(p), "+"):
		return d
	case strings.HasPrefix(d, "00"):
		return d[2:]
	case strings.HasPrefix(d, "0"):
		return cc + d[1:]
	case len(d) <= 9:
		return cc + d
	}
	return d
}

func readErrBody(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}

// ---- Providers ----

type logSender struct{ channel string }

func (l logSender) Name() string { return "simulation (log only)" }
func (l logSender) Send(_ context.Context, to, text string) (string, error) {
	log.Printf("[%s simulated] to %s: %s", strings.ToUpper(l.channel), to, strings.ReplaceAll(text, "\n", " | "))
	return "simulated", nil
}

// notifyLK sends SMS through Notify.lk (Sri Lanka).
type notifyLK struct {
	hc  *http.Client
	cfg Config
}

func (s *notifyLK) Name() string { return "Notify.lk SMS" }
func (s *notifyLK) Send(ctx context.Context, to, text string) (string, error) {
	if s.cfg.NotifyLKUserID == "" || s.cfg.NotifyLKAPIKey == "" {
		return "", errors.New("NOTIFYLK_USER_ID / NOTIFYLK_API_KEY not configured")
	}
	form := url.Values{
		"user_id": {s.cfg.NotifyLKUserID}, "api_key": {s.cfg.NotifyLKAPIKey},
		"sender_id": {s.cfg.NotifyLKSender}, "to": {normalizePhone(to, s.cfg.CountryCode)}, "message": {text},
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://app.notify.lk/api/v1/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	json.Unmarshal(body, &out)
	if resp.StatusCode != 200 || out.Status != "success" {
		return "", fmt.Errorf("notify.lk HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return "sent", nil
}

// twilio sends SMS or WhatsApp messages through Twilio's Messages API.
type twilio struct {
	hc       *http.Client
	cfg      Config
	whatsapp bool
}

func (t *twilio) Name() string {
	if t.whatsapp {
		return "Twilio WhatsApp"
	}
	return "Twilio SMS"
}
func (t *twilio) Send(ctx context.Context, to, text string) (string, error) {
	from := t.cfg.TwilioSMSFrom
	dest := "+" + normalizePhone(to, t.cfg.CountryCode)
	if t.whatsapp {
		from, dest = "whatsapp:"+t.cfg.TwilioWAFrom, "whatsapp:"+dest
	}
	if t.cfg.TwilioSID == "" || t.cfg.TwilioToken == "" || strings.TrimPrefix(from, "whatsapp:") == "" {
		return "", errors.New("Twilio credentials or sender number not configured")
	}
	form := url.Values{"To": {dest}, "From": {from}, "Body": {text}}
	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + url.PathEscape(t.cfg.TwilioSID) + "/Messages.json"
	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	req.SetBasicAuth(t.cfg.TwilioSID, t.cfg.TwilioToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", readErrBody(resp)
	}
	return "sent", nil
}

// smsWebhook posts {"to","message"} JSON to any SMS gateway (Dialog, Mobitel, custom).
type smsWebhook struct {
	hc  *http.Client
	cfg Config
}

func (s *smsWebhook) Name() string { return "SMS webhook gateway" }
func (s *smsWebhook) Send(ctx context.Context, to, text string) (string, error) {
	if s.cfg.SMSWebhookURL == "" {
		return "", errors.New("SMS_WEBHOOK_URL not configured")
	}
	body, _ := json.Marshal(map[string]string{"to": normalizePhone(to, s.cfg.CountryCode), "message": text})
	req, _ := http.NewRequestWithContext(ctx, "POST", s.cfg.SMSWebhookURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.SMSWebhookToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.SMSWebhookToken)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", readErrBody(resp)
	}
	return "sent", nil
}

// metaWhatsApp sends through the WhatsApp Business Cloud API. Outside the 24-hour
// customer-service window WhatsApp requires an approved template; set
// WHATSAPP_TEMPLATE to a template whose body has a single {{1}} parameter.
type metaWhatsApp struct {
	hc  *http.Client
	cfg Config
}

func (m *metaWhatsApp) Name() string { return "WhatsApp Cloud API" }
func (m *metaWhatsApp) Send(ctx context.Context, to, text string) (string, error) {
	if m.cfg.WAToken == "" || m.cfg.WAPhoneID == "" {
		return "", errors.New("WHATSAPP_TOKEN / WHATSAPP_PHONE_NUMBER_ID not configured")
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                normalizePhone(to, m.cfg.CountryCode),
	}
	if m.cfg.WATemplate != "" {
		payload["type"] = "template"
		payload["template"] = map[string]any{
			"name":     m.cfg.WATemplate,
			"language": map[string]string{"code": m.cfg.WATemplateLang},
			"components": []any{map[string]any{
				"type":       "body",
				"parameters": []any{map[string]string{"type": "text", "text": strings.ReplaceAll(text, "\n", " ")}},
			}},
		}
	} else {
		payload["type"] = "text"
		payload["text"] = map[string]any{"body": text}
	}
	body, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", m.cfg.WAAPIVersion, url.PathEscape(m.cfg.WAPhoneID))
	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.cfg.WAToken)
	resp, err := m.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", readErrBody(resp)
	}
	return "sent", nil
}
