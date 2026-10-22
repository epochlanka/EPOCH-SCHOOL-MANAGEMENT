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
	"os"
	"strings"
	"sync"
	"time"
)

// AI wraps the Google Gemini API (generateContent) for message drafting, translation,
// reply suggestions and a data-aware school assistant.
type AI struct {
	a       *App
	hc      *http.Client
	apiKey  string
	model   string
	enabled bool
	usage   *usageLimiter

	mu    sync.Mutex
	convs map[string]*conversation
}

type conversation struct {
	userID   int64
	contents []json.RawMessage // Gemini contents, kept exactly as sent/received (model turns carry thought signatures)
	updated  time.Time
}

var geminiBase = "https://generativelanguage.googleapis.com/v1beta/models/" // a variable so tests can point it at a fake server

func newAI(a *App) *AI {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		key = os.Getenv("GOOGLE_API_KEY")
	}
	return &AI{
		a:       a,
		hc:      &http.Client{Timeout: 5 * time.Minute},
		apiKey:  key,
		model:   a.cfg.AIModel,
		enabled: key != "",
		usage:   newUsageLimiter(),
		convs:   map[string]*conversation{},
	}
}

func (ai *AI) statusText() string {
	if ai.enabled {
		return "Google Gemini (" + ai.model + ")"
	}
	return "disabled (set GEMINI_API_KEY)"
}

var (
	errAIDisabled = errors.New("AI is not configured. Add GEMINI_API_KEY to the server's .env file and restart.")
	errAIRefused  = errors.New("the AI declined this request; please rephrase it")
	errAIBusy     = errors.New("You have used Epoch AI a lot in the last hour. Please try again later.")
)

// ---------- Usage limit ----------

// usageLimiter caps AI requests per user per hour, so a single account cannot run up the API bill.
type usageLimiter struct {
	mu   sync.Mutex
	hits map[int64][]time.Time
}

func newUsageLimiter() *usageLimiter { return &usageLimiter{hits: map[int64][]time.Time{}} }

func (l *usageLimiter) allow(userID int64, perHour int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-time.Hour)
	recent := l.hits[userID][:0]
	for _, t := range l.hits[userID] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= perHour {
		l.hits[userID] = recent
		return false
	}
	l.hits[userID] = append(recent, time.Now())
	return true
}

// allow reports whether u may make another AI request: staff 300 an hour, parents and students 40.
func (ai *AI) allow(u *User) bool {
	limit := 40
	if isStaff(u.Role) {
		limit = 300
	}
	return ai.usage.allow(u.ID, limit)
}

// ---------- Gemini REST types ----------

type gPart struct {
	Text             string             `json:"text,omitempty"`
	Thought          bool               `json:"thought,omitempty"`
	FunctionCall     *gFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *gFunctionResponse `json:"functionResponse,omitempty"`
}

type gFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type gFunctionResponse struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Response any    `json:"response"`
}

type gContent struct {
	Role  string  `json:"role"`
	Parts []gPart `json:"parts"`
}

type gFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type gResponse struct {
	Candidates []struct {
		Content      json.RawMessage `json:"content"`
		FinishReason string          `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

func userContent(parts ...gPart) json.RawMessage {
	b, _ := json.Marshal(gContent{Role: "user", Parts: parts})
	return b
}

// generate sends one generateContent request and returns the model's turn, both raw
// (to keep in the history unchanged) and decoded.
func (ai *AI) generate(ctx context.Context, system string, contents []json.RawMessage, tools []gFuncDecl, schema map[string]any) (json.RawMessage, gContent, error) {
	if !ai.enabled {
		return nil, gContent{}, errAIDisabled
	}
	body := map[string]any{
		"systemInstruction": map[string]any{"parts": []gPart{{Text: system}}},
		"contents":          contents,
	}
	if len(tools) > 0 {
		body["tools"] = []map[string]any{{"functionDeclarations": tools}}
	}
	if schema != nil {
		body["generationConfig"] = map[string]any{"responseMimeType": "application/json", "responseSchema": schema}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, gContent{}, err
	}
	endpoint := geminiBase + url.PathEscape(ai.model) + ":generateContent"

	var raw []byte
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", ai.apiKey)
		resp, err := ai.hc.Do(req)
		if err != nil {
			log.Printf("gemini error: %v", err)
			return nil, gContent{}, errors.New("could not reach the AI service")
		}
		raw, _ = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			break
		}
		retryable := resp.StatusCode == 429 || resp.StatusCode == 500 || resp.StatusCode == 503
		if retryable && attempt < 2 {
			select {
			case <-ctx.Done():
				return nil, gContent{}, errors.New("the AI service took too long; please try again")
			case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
			}
			continue
		}
		return nil, gContent{}, geminiError(resp.StatusCode, raw)
	}

	var out gResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, gContent{}, fmt.Errorf("unexpected reply from the AI service: %w", err)
	}
	if out.PromptFeedback.BlockReason != "" || len(out.Candidates) == 0 {
		return nil, gContent{}, errAIRefused
	}
	cand := out.Candidates[0]
	switch cand.FinishReason {
	case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII", "RECITATION":
		return nil, gContent{}, errAIRefused
	}
	var content gContent
	if len(cand.Content) > 0 {
		json.Unmarshal(cand.Content, &content)
	}
	if content.Role == "" {
		content.Role = "model"
	}
	rawContent := cand.Content
	if len(rawContent) == 0 {
		rawContent, _ = json.Marshal(content)
	}
	return rawContent, content, nil
}

// geminiError turns an API error into a message staff can act on.
func geminiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	json.Unmarshal(body, &e)
	log.Printf("gemini api error %d %s: %s", status, e.Error.Status, e.Error.Message)
	msg := strings.ToLower(e.Error.Message)
	switch {
	case status == 401 || status == 403 || strings.Contains(msg, "api key"):
		return errors.New("the AI service rejected the API key; check GEMINI_API_KEY")
	case status == 404:
		return errors.New("the AI model was not found; check AI_MODEL")
	case status == 429:
		return errors.New("the AI service is busy or the quota is used up; please try again in a moment")
	case status >= 500:
		return errors.New("the AI service is temporarily unavailable; please try again")
	}
	return fmt.Errorf("AI request failed (HTTP %d)", status)
}

// textOf joins the visible text of a model turn, leaving out thought summaries.
func textOf(c gContent) string {
	var sb strings.Builder
	for _, p := range c.Parts {
		if !p.Thought {
			sb.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

// complete runs a single prompt. With a schema the reply is JSON matching it.
func (ai *AI) complete(ctx context.Context, system, prompt string, schema map[string]any) (string, error) {
	_, content, err := ai.generate(ctx, system, []json.RawMessage{userContent(gPart{Text: prompt})}, nil, schema)
	if err != nil {
		return "", err
	}
	return textOf(content), nil
}

func aiFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAIDisabled):
		errJSON(w, 503, err.Error())
	case errors.Is(err, errAIBusy):
		errJSON(w, 429, err.Error())
	case errors.Is(err, errAIRefused):
		errJSON(w, 422, err.Error())
	default:
		errJSON(w, 502, err.Error())
	}
}

func (a *App) schoolContext() string {
	return fmt.Sprintf("School: %s, %s, Sri Lanka. Phone: %s. Today is %s. Currency: Sri Lankan Rupees (Rs.).",
		a.setting("school_name"), a.setting("school_address"), a.setting("school_phone"), time.Now().Format("Monday, 02 January 2006"))
}

func (a *App) handleAIStatus(w http.ResponseWriter, r *http.Request, u *User) {
	writeJSON(w, 200, map[string]any{"enabled": a.ai.enabled, "model": a.ai.model})
}

// ---------- Compose ----------

var composeSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":   map[string]any{"type": "string", "description": "Short notification title, under 60 characters"},
		"message": map[string]any{"type": "string", "description": "Full message for the app notification"},
	},
	"required": []string{"title", "message"},
}

func (a *App) handleAICompose(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Prompt   string `json:"prompt"`
		Audience string `json:"audience"`
		Tone     string `json:"tone"`
		Language string `json:"language"`
		Draft    string `json:"draft"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" && strings.TrimSpace(req.Draft) == "" {
		errJSON(w, 400, "Describe what the message should say")
		return
	}
	if !a.ai.allow(u) {
		aiFail(w, errAIBusy)
		return
	}
	if req.Language == "" {
		req.Language = "English"
	}
	if req.Tone == "" {
		req.Tone = "warm and professional"
	}
	system := `You write official communications from a school to parents, students and staff. ` + a.schoolContext() + `
Write clear, respectful, concise messages suitable for a mobile app notification.
Address parents as "Dear Parent" (or the local-language equivalent) unless the audience is staff or students.
Never invent specific facts such as dates, times, amounts or names that the request does not give; if one is needed and missing, insert a clear placeholder like [DATE].
Sign off with the school name. Write every field in the requested language, using its native script.`
	prompt := fmt.Sprintf("Audience: %s\nTone: %s\nLanguage: %s\n\nRequest: %s", req.Audience, req.Tone, req.Language, req.Prompt)
	if strings.TrimSpace(req.Draft) != "" {
		prompt += "\n\nImprove this existing draft:\n" + req.Draft
	}
	text, err := a.ai.complete(r.Context(), system, prompt, composeSchema)
	if err != nil {
		aiFail(w, err)
		return
	}
	var out struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		out.Title, out.Message = "Announcement", text
	}
	writeJSON(w, 200, out)
}

