package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Transport ----------

func (a *App) handleListBusUpdates(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT b.*, r.name AS route_name, r.bus_no, u.name AS created_by_name
		FROM bus_updates b JOIN routes r ON r.id=b.route_id LEFT JOIN users u ON u.id=b.created_by
		ORDER BY b.id DESC LIMIT 100`)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleCreateBusUpdate(w http.ResponseWriter, r *http.Request, u *User) {
	routeID := pathID(r, "id")
	var req struct {
		Message string `json:"message"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		errJSON(w, 400, "Message is required")
		return
	}
	var name, busNo string
	if err := a.db.QueryRow(`SELECT name,bus_no FROM routes WHERE id=?`, routeID).Scan(&name, &busNo); err != nil {
		errJSON(w, 404, "Route not found")
		return
	}
	a.db.Exec(`INSERT INTO bus_updates(route_id,message,created_by,created_at) VALUES(?,?,?,?)`, routeID, req.Message, u.ID, now())
	parents, _ := a.queryIDs(`SELECT parent_id FROM students WHERE route_id=? UNION SELECT user_id FROM students WHERE route_id=?`, routeID, routeID)
	title := "Bus Update"
	if busNo != "" {
		title += " – Bus " + busNo
	}
	n, err := a.notify.Notify("transport", title, fmt.Sprintf("%s: %s", name, req.Message), parents, a.notify.channelsFor("transport"))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true, "notified": n})
}

// ---------- Announcements ----------

