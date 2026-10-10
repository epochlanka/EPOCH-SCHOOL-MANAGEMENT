package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AI wraps the Claude API for message drafting, translation, reply suggestions
// and a data-aware school assistant.
type AI struct {
	a       *App
	client  anthropic.Client
	model   string
	enabled bool

	mu    sync.Mutex
	convs map[string]*conversation
}

type conversation struct {
	userID   int64
	messages []anthropic.MessageParam
	updated  time.Time
}

func newAI(a *App) *AI {
	enabled := os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != ""
	return &AI{
		a:       a,
		client:  anthropic.NewClient(option.WithMaxRetries(2), option.WithRequestTimeout(3*time.Minute)),
		model:   a.cfg.AIModel,
		enabled: enabled,
		convs:   map[string]*conversation{},
	}
}

func (ai *AI) statusText() string {
	if ai.enabled {
		return "Claude (" + ai.model + ")"
	}
	return "disabled (set ANTHROPIC_API_KEY)"
}

var errAIDisabled = errors.New("AI is not configured. Add ANTHROPIC_API_KEY to the server's .env file and restart.")

// requestOptions opts into server-side refusal fallbacks and sets the effort level.
func requestOptions(effort string, format map[string]any) []option.RequestOption {
	outputConfig := map[string]any{"effort": effort}
	if format != nil {
		outputConfig["format"] = format
	}
	return []option.RequestOption{
		option.WithHeaderAdd("anthropic-beta", "server-side-fallback-2026-07-01"),
		option.WithJSONSet("fallbacks", "default"),
		option.WithJSONSet("output_config", outputConfig),
	}
}

func responseText(resp *anthropic.Message) (string, error) {
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", errors.New("the AI declined this request; please rephrase it")
	}
	var sb strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func (ai *AI) complete(ctx context.Context, system, prompt, effort string, format map[string]any) (string, error) {
	if !ai.enabled {
		return "", errAIDisabled
	}
	resp, err := ai.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(ai.model),
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
	}, requestOptions(effort, format)...)
	if err != nil {
		return "", aiError(err)
	}
	return responseText(resp)
}

func aiError(err error) error {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		log.Printf("claude api error %d: %v", apierr.StatusCode, err)
		switch apierr.StatusCode {
		case 401, 403:
			return errors.New("the AI service rejected the API key; check ANTHROPIC_API_KEY")
		case 429:
			return errors.New("the AI service is busy (rate limited); please try again in a moment")
		case 529, 500, 502, 503:
			return errors.New("the AI service is temporarily unavailable; please try again")
		}
		return fmt.Errorf("AI request failed (HTTP %d)", apierr.StatusCode)
	}
	log.Printf("claude error: %v", err)
	return errors.New("could not reach the AI service")
}