// ---------- Translate ----------

func (a *App) handleAITranslate(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" || len(req.Text) > 8000 {
		errJSON(w, 400, "Text is required (max 8000 characters)")
		return
	}
	switch req.Language {
	case "English", "Sinhala", "Tamil":
	default:
		errJSON(w, 400, "Language must be English, Sinhala or Tamil")
		return
	}
	if !a.ai.allow(u) {
		aiFail(w, errAIBusy)
		return
	}
	text, err := a.ai.translate(r.Context(), req.Text, req.Language)
	if err != nil {
		aiFail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"text": text})
}

// translate renders school text in English, Sinhala or Tamil.
func (ai *AI) translate(ctx context.Context, text, lang string) (string, error) {
	system := "You translate school communications for Sri Lankan families. Return only the translation in the target language's native script, preserving names, numbers, dates and formatting. Do not add commentary."
	return ai.complete(ctx, system, "Translate into "+lang+":\n\n"+text, nil)
}

// ---------- Reply suggestion ----------

func (a *App) handleAIReply(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		UserID int64  `json:"user_id"`
		Hint   string `json:"hint"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	var otherName, otherRole string
	if err := a.db.QueryRow(`SELECT name,role FROM users WHERE id=?`, req.UserID).Scan(&otherName, &otherRole); err != nil || !canMessage(u.Role, otherRole) {
		errJSON(w, 404, "Conversation not found")
		return
	}
	if !a.ai.allow(u) {
		aiFail(w, errAIBusy)
		return
	}
	msgs, err := a.threadMessages(u.ID, req.UserID, 20)
	if err != nil {
		serverError(w, err)
		return
	}
	var sb strings.Builder
	for _, m := range msgs {
		who := otherName
		if m["sender_id"].(int64) == u.ID {
			who = u.Name + " (me)"
		}
		fmt.Fprintf(&sb, "%s: %s\n", who, m["body"])
	}
	if sb.Len() == 0 {
		sb.WriteString("(no messages yet)\n")
	}
	system := fmt.Sprintf(`You help %s (a %s at the school) reply to messages. %s
Draft one reply the user can send as-is: polite, helpful, and concise. Do not promise anything the conversation does not support. Reply in the same language the other person used. Return only the reply text.`, u.Name, u.Role, a.schoolContext())
	prompt := fmt.Sprintf("Conversation with %s (%s):\n%s", otherName, otherRole, sb.String())
	if strings.TrimSpace(req.Hint) != "" {
		prompt += "\nWhat I want to say: " + req.Hint
	}
	text, err := a.ai.complete(r.Context(), system, prompt, nil)
	if err != nil {
		aiFail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"text": text})
}

// ---------- Assistant with tools ----------

// aiTool is a function the assistant may call. Family tools are offered to parents and
// students; staff tools to staff with the ai.data permission, and only when they also
// hold perm (if set). Data is limited to the classes the user works with.
type aiTool struct {
	def    gFuncDecl
	family bool
	staff  bool
	perm   string
	run    func(u *User, input json.RawMessage) (any, error)
}

func (t aiTool) availableTo(u *User) bool {
	if u.Role == "parent" || u.Role == "student" {
		return t.family
	}
	return t.staff && hasPerm(u.Role, PAIData) && (t.perm == "" || hasPerm(u.Role, t.perm))
}

func tool(name, desc string, props map[string]any, required []string) gFuncDecl {
	d := gFuncDecl{Name: name, Description: desc}
	if len(props) > 0 {
		d.Parameters = map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			d.Parameters["required"] = required
		}
	}
	return d
}

func (a *App) assistantTools() []aiTool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

	return []aiTool{
		{
			def:   tool("get_school_overview", "Headline numbers: student/teacher/parent counts, today's attendance, outstanding and overdue fees, messages sent today.", nil, nil),
			staff: true, perm: PDashboard,
			run: func(u *User, _ json.RawMessage) (any, error) {
				t := today()
				return map[string]any{
					"students":            a.scalar(`SELECT COUNT(*) FROM students`),
					"teaching_staff":      a.scalar(`SELECT COUNT(*) FROM users WHERE role IN ` + teachingRolesSQL + ` AND active=1`),
					"teachers_on_leave":   len(a.staffOnLeave(t)),
					"teachers_absent":     a.scalar(`SELECT COUNT(*) FROM staff_attendance WHERE date=? AND status='absent'`, t),
					"pending_leave":       a.scalar(`SELECT COUNT(*) FROM leave_requests WHERE status='pending'`),
					"relief_periods":      a.scalar(`SELECT COUNT(*) FROM substitutions WHERE date=? AND status='assigned'`, t),
					"uncovered_periods":   a.scalar(`SELECT COUNT(*) FROM substitutions WHERE date=? AND status='unfilled'`, t),
					"parents":             a.scalar(`SELECT COUNT(*) FROM users WHERE role='parent' AND active=1`),
					"present_today":       a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status='present'`, t),
					"absent_today":        a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status='absent'`, t),
					"late_today":          a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status='late'`, t),
					"outstanding_fees":    a.scalar(`SELECT COALESCE(SUM(amount),0) FROM fees WHERE status='unpaid'`),
					"overdue_fees":        a.scalar(`SELECT COALESCE(SUM(amount),0) FROM fees WHERE status='unpaid' AND due_date<?`, t),
					"messages_sent_today": a.scalar(`SELECT COUNT(*) FROM deliveries WHERE substr(created_at,1,10)=?`, t),
				}, nil
			},
		},
		{
			def: tool("get_attendance", "Attendance summary per class for a date, plus the list of absent and late students. Covers only the classes the user works with.",
				map[string]any{"date": str("Date as YYYY-MM-DD; defaults to today")}, nil),
			staff: true, perm: PStudentsView,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct{ Date string }
				json.Unmarshal(in, &p)
				if !validDate(p.Date) {
					p.Date = today()
				}
				classScope, classArgs := a.scopeSQL(u, PStudentsView, "c.id")
				classes, err := a.queryMaps(`SELECT c.name||' - '||c.section AS class, COUNT(s.id) AS students,
					COUNT(a.id) FILTER (WHERE a.status='present') AS present, COUNT(a.id) FILTER (WHERE a.status='absent') AS absent, COUNT(a.id) FILTER (WHERE a.status='late') AS late
					FROM classes c LEFT JOIN students s ON s.class_id=c.id LEFT JOIN attendance a ON a.student_id=s.id AND a.date=?
					WHERE 1=1`+classScope+` GROUP BY c.id ORDER BY c.name`, append([]any{p.Date}, classArgs...)...)
				if err != nil {
					return nil, err
				}
				studentScope, studentArgs := a.scopeSQL(u, PStudentsView, "s.class_id")
				missing, _ := a.queryMaps(`SELECT s.id AS student_id, s.name, c.name||' - '||c.section AS class, a.status
					FROM attendance a JOIN students s ON s.id=a.student_id LEFT JOIN classes c ON c.id=s.class_id
					WHERE a.date=? AND a.status IN ('absent','late')`+studentScope, append([]any{p.Date}, studentArgs...)...)
				return map[string]any{"date": p.Date, "classes": classes, "absent_or_late": missing}, nil
			},
		},
		{
			def: tool("find_students", "Search students by name, admission number or parent name. Returns class, parent contact, attendance % and (for fee staff) fee balance. Covers only the classes the user works with.",
				map[string]any{"query": str("Name or admission number to search for")}, []string{"query"}),
			staff: true, perm: PStudentsView,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct{ Query string }
				json.Unmarshal(in, &p)
				q := "%" + p.Query + "%"
				fees := ""
				if hasPerm(u.Role, PFeesView) {
					fees = `, (SELECT COALESCE(SUM(amount),0) FROM fees WHERE student_id=s.id AND status='unpaid') AS fee_balance`
				}
				scope, args := a.scopeSQL(u, PStudentsView, "s.class_id")
				return a.queryMaps(`SELECT s.id, s.name, s.admission_no, c.name||' - '||c.section AS class, p.name AS parent, p.phone AS parent_phone,
					(SELECT ROUND(100.0*COUNT(*) FILTER (WHERE status<>'absent')/GREATEST(COUNT(*),1),1)::float8 FROM attendance WHERE student_id=s.id) AS attendance_pct`+fees+`
					FROM students s LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users p ON p.id=s.parent_id
					WHERE (s.name ILIKE ? OR s.admission_no ILIKE ? OR p.name ILIKE ?)`+scope+` LIMIT 20`, append([]any{q, q, q}, args...)...)
			},
		},
		{
			def: tool("get_student_report", "Full report for one student: profile, attendance history and stats, fees, published exam results and upcoming homework/exams.",
				map[string]any{"student_id": num("The student's id")}, []string{"student_id"}),
			staff: true, family: true,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct {
					StudentID int64 `json:"student_id"`
				}
				json.Unmarshal(in, &p)
				if !a.canViewStudent(u, p.StudentID) {
					return nil, errors.New("not permitted: this student is not linked to your account or classes")
				}
				return a.studentReport(p.StudentID)
			},
		},
		{
			def:    tool("list_my_children", "List the students linked to the signed-in parent or student account, with their ids.", nil, nil),
			family: true,
			run: func(u *User, _ json.RawMessage) (any, error) {
				return a.queryMaps(`SELECT s.id, s.name, c.name||' - '||c.section AS class FROM students s LEFT JOIN classes c ON c.id=s.class_id
					WHERE s.parent_id=? OR s.user_id=?`, u.ID, u.ID)
			},
		},
		{
			def: tool("get_unpaid_fees", "Unpaid fees with student, class, parent and due date.",
				map[string]any{"overdue_only": map[string]any{"type": "boolean", "description": "Only fees past their due date"}}, nil),
			staff: true, perm: PFeesView,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct {
					OverdueOnly bool `json:"overdue_only"`
				}
				json.Unmarshal(in, &p)
				cutoff := "9999-12-31"
				if p.OverdueOnly {
					cutoff = today()
				}
				return a.queryMaps(`SELECT s.name AS student, c.name||' - '||c.section AS class, p.name AS parent, f.title, f.amount, f.due_date
					FROM fees f JOIN students s ON s.id=f.student_id LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users p ON p.id=s.parent_id
					WHERE f.status='unpaid' AND f.due_date < ? ORDER BY f.due_date LIMIT 200`, cutoff)
			},
		},
		{
			def:   tool("get_exam_performance", "Exam list with class averages, highest and lowest marks, for the classes the user works with.", nil, nil),
			staff: true, perm: PAcademic,
			run: func(u *User, _ json.RawMessage) (any, error) {
				scope, args := a.scopeSQL(u, PAcademic, "e.class_id")
				return a.queryMaps(`SELECT e.id, e.subject, e.title, e.exam_date, e.max_marks, c.name||' - '||c.section AS class,
					COUNT(x.id) AS results, ROUND(AVG(x.marks)::numeric,1)::float8 AS average, MAX(x.marks) AS highest, MIN(x.marks) AS lowest
					FROM exams e JOIN classes c ON c.id=e.class_id LEFT JOIN results x ON x.exam_id=e.id
					WHERE 1=1`+scope+` GROUP BY e.id, c.id ORDER BY e.exam_date DESC LIMIT 50`, args...)
			},
		},
		{
			def: tool("get_staff_today", "Teacher attendance for a date: who is present, late, absent or on leave, pending leave requests, and relief (substitute) teacher assignments.",
				map[string]any{"date": str("Date as YYYY-MM-DD; defaults to today")}, nil),
			staff: true, perm: PStaffView,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct{ Date string }
				json.Unmarshal(in, &p)
				if !validDate(p.Date) {
					p.Date = today()
				}
				att, err := a.queryMaps(`SELECT u.name, u.role, COALESCE(sa.status,'not marked') AS status, COALESCE(sa.check_in,'') AS check_in
					FROM users u LEFT JOIN staff_attendance sa ON sa.user_id=u.id AND sa.date=? WHERE u.active=1 AND u.role IN `+staffRolesSQL, p.Date)
				if err != nil {
					return nil, err
				}
				leave, _ := a.queryMaps(`SELECT u.name, l.leave_type, l.from_date, l.to_date, l.status, l.reason FROM leave_requests l JOIN users u ON u.id=l.user_id
					WHERE l.status='pending' OR (l.status='approved' AND ? BETWEEN l.from_date AND l.to_date)`, p.Date)
				subs, _ := a.queryMaps(`SELECT x.period, c.name||' - '||c.section AS class, s.name AS subject, ab.name AS absent_teacher, sb.name AS relief_teacher, x.status
					FROM substitutions x JOIN classes c ON c.id=x.class_id LEFT JOIN subjects s ON s.id=x.subject_id LEFT JOIN users ab ON ab.id=x.absent_teacher_id
					LEFT JOIN users sb ON sb.id=x.substitute_id WHERE x.date=? ORDER BY x.period`, p.Date)
				return map[string]any{"date": p.Date, "attendance": att, "leave": leave, "substitutions": subs}, nil
			},
		},
		{
			def: tool("get_timetable", "Weekly timetable for a class (by class name, e.g. 'Grade 8 A') or a teacher (by name). Day 1 is Monday.",
				map[string]any{"class_name": str("Class name"), "teacher_name": str("Teacher name")}, nil),
			staff: true,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct {
					ClassName   string `json:"class_name"`
					TeacherName string `json:"teacher_name"`
				}
				json.Unmarshal(in, &p)
				q := `SELECT t.day, t.period, c.name||' '||c.section AS class, s.name AS subject, u.name AS teacher FROM timetable t
					JOIN classes c ON c.id=t.class_id JOIN subjects s ON s.id=t.subject_id LEFT JOIN users u ON u.id=t.teacher_id WHERE `
				var rows []map[string]any
				var err error
				if p.TeacherName != "" {
					rows, err = a.queryMaps(q+`u.name ILIKE ? ORDER BY t.day, t.period`, "%"+p.TeacherName+"%")
				} else {
					rows, err = a.queryMaps(q+`(c.name||' '||c.section) ILIKE ? ORDER BY t.day, t.period`, "%"+strings.ReplaceAll(p.ClassName, "-", "")+"%")
				}
				return map[string]any{"period_times": a.timetableConfig().Times, "entries": rows}, err
			},
		},
		{
			def: tool("get_communication_stats", "Messages sent by channel, status and category over the last N days.",
				map[string]any{"days": num("Number of days to look back (default 7)")}, nil),
			staff: true, perm: PReports,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct{ Days int }
				json.Unmarshal(in, &p)
				if p.Days <= 0 || p.Days > 365 {
					p.Days = 7
				}
				since := time.Now().AddDate(0, 0, -p.Days).Format(tsLayout)
				return a.queryMaps(`SELECT channel, category, status, COUNT(*) AS count FROM deliveries WHERE created_at>=? GROUP BY channel, category, status`, since)
			},
		},
	}
}

