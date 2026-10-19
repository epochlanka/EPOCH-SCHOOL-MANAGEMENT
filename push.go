package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type pushJob struct {
	userID int64
	title  string
	body   string
}

type pushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

func (a *App) pushEnabled() bool {
	return a.cfg.PushPublicKey != "" && a.cfg.PushPrivateKey != ""
}

func (a *App) handlePushPublicKey(w http.ResponseWriter, _ *http.Request, _ *User) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    a.pushEnabled(),
		"public_key": a.cfg.PushPublicKey,
	})
}

func (a *App) handleSavePushSubscription(w http.ResponseWriter, r *http.Request, u *User) {
	var req pushSubscriptionRequest
	if err := readJSON(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.pushEnabled() {
		errJSON(w, http.StatusServiceUnavailable, "Phone notifications are not configured on this server")
		return
	}
	if !validPushEndpoint(req.Endpoint) || strings.TrimSpace(req.Keys.P256dh) == "" || strings.TrimSpace(req.Keys.Auth) == "" {
		errJSON(w, http.StatusBadRequest, "Invalid push subscription")
		return
	}
	_, err := a.db.Exec(`INSERT INTO push_subscriptions(user_id,endpoint,p256dh,auth,created_at)
		VALUES(?,?,?,?,?) ON CONFLICT(user_id,endpoint) DO UPDATE SET p256dh=excluded.p256dh,auth=excluded.auth,created_at=excluded.created_at`,
		u.ID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth, now())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleDeletePushSubscription(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Endpoint == "" {
		errJSON(w, http.StatusBadRequest, "Push endpoint is required")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM push_subscriptions WHERE user_id=? AND endpoint=?`, u.ID, req.Endpoint); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Accept only known browser push service hosts so a forged subscription cannot
// make the school server send requests to arbitrary internal or private URLs.
func validPushEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "fcm.googleapis.com" ||
		host == "web.push.apple.com" || host == "api.push.apple.com" ||
		host == "notify.windows.com" || strings.HasSuffix(host, ".notify.windows.com") ||
		host == "push.services.mozilla.com" || strings.HasSuffix(host, ".push.services.mozilla.com")
}

func (n *Notifier) enqueuePush(j pushJob) {
	if !n.a.pushEnabled() {
		return
	}
	select {
	case n.pushJobs <- j:
	default:
		go func() { n.pushJobs <- j }()
	}
}

func (n *Notifier) deliverPush(j pushJob) {
	rows, err := n.a.db.Query(`SELECT endpoint,p256dh,auth FROM push_subscriptions WHERE user_id=?`, j.userID)
	if err != nil {
		log.Printf("web push subscriptions for user %d: %v", j.userID, err)
		return
	}
	var subs []webpush.Subscription
	for rows.Next() {
		var sub webpush.Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.Keys.P256dh, &sub.Keys.Auth); err == nil {
			subs = append(subs, sub)
		}
	}
	rows.Close()

	payload, _ := json.Marshal(pushPayload{Title: j.title, Body: j.body, URL: "/"})
	for _, sub := range subs {
		if !validPushEndpoint(sub.Endpoint) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &sub, &webpush.Options{
			Subscriber:      n.a.cfg.PushSubject,
			VAPIDPublicKey:  n.a.cfg.PushPublicKey,
			VAPIDPrivateKey: n.a.cfg.PushPrivateKey,
			TTL:             60 * 60,
			HTTPClient:      &http.Client{Timeout: 15 * time.Second},
		})
		cancel()
		if err != nil {
			log.Printf("web push to user %d failed: %v", j.userID, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			if _, err := n.a.db.Exec(`DELETE FROM push_subscriptions WHERE user_id=? AND endpoint=?`, j.userID, sub.Endpoint); err != nil {
				log.Printf("remove expired web push subscription: %v", err)
			}
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			log.Printf("web push to user %d returned HTTP %d", j.userID, resp.StatusCode)
		}
	}
}