func aiFail(w http.ResponseWriter, err error) {
	if errors.Is(err, errAIDisabled) {
		errJSON(w, 503, err.Error())
		return
	}
	errJSON(w, 502, err.Error())
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
	"type": "json_schema",
	"schema": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":   map[string]any{"type": "string", "description": "Short notification title, under 60 characters"},
			"message": map[string]any{"type": "string", "description": "Full message for app and WhatsApp"},
			"sms":     map[string]any{"type": "string", "description": "Condensed SMS version, under 300 characters"},
		},
		"required":             []string{"title", "message", "sms"},
		"additionalProperties": false,
	},
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
	if req.Language == "" {
		req.Language = "English"
	}
	if req.Tone == "" {
		req.Tone = "warm and professional"
	}
	system := `You write official communications from a school to parents, students and staff. ` + a.schoolContext() + `
Write clear, respectful, concise messages suitable for WhatsApp, SMS and a mobile app notification.
Address parents as "Dear Parent" (or the local-language equivalent) unless the audience is staff or students.
Never invent specific facts such as dates, times, amounts or names that the request does not give; if one is needed and missing, insert a clear placeholder like [DATE].
Sign off with the school name. Write every field in the requested language, using its native script.`
	prompt := fmt.Sprintf("Audience: %s\nTone: %s\nLanguage: %s\n\nRequest: %s", req.Audience, req.Tone, req.Language, req.Prompt)
	if strings.TrimSpace(req.Draft) != "" {
		prompt += "\n\nImprove this existing draft:\n" + req.Draft
	}
	text, err := a.ai.complete(r.Context(), system, prompt, "low", composeSchema)
	if err != nil {
		aiFail(w, err)
		return
	}
	var out struct {
		Title   string `json:"title"`
		Message string `json:"message"`
		SMS     string `json:"sms"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		out.Title, out.Message, out.SMS = "Announcement", text, text
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
	return ai.complete(ctx, system, "Translate into "+lang+":\n\n"+text, "low", nil)
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
	text, err := a.ai.complete(r.Context(), system, prompt, "low", nil)
	if err != nil {
		aiFail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"text": text})
}

// ---------- Assistant with tools ----------

type aiTool struct {
	def   anthropic.ToolParam
	roles []string // "staff" = staff with the ai.data permission
	run   func(u *User, input json.RawMessage) (any, error)
}

func tool(name, desc string, props map[string]any, required []string) anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        name,
		Description: anthropic.String(desc),
		InputSchema: anthropic.ToolInputSchemaParam{Properties: props, Required: required},
	}
}

func (a *App) assistantTools() []aiTool {
	staff := []string{"staff"}
	family := []string{"parent", "student"}
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

	return []aiTool{
		{
			def:   tool("get_school_overview", "Headline numbers: student/teacher/parent counts, today's attendance, outstanding and overdue fees, messages sent today.", map[string]any{}, nil),
			roles: staff,
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
			def: tool("get_attendance", "Attendance summary per class for a date, plus the list of absent and late students.",
				map[string]any{"date": str("Date as YYYY-MM-DD; defaults to today")}, nil),
			roles: staff,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct{ Date string }
				json.Unmarshal(in, &p)
				if !validDate(p.Date) {
					p.Date = today()
				}
				classes, err := a.queryMaps(`SELECT c.name||' - '||c.section AS class, COUNT(s.id) AS students,
					SUM(a.status='present') AS present, SUM(a.status='absent') AS absent, SUM(a.status='late') AS late
					FROM classes c LEFT JOIN students s ON s.class_id=c.id LEFT JOIN attendance a ON a.student_id=s.id AND a.date=?
					GROUP BY c.id ORDER BY c.name`, p.Date)
				if err != nil {
					return nil, err
				}
				missing, _ := a.queryMaps(`SELECT s.id AS student_id, s.name, c.name||' - '||c.section AS class, a.status
					FROM attendance a JOIN students s ON s.id=a.student_id LEFT JOIN classes c ON c.id=s.class_id
					WHERE a.date=? AND a.status IN ('absent','late')`, p.Date)
				return map[string]any{"date": p.Date, "classes": classes, "absent_or_late": missing}, nil
			},
		},
		{
			def: tool("find_students", "Search students by name, admission number or parent name. Returns class, parent contact, attendance % and fee balance.",
				map[string]any{"query": str("Name or admission number to search for")}, []string{"query"}),
			roles: staff,
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct{ Query string }
				json.Unmarshal(in, &p)
				q := "%" + p.Query + "%"
				return a.queryMaps(`SELECT s.id, s.name, s.admission_no, c.name||' - '||c.section AS class, p.name AS parent, p.phone AS parent_phone,
					(SELECT ROUND(100.0*SUM(status!='absent')/MAX(COUNT(*),1),1) FROM attendance WHERE student_id=s.id) AS attendance_pct,
					(SELECT COALESCE(SUM(amount),0) FROM fees WHERE student_id=s.id AND status='unpaid') AS fee_balance
					FROM students s LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users p ON p.id=s.parent_id
					WHERE s.name LIKE ? OR s.admission_no LIKE ? OR p.name LIKE ? LIMIT 20`, q, q, q)
			},
		},
		{
			def: tool("get_student_report", "Full report for one student: profile, attendance history and stats, fees, published exam results and upcoming homework/exams.",
				map[string]any{"student_id": num("The student's id")}, []string{"student_id"}),
			roles: append(staff, family...),
			run: func(u *User, in json.RawMessage) (any, error) {
				var p struct {
					StudentID int64 `json:"student_id"`
				}
				json.Unmarshal(in, &p)
				if !a.canViewStudent(u, p.StudentID) {
					return nil, errors.New("not permitted: this student is not linked to your account")
				}
				return a.studentReport(p.StudentID)
			},
		},
		{
			def:   tool("list_my_children", "List the students linked to the signed-in parent or student account, with their ids.", map[string]any{}, nil),
			roles: family,
			run: func(u *User, _ json.RawMessage) (any, error) {
				return a.queryMaps(`SELECT s.id, s.name, c.name||' - '||c.section AS class FROM students s LEFT JOIN classes c ON c.id=s.class_id
					WHERE s.parent_id=? OR s.user_id=?`, u.ID, u.ID)
			},
		},
		{
			def: tool("get_unpaid_fees", "Unpaid fees with student, class, parent and due date.",
				map[string]any{"overdue_only": map[string]any{"type": "boolean", "description": "Only fees past their due date"}}, nil),
			roles: staff,
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
			def:   tool("get_exam_performance", "Exam list with class averages, highest and lowest marks.", map[string]any{}, nil),
			roles: staff,
			run: func(u *User, _ json.RawMessage) (any, error) {
				return a.queryMaps(`SELECT e.id, e.subject, e.title, e.exam_date, e.max_marks, c.name||' - '||c.section AS class,
					COUNT(x.id) AS results, ROUND(AVG(x.marks),1) AS average, MAX(x.marks) AS highest, MIN(x.marks) AS lowest
					FROM exams e JOIN classes c ON c.id=e.class_id LEFT JOIN results x ON x.exam_id=e.id
					GROUP BY e.id ORDER BY e.exam_date DESC LIMIT 50`)
			},
		},
		{
			def: tool("get_staff_today", "Teacher attendance for a date: who is present, late, absent or on leave, pending leave requests, and relief (substitute) teacher assignments.",
				map[string]any{"date": str("Date as YYYY-MM-DD; defaults to today")}, nil),
			roles: staff,
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
			roles: staff,
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
					rows, err = a.queryMaps(q+`u.name LIKE ? ORDER BY t.day, t.period`, "%"+p.TeacherName+"%")
				} else {
					rows, err = a.queryMaps(q+`(c.name||' '||c.section) LIKE ? ORDER BY t.day, t.period`, "%"+strings.ReplaceAll(p.ClassName, "-", "")+"%")
				}
				return map[string]any{"period_times": a.timetableConfig().Times, "entries": rows}, err
			},
		},
		{
			def: tool("get_communication_stats", "Messages sent by channel, status and category over the last N days.",
				map[string]any{"days": num("Number of days to look back (default 7)")}, nil),
			roles: staff,
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
		(SELECT ROUND(AVG(marks),1) FROM results y WHERE y.exam_id=e.id) AS class_average
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

	var tools []aiTool
	var toolParams []anthropic.ToolUnionParam
	for _, t := range a.assistantTools() {
		for _, role := range t.roles {
			if role == u.Role || (role == "staff" && hasPerm(u.Role, PAIData)) {
				t := t
				tools = append(tools, t)
				toolParams = append(toolParams, anthropic.ToolUnionParam{OfTool: &t.def})
				break
			}
		}
	}

	conv := a.ai.getConversation(req.ConversationID, u.ID)
	if conv == nil {
		req.ConversationID = newToken()[:16]
		conv = &conversation{userID: u.ID}
	}
	// Work on a copy so a failed request leaves the stored history untouched (append-only).
	messages := append([]anthropic.MessageParam{}, conv.messages...)
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(req.Message)))

	var persona string
	if hasPerm(u.Role, PAIData) {
		persona = "You are Epoch AI, the assistant inside the school's management system, helping school staff. Use the tools to look up live school data before answering questions about students, attendance, fees, exams or communications. Give concrete, actionable answers; when useful, suggest what message to send and to whom. Format answers with short paragraphs and bullet lists (Markdown)."
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
		resp, err := a.ai.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.ai.model),
			MaxTokens: 16000,
			System:    []anthropic.TextBlockParam{{Text: system}},
			Messages:  messages,
			Tools:     toolParams,
		}, requestOptions("medium", nil)...)
		if err != nil {
			aiFail(w, aiError(err))
			return
		}
		if resp.StopReason == anthropic.StopReasonRefusal {
			errJSON(w, 422, "The AI declined this request; please rephrase it.")
			return
		}
		messages = append(messages, resp.ToParam())

		var results []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				answer = b.Text
			case anthropic.ToolUseBlock:
				var out any
				var runErr error = fmt.Errorf("unknown tool %s", b.Name)
				for _, t := range tools {
					if t.def.Name == b.Name {
						out, runErr = t.run(u, json.RawMessage(b.JSON.Input.Raw()))
						break
					}
				}
				if runErr != nil {
					results = append(results, anthropic.NewToolResultBlock(b.ID, runErr.Error(), true))
					continue
				}
				data, _ := json.Marshal(out)
				results = append(results, anthropic.NewToolResultBlock(b.ID, string(data), false))
			}
		}
		if resp.StopReason != anthropic.StopReasonToolUse {
			break
		}
		messages = append(messages, anthropic.NewUserMessage(results...))
	}
	if answer == "" {
		answer = "I couldn't complete that request. Please try asking in a different way."
	}

	a.ai.mu.Lock()
	conv.messages = messages
	conv.updated = time.Now()
	a.ai.convs[req.ConversationID] = conv
	a.ai.mu.Unlock()
	writeJSON(w, 200, map[string]string{"conversation_id": req.ConversationID, "answer": answer})
}