func (a *App) studentReport(id int64) (map[string]any, error) {
	student, err := a.queryOne(`SELECT s.id,s.name,s.admission_no,c.name||' - '||c.section AS class, t.name AS class_teacher, r.name AS bus_route, r.bus_no
		FROM students s LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users t ON t.id=c.teacher_id LEFT JOIN routes r ON r.id=s.route_id WHERE s.id=?`, id)
	if err != nil {
		return nil, errors.New("student not found")
	}
	attendance, _ := a.queryMaps(`SELECT date,status FROM attendance WHERE student_id=? ORDER BY date DESC LIMIT 30`, id)
	fees, _ := a.queryMaps(`SELECT title,amount,due_date,status FROM fees WHERE student_id=?`, id)
	results, _ := a.queryMaps(`SELECT e.subject,e.title,e.exam_date,e.max_marks,x.marks,x.remarks,
		(SELECT ROUND(AVG(marks)::numeric,1)::float8 FROM results y WHERE y.exam_id=e.id) AS class_average
		FROM results x JOIN exams e ON e.id=x.exam_id WHERE x.student_id=? AND e.published=1`, id)
	homework, _ := a.queryMaps(`SELECT subject,title,due_date FROM homework WHERE class_id=(SELECT class_id FROM students WHERE id=?) AND due_date>=?`, id, today())
	exams, _ := a.queryMaps(`SELECT subject,title,exam_date,exam_time FROM exams WHERE class_id=(SELECT class_id FROM students WHERE id=?) AND exam_date>=?`, id, today())
	return map[string]any{"student": student, "recent_attendance": attendance, "fees": fees, "results": results,
		"upcoming_homework": homework, "upcoming_exams": exams}, nil
}