func (a *App) handleListAnnouncements(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT an.*, u.name AS created_by_name,
		CASE an.audience WHEN 'class' THEN (SELECT name||' - '||section FROM classes WHERE id=an.audience_id)
			WHEN 'route' THEN (SELECT name FROM routes WHERE id=an.audience_id)
			WHEN 'section' THEN (SELECT name FROM sections WHERE id=an.audience_id)
			WHEN 'user' THEN (SELECT name FROM users WHERE id=an.audience_id) ELSE '' END AS audience_label
		FROM announcements an LEFT JOIN users u ON u.id=an.created_by ORDER BY an.id DESC LIMIT 200`)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

// audienceIDs resolves an announcement audience into user IDs.
func (a *App) audienceIDs(audience string, id int64, sender int64) ([]int64, error) {
	switch audience {
	case "all":
		return a.queryIDs(`SELECT id FROM users WHERE active=1 AND id<>?`, sender)
	case "parents":
		return a.queryIDs(`SELECT id FROM users WHERE active=1 AND role='parent'`)
	case "teachers":
		return a.queryIDs(`SELECT id FROM users WHERE active=1 AND role IN `+staffRolesSQL+` AND id<>?`, sender)
	case "teaching":
		return a.queryIDs(`SELECT id FROM users WHERE active=1 AND role IN `+teachingRolesSQL+` AND id<>?`, sender)
	case "section":
		return a.queryIDs(`SELECT s.parent_id FROM students s JOIN classes c ON c.id=s.class_id WHERE c.section_id=?
			UNION SELECT s.user_id FROM students s JOIN classes c ON c.id=s.class_id WHERE c.section_id=?`, id, id)
	case "students":
		return a.queryIDs(`SELECT id FROM users WHERE active=1 AND role='student'`)
	case "class":
		return a.classAudience(id), nil
	case "class_parents":
		return a.queryIDs(`SELECT parent_id FROM students WHERE class_id=?`, id)
	case "route":
		return a.queryIDs(`SELECT parent_id FROM students WHERE route_id=? UNION SELECT user_id FROM students WHERE route_id=?`, id, id)
	case "user":
		return []int64{id}, nil
	}
	return nil, fmt.Errorf("unknown audience %q", audience)
}

func (a *App) handleCreateAnnouncement(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Title      string   `json:"title"`
		Body       string   `json:"body"`
		Audience   string   `json:"audience"`
		AudienceID int64    `json:"audience_id"`
		Channels   []string `json:"channels"`
		Category   string   `json:"category"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	req.Title, req.Body = strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	channels := cleanChannels(strings.Join(req.Channels, ","))
	if req.Title == "" || req.Body == "" || channels == "" {
		errJSON(w, 400, "Title, message and at least one channel are required")
		return
	}
	if !hasPerm(u.Role, PAnnounceAll) {
		ok := req.Audience == "user" || ((req.Audience == "class" || req.Audience == "class_parents") && a.inClassScope(u, PAcademic, req.AudienceID))
		if !ok {
			errJSON(w, 403, "You can send to your own classes or to one person")
			return
		}
	}
	ids, err := a.audienceIDs(req.Audience, req.AudienceID, u.ID)
	if err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.Category == "" {
		req.Category = "announcement"
	}
	n, err := a.notify.Notify(req.Category, req.Title, req.Body, ids, strings.Split(channels, ","))
	if err != nil {
		serverError(w, err)
		return
	}
	a.db.Exec(`INSERT INTO announcements(title,body,audience,audience_id,channels,recipients,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		req.Title, req.Body, req.Audience, nullID(req.AudienceID), channels, n, u.ID, now())
	writeJSON(w, 201, map[string]any{"ok": true, "recipients": n})
}

// ---------- In-app notifications ----------

func (a *App) handleListNotifications(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT * FROM notifications WHERE user_id=? ORDER BY id DESC LIMIT 100`, u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleReadAllNotifications(w http.ResponseWriter, r *http.Request, u *User) {
	a.db.Exec(`UPDATE notifications SET is_read=1 WHERE user_id=?`, u.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleReadNotification(w http.ResponseWriter, r *http.Request, u *User) {
	a.db.Exec(`UPDATE notifications SET is_read=1 WHERE id=? AND user_id=?`, pathID(r, "id"), u.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Two-way messaging ----------

// canMessage enforces who may talk to whom: parents and students can only
// message staff; staff can message anyone.
func canMessage(from, toRole string) bool {
	return isStaff(from) || isStaff(toRole)
}

func (a *App) handleContacts(w http.ResponseWriter, r *http.Request, u *User) {
	var rows []map[string]any
	var err error
	if isStaff(u.Role) {
		rows, err = a.queryMaps(`SELECT u.id,u.name,u.role,
			COALESCE((SELECT group_concat(s.name, ', ') FROM students s WHERE s.parent_id=u.id),
				(SELECT group_concat(c.name||' - '||c.section, ', ') FROM classes c WHERE c.teacher_id=u.id)) AS detail
			FROM users u WHERE u.active=1 AND u.id<>? AND u.role<>'student' ORDER BY u.role='parent', u.name`, u.ID)
	} else {
		// Families see their children's class and subject teachers first, then school management and office.
		rows, err = a.queryMaps(`SELECT u.id,u.name,u.role,
			(SELECT group_concat(c.name||' - '||c.section, ', ') FROM classes c WHERE c.teacher_id=u.id) AS detail,
			EXISTS(SELECT 1 FROM students s JOIN classes c ON c.id=s.class_id WHERE (s.parent_id=? OR s.user_id=?)
				AND (c.teacher_id=u.id OR EXISTS(SELECT 1 FROM class_subjects cs WHERE cs.class_id=c.id AND cs.teacher_id=u.id))) AS mine
			FROM users u WHERE u.active=1 AND u.role IN `+staffRolesSQL+` ORDER BY mine DESC, u.name`, u.ID, u.ID)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	for _, row := range rows {
		row["role_label"] = roleLabel(row["role"].(string))
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleConversations(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT c.other_id, u.name, u.role, m.body, m.created_at, m.sender_id, c.unread
		FROM (SELECT CASE WHEN sender_id=? THEN recipient_id ELSE sender_id END AS other_id, MAX(id) AS last_id,
			SUM(CASE WHEN recipient_id=? AND is_read=0 THEN 1 ELSE 0 END) AS unread
			FROM messages WHERE sender_id=? OR recipient_id=? GROUP BY other_id) c
		JOIN messages m ON m.id=c.last_id JOIN users u ON u.id=c.other_id ORDER BY m.id DESC`, u.ID, u.ID, u.ID, u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) threadMessages(userID, otherID int64, limit int) ([]map[string]any, error) {
	return a.queryMaps(`SELECT * FROM (SELECT m.*, s.name AS sender_name FROM messages m JOIN users s ON s.id=m.sender_id
		WHERE (m.sender_id=? AND m.recipient_id=?) OR (m.sender_id=? AND m.recipient_id=?)
		ORDER BY m.id DESC LIMIT ?) ORDER BY id`, userID, otherID, otherID, userID, limit)
}

func (a *App) handleThread(w http.ResponseWriter, r *http.Request, u *User) {
	other := pathID(r, "id")
	contact, err := a.queryOne(`SELECT id,name,role,phone FROM users WHERE id=?`, other)
	if err != nil {
		errJSON(w, 404, "User not found")
		return
	}
	if !canMessage(u.Role, contact["role"].(string)) {
		errJSON(w, 403, "You cannot message this user")
		return
	}
	if !isStaff(u.Role) {
		delete(contact, "phone")
	}
	contact["role_label"] = roleLabel(contact["role"].(string))
	msgs, err := a.threadMessages(u.ID, other, 200)
	if err != nil {
		serverError(w, err)
		return
	}
	a.db.Exec(`UPDATE messages SET is_read=1 WHERE sender_id=? AND recipient_id=?`, other, u.ID)
	writeJSON(w, 200, map[string]any{"contact": contact, "messages": msgs})
}

func (a *App) handleSendMessage(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		To   int64  `json:"to"`
		Body string `json:"body"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" || len(req.Body) > 4000 {
		errJSON(w, 400, "Message must be between 1 and 4000 characters")
		return
	}
	var role string
	if err := a.db.QueryRow(`SELECT role FROM users WHERE id=? AND active=1`, req.To).Scan(&role); err != nil || req.To == u.ID {
		errJSON(w, 404, "Recipient not found")
		return
	}
	if !canMessage(u.Role, role) {
		errJSON(w, 403, "You cannot message this user")
		return
	}
	res, err := a.db.Exec(`INSERT INTO messages(sender_id,recipient_id,body,created_at) VALUES(?,?,?,?)`, u.ID, req.To, req.Body, now())
	if err != nil {
		serverError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	preview := req.Body
	if len([]rune(preview)) > 140 {
		preview = string([]rune(preview)[:140]) + "…"
	}
	a.notify.Notify("message", "New message from "+u.Name, preview, []int64{req.To}, a.notify.channelsFor("messages"))
	writeJSON(w, 201, map[string]any{"id": id})
}

// ---------- Reports ----------

func (a *App) handleCommReport(w http.ResponseWriter, r *http.Request, u *User) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if !validDate(from) {
		from = time.Now().AddDate(0, 0, -30).Format(dateLayout)
	}
	if !validDate(to) {
		to = today()
	}
	channel, category, status := q.Get("channel"), q.Get("category"), q.Get("status")
	where := `WHERE substr(d.created_at,1,10) BETWEEN ? AND ? AND (?='' OR d.channel=?) AND (?='' OR d.category=?) AND (?='' OR d.status=?)`
	args := []any{from, to, channel, channel, category, category, status, status}
	rows, err := a.queryMaps(`SELECT d.id,d.channel,d.recipient,d.category,d.title,d.status,d.error,d.created_at,d.sent_at,u.name AS user_name, u.role AS user_role
		FROM deliveries d LEFT JOIN users u ON u.id=d.user_id `+where+` ORDER BY d.id DESC LIMIT 500`, args...)
	if err != nil {
		serverError(w, err)
		return
	}
	byChannel, _ := a.queryMaps(`SELECT d.channel, d.status, COUNT(*) AS n FROM deliveries d `+where+` GROUP BY d.channel, d.status`, args...)
	byCategory, _ := a.queryMaps(`SELECT d.category, COUNT(*) AS n FROM deliveries d `+where+` GROUP BY d.category ORDER BY n DESC`, args...)
	byDay, _ := a.queryMaps(`SELECT substr(d.created_at,1,10) AS day, COUNT(*) AS n FROM deliveries d `+where+` GROUP BY day ORDER BY day`, args...)
	writeJSON(w, 200, map[string]any{"from": from, "to": to, "rows": rows, "by_channel": byChannel, "by_category": byCategory, "by_day": byDay})
}

// ---------- Parent / student portal ----------

func (a *App) handlePortal(w http.ResponseWriter, r *http.Request, u *User) {
	sid := pathID(r, "id")
	if !a.canViewStudent(u, sid) {
		errJSON(w, 403, "You can only view your own child's information")
		return
	}
	student, err := a.queryOne(`SELECT s.id,s.name,s.admission_no,s.gender,s.class_id,s.route_id,
		c.name||' - '||c.section AS class_name, t.name AS class_teacher, t.id AS class_teacher_id,
		r.name AS route_name, r.bus_no, r.driver_name, r.driver_phone
		FROM students s LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users t ON t.id=c.teacher_id
		LEFT JOIN routes r ON r.id=s.route_id WHERE s.id=?`, sid)
	if err != nil {
		errJSON(w, 404, "Student not found")
		return
	}
	classID, _ := student["class_id"].(int64)
	routeID, _ := student["route_id"].(int64)
	t := today()
	month := t[:7]

	attendance, _ := a.queryMaps(`SELECT date,status FROM attendance WHERE student_id=? ORDER BY date DESC LIMIT 30`, sid)
	stats := map[string]float64{
		"present": a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND status='present' AND substr(date,1,7)=?`, sid, month),
		"absent":  a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND status='absent' AND substr(date,1,7)=?`, sid, month),
		"late":    a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND status='late' AND substr(date,1,7)=?`, sid, month),
		"overall": a.scalar(`SELECT ROUND(100.0*SUM(status!='absent')/MAX(COUNT(*),1),1) FROM attendance WHERE student_id=?`, sid),
	}
	var todayStatus string
	a.db.QueryRow(`SELECT status FROM attendance WHERE student_id=? AND date=?`, sid, t).Scan(&todayStatus)
	fees, _ := a.queryMaps(`SELECT id,title,amount,due_date,status,paid_at, CASE WHEN status='unpaid' AND due_date<? THEN 1 ELSE 0 END AS overdue
		FROM fees WHERE student_id=? ORDER BY status DESC, due_date`, t, sid)
	homework, _ := a.queryMaps(`SELECT h.subject,h.title,h.description,h.due_date,h.created_at,u.name AS teacher_name
		FROM homework h LEFT JOIN users u ON u.id=h.teacher_id WHERE h.class_id=? ORDER BY h.due_date DESC LIMIT 30`, classID)
	exams, _ := a.queryMaps(`SELECT subject,title,exam_date,exam_time,max_marks FROM exams WHERE class_id=? AND exam_date>=? ORDER BY exam_date`, classID, t)
	results, _ := a.queryMaps(`SELECT e.subject,e.title,e.exam_date,e.max_marks,x.marks,x.remarks,
		(SELECT ROUND(AVG(marks),1) FROM results y WHERE y.exam_id=e.id) AS class_average
		FROM results x JOIN exams e ON e.id=x.exam_id WHERE x.student_id=? AND e.published=1 ORDER BY e.exam_date DESC`, sid)
	for _, res := range results {
		m, _ := res["marks"].(float64)
		mx, _ := res["max_marks"].(float64)
		if mx > 0 {
			res["grade"] = grade(m * 100 / mx)
		}
	}
	bus, _ := a.queryMaps(`SELECT message,created_at FROM bus_updates WHERE route_id=? ORDER BY id DESC LIMIT 5`, routeID)
	notifications, _ := a.queryMaps(`SELECT id,category,title,body,is_read,created_at FROM notifications WHERE user_id=? ORDER BY id DESC LIMIT 20`, u.ID)
	cfg := a.timetableConfig()
	timetable, _ := a.queryMaps(`SELECT t.day, t.period, s.name AS subject_name, u.name AS teacher_name,
		(SELECT sb.name FROM substitutions x JOIN users sb ON sb.id=x.substitute_id WHERE x.date=? AND x.class_id=t.class_id AND x.period=t.period AND t.day=?) AS relief_teacher
		FROM timetable t JOIN subjects s ON s.id=t.subject_id LEFT JOIN users u ON u.id=t.teacher_id WHERE t.class_id=? ORDER BY t.day, t.period`,
		t, cfg.schoolDay(t), classID)
	loans, _ := a.queryMaps(`SELECT b.title, l.due_date, CASE WHEN l.due_date<? THEN 1 ELSE 0 END AS overdue FROM loans l JOIN books b ON b.id=l.book_id
		WHERE l.student_id=? AND l.returned_at=''`, t, sid)

	writeJSON(w, 200, map[string]any{
		"student": student, "today_status": todayStatus, "attendance": attendance, "attendance_stats": stats,
		"fees": fees, "homework": homework, "exams": exams, "results": results, "bus_updates": bus,
		"notifications": notifications, "timetable": timetable, "timetable_config": cfg, "today_day": cfg.schoolDay(t), "loans": loans,
	})
}

// ---------- Scheduler ----------

// runScheduler sends automated fee reminders once a day after the configured hour.
func (a *App) runScheduler(ctx context.Context) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		hour, _ := strconv.Atoi(a.setting("fee_reminder_hour"))
		if time.Now().Hour() >= hour && a.setting("fee_last_run") != today() {
			a.setSetting("fee_last_run", today())
			if n, err := a.sendFeeReminders(0, false); err != nil {
				log.Printf("fee reminders: %v", err)
			} else if n > 0 {
				log.Printf("Sent %d automated fee reminders", n)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