func (ai *AI) getConversation(id string, userID int64) *conversation {
	ai.mu.Lock()
	defer ai.mu.Unlock()
	for k, c := range ai.convs { // drop idle conversations
		if time.Since(c.updated) > 2*time.Hour {
			delete(ai.convs, k)
		}
	}
	c, ok := ai.convs[id]
	if !ok || c.userID != userID {
		return nil
	}
	return c
}

func (a *App) handleAIChat(w http.ResponseWriter, r *http.Request, u *User) {
	if !a.ai.enabled {
		aiFail(w, errAIDisabled)
		return
	}
	var req struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || len(req.Message) > 4000 {
		errJSON(w, 400, "Message must be between 1 and 4000 characters")
		return
	}
	if !a.ai.allow(u) {
		aiFail(w, errAIBusy)
		return
	}

	var tools []aiTool
	var decls []gFuncDecl
	for _, t := range a.assistantTools() {
		if t.availableTo(u) {
			tools = append(tools, t)
			decls = append(decls, t.def)
		}
	}

	conv := a.ai.getConversation(req.ConversationID, u.ID)
	if conv == nil {
		req.ConversationID = newToken()[:16]
		conv = &conversation{userID: u.ID}
	}
	// Work on a copy so a failed request leaves the stored history untouched (append-only).
	contents := append([]json.RawMessage{}, conv.contents...)
	contents = append(contents, userContent(gPart{Text: req.Message}))

	var persona string
	if hasPerm(u.Role, PAIData) {
		persona = "You are Epoch AI, the assistant inside the school's management system, helping school staff. Use the tools to look up live school data before answering questions about students, attendance, fees, exams or communications. The tools only return data this user is allowed to see; if something is not available, say so. Give concrete, actionable answers; when useful, suggest what message to send and to whom. Format answers with short paragraphs and bullet lists (Markdown)."
	} else if isStaff(u.Role) {
		persona = "You are Epoch AI, the assistant inside the school's management system. You help " + roleLabel(u.Role) + " staff draft messages, plan work and answer general questions. You do not have access to school records in this conversation. Format answers with short paragraphs and bullet lists (Markdown)."
	} else {
		persona = "You are Epoch AI, a helpful assistant for parents and students of the school. You can only access information about the signed-in user's own children through the tools. Use list_my_children first to find student ids. Explain attendance, fees, homework, exams and results kindly and clearly. If asked about other students, politely decline. Format answers with short paragraphs and bullet lists (Markdown)."
	}
	system := persona + "\n" + a.schoolContext() + fmt.Sprintf("\nSigned-in user: %s (%s). Reply in the language the user writes in (English, Sinhala or Tamil).", u.Name, roleLabel(u.Role))

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	var answer string
	for step := 0; step < 8; step++ {
		raw, content, err := a.ai.generate(ctx, system, contents, decls, nil)
		if err != nil {
			aiFail(w, err)
			return
		}
		contents = append(contents, raw)
		if text := textOf(content); text != "" {
			answer = text
		}

		var results []gPart
		for _, part := range content.Parts {
			call := part.FunctionCall
			if call == nil {
				continue
			}
			var out any
			var runErr error = fmt.Errorf("unknown tool %s", call.Name)
			for _, t := range tools {
				if t.def.Name == call.Name {
					out, runErr = t.run(u, call.Args)
					break
				}
			}
			resp := map[string]any{"result": out}
			if runErr != nil {
				resp = map[string]any{"error": runErr.Error()}
			}
			results = append(results, gPart{FunctionResponse: &gFunctionResponse{ID: call.ID, Name: call.Name, Response: resp}})
		}
		if len(results) == 0 {
			break
		}
		contents = append(contents, userContent(results...))
	}
	if answer == "" {
		answer = "I couldn't complete that request. Please try asking in a different way."
	}

	a.ai.mu.Lock()
	conv.contents = contents
	conv.updated = time.Now()
	a.ai.convs[req.ConversationID] = conv
	a.ai.mu.Unlock()
	writeJSON(w, 200, map[string]string{"conversation_id": req.ConversationID, "answer": answer})
}
