package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request, u *User) {
	t := today()
	counts := map[string]float64{
		"students": a.scalar(`SELECT COUNT(*) FROM students`),
		"teachers": a.scalar(`SELECT COUNT(*) FROM users WHERE role IN ` + teachingRolesSQL + ` AND active=1`),
		"staff":    a.scalar(`SELECT COUNT(*) FROM users WHERE role IN ` + staffRolesSQL + ` AND active=1`),
		"parents":  a.scalar(`SELECT COUNT(*) FROM users WHERE role='parent' AND active=1`),
		"classes":  a.scalar(`SELECT COUNT(*) FROM classes`),
		"routes":   a.scalar(`SELECT COUNT(*) FROM routes`),
	}
	att := map[string]float64{
		"present": a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status='present'`, t),
		"absent":  a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status='absent'`, t),
		"late":    a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status='late'`, t),
	}
	att["unmarked"] = counts["students"] - att["present"] - att["absent"] - att["late"]

	fees := map[string]float64{
		"outstanding":     a.scalar(`SELECT COALESCE(SUM(amount),0) FROM fees WHERE status='unpaid'`),
		"overdue_count":   a.scalar(`SELECT COUNT(*) FROM fees WHERE status='unpaid' AND due_date < ?`, t),
		"overdue_amount":  a.scalar(`SELECT COALESCE(SUM(amount),0) FROM fees WHERE status='unpaid' AND due_date < ?`, t),
		"collected_month": a.scalar(`SELECT COALESCE(SUM(amount),0) FROM fees WHERE status='paid' AND substr(paid_at,1,7)=?`, t[:7]),
	}

	trend := []map[string]any{}
	for i := 13; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		ds := d.Format(dateLayout)
		total := a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=?`, ds)
		present := a.scalar(`SELECT COUNT(*) FROM attendance WHERE date=? AND status IN ('present','late')`, ds)
		pct := 0.0
		if total > 0 {
			pct = present * 100 / total
		}
		trend = append(trend, map[string]any{"date": ds, "label": d.Format("Mon 02"), "percent": pct, "marked": total})
	}

	channels, err := a.queryMaps(`SELECT channel, status, COUNT(*) AS n FROM deliveries WHERE created_at >= ? GROUP BY channel, status`,
		time.Now().AddDate(0, 0, -30).Format(tsLayout))
	if err != nil {
		serverError(w, err)
		return
	}
	recent, _ := a.queryMaps(`SELECT d.channel,d.category,d.title,d.status,d.created_at,u.name AS user_name
		FROM deliveries d LEFT JOIN users u ON u.id=d.user_id ORDER BY d.id DESC LIMIT 10`)
	absentees, _ := a.queryMaps(`SELECT s.name, c.name||' - '||c.section AS class_name, a.status
		FROM attendance a JOIN students s ON s.id=a.student_id LEFT JOIN classes c ON c.id=s.class_id
		WHERE a.date=? AND a.status IN ('absent','late') ORDER BY a.status, s.name`, t)
	upcoming, _ := a.queryMaps(`SELECT e.subject,e.title,e.exam_date,c.name||' - '||c.section AS class_name
		FROM exams e JOIN classes c ON c.id=e.class_id WHERE e.exam_date >= ? ORDER BY e.exam_date LIMIT 5`, t)

	// Teacher attendance today.
	onLeave := a.staffOnLeave(t)
	teachingIDs, _ := a.queryIDs(`SELECT id FROM users WHERE active=1 AND role IN ` + teachingRolesSQL)
	staffAtt := map[string]int{"present": 0, "late": 0, "absent": 0, "on_leave": 0, "not_marked": 0}
	marks := map[int64]string{}
	markRows, _ := a.db.Query(`SELECT user_id, status FROM staff_attendance WHERE date=?`, t)
	for markRows.Next() {
		var id int64
		var st string
		markRows.Scan(&id, &st)
		marks[id] = st
	}
	markRows.Close()
	for _, id := range teachingIDs {
		switch {
		case onLeave[id] || marks[id] == "leave":
			staffAtt["on_leave"]++
		case marks[id] == "":
			staffAtt["not_marked"]++
		default:
			staffAtt[marks[id]]++
		}
	}
	staffAway, _ := a.queryMaps(`SELECT u.id, u.name, u.role, COALESCE(sa.status,'leave') AS status,
		(SELECT leave_type FROM leave_requests l WHERE l.user_id=u.id AND l.status='approved' AND ? BETWEEN l.from_date AND l.to_date LIMIT 1) AS leave_type
		FROM users u LEFT JOIN staff_attendance sa ON sa.user_id=u.id AND sa.date=?
		WHERE u.active=1 AND u.role IN `+staffRolesSQL+` AND (sa.status IN ('absent','late','leave') OR EXISTS
		(SELECT 1 FROM leave_requests l WHERE l.user_id=u.id AND l.status='approved' AND ? BETWEEN l.from_date AND l.to_date)) ORDER BY u.name`, t, t, t)
	for _, row := range staffAway {
		row["role_label"] = roleLabel(row["role"].(string))
	}
	pending, _ := a.queryMaps(`SELECT l.id, l.leave_type, l.from_date, l.to_date, l.reason, u.name, u.role,
		CAST(julianday(l.to_date)-julianday(l.from_date)+1 AS INTEGER) AS days
		FROM leave_requests l JOIN users u ON u.id=l.user_id WHERE l.status='pending' AND l.user_id<>? ORDER BY l.from_date LIMIT 10`, u.ID)
	for _, row := range pending {
		row["role_label"] = roleLabel(row["role"].(string))
		row["type_label"] = leaveTypes[row["leave_type"].(string)]
	}
	subs := map[string]float64{
		"assigned": a.scalar(`SELECT COUNT(*) FROM substitutions WHERE date=? AND status='assigned'`, t),
		"unfilled": a.scalar(`SELECT COUNT(*) FROM substitutions WHERE date=? AND status='unfilled'`, t),
	}

	writeJSON(w, 200, map[string]any{
		"staff_attendance": staffAtt, "staff_away": staffAway, "pending_leave": pending, "substitutions": subs,
		"can_approve": hasPerm(u.Role, PLeaveApprove), "can_fees": hasPerm(u.Role, PFeesView),
		"counts": counts, "attendance": att, "fees": fees, "trend": trend,
		"channels": channels, "recent": recent, "absentees": absentees, "upcoming_exams": upcoming,
		"sent_today": a.scalar(`SELECT COUNT(*) FROM deliveries WHERE substr(created_at,1,10)=?`, t),
	})
}

// ---------- Users ----------

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request, u *User) {
	role := r.URL.Query().Get("role")
	q := `SELECT u.id,u.name,u.email,u.phone,u.whatsapp,u.role,u.language,u.active,u.created_at,
		(SELECT group_concat(s.name, ', ') FROM students s WHERE s.parent_id=u.id) AS children
		FROM users u WHERE (?='' OR u.role=?)`
	args := []any{role, role}
	if !hasPerm(u.Role, PUsers) { // others may only browse staff and parents
		q += ` AND u.role<>'student'`
	}
	rows, err := a.queryMaps(q+` ORDER BY u.name`, args...)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, row := range rows {
		row["role_label"] = roleLabel(row["role"].(string))
	}
	writeJSON(w, 200, rows)
}

type userReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	WhatsApp string `json:"whatsapp"`
	Role     string `json:"role"`
	Language string `json:"language"`
	Password string `json:"password"`
	Active   *bool  `json:"active"`
}

func (req *userReq) validate(creating bool) string {
	req.Name, req.Email = strings.TrimSpace(req.Name), strings.TrimSpace(req.Email)
	if req.Name == "" || !strings.Contains(req.Email, "@") {
		return "Name and a valid email are required"
	}
	if !validRole(req.Role) {
		return "Invalid role"
	}
	if creating && len(req.Password) < 6 {
		return "Password must be at least 6 characters"
	}
	if !creating && req.Password != "" && len(req.Password) < 6 {
		return "Password must be at least 6 characters"
	}
	if !validLanguage(req.Language) {
		req.Language = "English"
	}
	if req.WhatsApp == "" {
		req.WhatsApp = req.Phone
	}
	return ""
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request, u *User) {
	var req userReq
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if msg := req.validate(true); msg != "" {
		errJSON(w, 400, msg)
		return
	}
	if req.Role == "admin" && u.Role != "admin" {
		errJSON(w, 403, "Only a System Admin can create admin accounts")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		serverError(w, err)
		return
	}
	res, err := a.db.Exec(`INSERT INTO users(name,email,phone,whatsapp,role,language,password_hash,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		req.Name, req.Email, req.Phone, req.WhatsApp, req.Role, req.Language, hash, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			errJSON(w, 409, "A user with this email already exists")
			return
		}
		serverError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]any{"id": id})
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var req userReq
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if msg := req.validate(false); msg != "" {
		errJSON(w, 400, msg)
		return
	}
	var currentRole string
	a.db.QueryRow(`SELECT role FROM users WHERE id=?`, id).Scan(&currentRole)
	if (req.Role == "admin" || currentRole == "admin") && u.Role != "admin" {
		errJSON(w, 403, "Only a System Admin can change admin accounts")
		return
	}
	active := 1
	if req.Active != nil && !*req.Active {
		if id == u.ID {
			errJSON(w, 400, "You cannot deactivate your own account")
			return
		}
		active = 0
	}
	_, err := a.db.Exec(`UPDATE users SET name=?,email=?,phone=?,whatsapp=?,role=?,language=?,active=? WHERE id=?`,
		req.Name, req.Email, req.Phone, req.WhatsApp, req.Role, req.Language, active, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			errJSON(w, 409, "A user with this email already exists")
			return
		}
		serverError(w, err)
		return
	}
	if req.Password != "" {
		hash, err := hashPassword(req.Password)
		if err != nil {
			serverError(w, err)
			return
		}
		a.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
		a.db.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	}
	if active == 0 {
		a.db.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	if id == u.ID {
		errJSON(w, 400, "You cannot delete your own account")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Classes ----------

func (a *App) handleListClasses(w http.ResponseWriter, r *http.Request, u *User) {
	// ?scope=attendance|academic limits the list to classes the user may work with.
	where, args := "", []any{}
	switch r.URL.Query().Get("scope") {
	case "attendance":
		where, args = a.scopeSQL(u, PAttendance, "c.id")
	case "academic":
		where, args = a.scopeSQL(u, PAcademic, "c.id")
	}
	rows, err := a.queryMaps(`SELECT c.id,c.name,c.section,c.teacher_id,c.section_id,t.name AS teacher_name,
		c.name||' - '||c.section AS label, sec.name AS section_name,
		(SELECT COUNT(*) FROM students s WHERE s.class_id=c.id) AS student_count,
		(SELECT COUNT(*) FROM class_subjects cs WHERE cs.class_id=c.id) AS subject_count,
		(SELECT COALESCE(SUM(periods_per_week),0) FROM class_subjects cs WHERE cs.class_id=c.id) AS periods
		FROM classes c LEFT JOIN users t ON t.id=c.teacher_id LEFT JOIN sections sec ON sec.id=c.section_id
		WHERE 1=1`+where+` ORDER BY sec.sort, c.name, c.section`, args...)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveClass(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Name      string `json:"name"`
		Section   string `json:"section"`
		TeacherID int64  `json:"teacher_id"`
		SectionID int64  `json:"section_id"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		errJSON(w, 400, "Class name is required")
		return
	}
	if req.TeacherID > 0 {
		var role string
		a.db.QueryRow(`SELECT role FROM users WHERE id=?`, req.TeacherID).Scan(&role)
		if !isTeaching(role) {
			errJSON(w, 400, "The class teacher must be a teaching staff member")
			return
		}
	}
	var err error
	if id := pathID(r, "id"); id > 0 {
		_, err = a.db.Exec(`UPDATE classes SET name=?,section=?,teacher_id=?,section_id=? WHERE id=?`, req.Name, req.Section, nullID(req.TeacherID), nullID(req.SectionID), id)
	} else {
		_, err = a.db.Exec(`INSERT INTO classes(name,section,teacher_id,section_id) VALUES(?,?,?,?)`, req.Name, req.Section, nullID(req.TeacherID), nullID(req.SectionID))
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			errJSON(w, 409, "This class and section already exist")
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteClass(w http.ResponseWriter, r *http.Request, u *User) {
	if _, err := a.db.Exec(`DELETE FROM classes WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Transport routes ----------

func (a *App) handleListRoutes(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT r.*, (SELECT COUNT(*) FROM students s WHERE s.route_id=r.id) AS student_count,
		(SELECT message FROM bus_updates b WHERE b.route_id=r.id ORDER BY b.id DESC LIMIT 1) AS last_update,
		(SELECT created_at FROM bus_updates b WHERE b.route_id=r.id ORDER BY b.id DESC LIMIT 1) AS last_update_at
		FROM routes r ORDER BY r.name`)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveRoute(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Name        string `json:"name"`
		BusNo       string `json:"bus_no"`
		DriverName  string `json:"driver_name"`
		DriverPhone string `json:"driver_phone"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		errJSON(w, 400, "Route name is required")
		return
	}
	var err error
	if id := pathID(r, "id"); id > 0 {
		_, err = a.db.Exec(`UPDATE routes SET name=?,bus_no=?,driver_name=?,driver_phone=? WHERE id=?`, req.Name, req.BusNo, req.DriverName, req.DriverPhone, id)
	} else {
		_, err = a.db.Exec(`INSERT INTO routes(name,bus_no,driver_name,driver_phone) VALUES(?,?,?,?)`, req.Name, req.BusNo, req.DriverName, req.DriverPhone)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteRoute(w http.ResponseWriter, r *http.Request, u *User) {
	if _, err := a.db.Exec(`DELETE FROM routes WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Students ----------

func (a *App) handleListStudents(w http.ResponseWriter, r *http.Request, u *User) {
	classID := queryInt(r, "class_id")
	q := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	scope, scopeArgs := a.scopeSQL(u, PStudentsView, "s.class_id")
	rows, err := a.queryMaps(`SELECT s.id,s.admission_no,s.name,s.gender,s.dob,s.class_id,s.parent_id,s.route_id,s.user_id,
		c.name||' - '||c.section AS class_name, p.name AS parent_name, p.phone AS parent_phone, rt.name AS route_name,
		(SELECT COALESCE(SUM(amount),0) FROM fees f WHERE f.student_id=s.id AND f.status='unpaid') AS fee_due,
		(SELECT ROUND(100.0*SUM(status!='absent')/COUNT(*),1) FROM attendance a WHERE a.student_id=s.id) AS attendance_pct
		FROM students s LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users p ON p.id=s.parent_id LEFT JOIN routes rt ON rt.id=s.route_id
		WHERE (?=0 OR s.class_id=?) AND (s.name LIKE ? OR s.admission_no LIKE ? OR COALESCE(p.name,'') LIKE ?)`+scope+`
		ORDER BY c.name, c.section, s.name`, append([]any{classID, classID, q, q, q}, scopeArgs...)...)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveStudent(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		AdmissionNo string `json:"admission_no"`
		Name        string `json:"name"`
		ClassID     int64  `json:"class_id"`
		ParentID    int64  `json:"parent_id"`
		RouteID     int64  `json:"route_id"`
		Gender      string `json:"gender"`
		DOB         string `json:"dob"`
		// Optional: create a new parent account in the same step.
		ParentName     string `json:"parent_name"`
		ParentEmail    string `json:"parent_email"`
		ParentPhone    string `json:"parent_phone"`
		ParentPassword string `json:"parent_password"`
		ParentLanguage string `json:"parent_language"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.ClassID == 0 {
		errJSON(w, 400, "Student name and class are required")
		return
	}
	if req.ParentID == 0 && strings.TrimSpace(req.ParentEmail) != "" {
		pr := userReq{Name: req.ParentName, Email: req.ParentEmail, Phone: req.ParentPhone, Role: "parent", Password: req.ParentPassword, Language: req.ParentLanguage}
		if pr.Password == "" {
			pr.Password = newToken()[:10]
		}
		if msg := pr.validate(true); msg != "" {
			errJSON(w, 400, "Parent: "+msg)
			return
		}
		hash, _ := hashPassword(pr.Password)
		res, err := a.db.Exec(`INSERT INTO users(name,email,phone,whatsapp,role,language,password_hash,created_at) VALUES(?,?,?,?,?,?,?,?)`,
			pr.Name, pr.Email, pr.Phone, pr.WhatsApp, "parent", pr.Language, hash, now())
		if err != nil {
			errJSON(w, 409, "Could not create parent (email may already exist)")
			return
		}
		req.ParentID, _ = res.LastInsertId()
	}
	id := pathID(r, "id")
	if req.AdmissionNo == "" {
		req.AdmissionNo = fmt.Sprintf("EP%04d", 1001+int(a.scalar(`SELECT COALESCE(MAX(id),0) FROM students`)))
	}
	var err error
	if id > 0 {
		_, err = a.db.Exec(`UPDATE students SET admission_no=?,name=?,class_id=?,parent_id=?,route_id=?,gender=?,dob=? WHERE id=?`,
			req.AdmissionNo, req.Name, req.ClassID, nullID(req.ParentID), nullID(req.RouteID), req.Gender, req.DOB, id)
	} else {
		var res interface{ LastInsertId() (int64, error) }
		res, err = a.db.Exec(`INSERT INTO students(admission_no,name,class_id,parent_id,route_id,gender,dob,created_at) VALUES(?,?,?,?,?,?,?,?)`,
			req.AdmissionNo, req.Name, req.ClassID, nullID(req.ParentID), nullID(req.RouteID), req.Gender, req.DOB, now())
		if err == nil {
			id, _ = res.LastInsertId()
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			errJSON(w, 409, "Admission number already exists")
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (a *App) handleDeleteStudent(w http.ResponseWriter, r *http.Request, u *User) {
	if _, err := a.db.Exec(`DELETE FROM students WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Settings ----------

var publicSettings = []string{"school_name", "school_phone", "school_email", "school_address"}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request, u *User) {
	out := map[string]any{}
	if hasPerm(u.Role, PSettings) {
		rows, err := a.queryMaps(`SELECT key,value FROM settings`)
		if err != nil {
			serverError(w, err)
			return
		}
		for _, row := range rows {
			out[row["key"].(string)] = row["value"]
		}
		out["timetable"] = a.timetableConfig()
		out["providers"] = map[string]any{
			"sms": a.notify.sms.Name(), "whatsapp": a.notify.wa.Name(), "ai": a.ai.statusText(), "ai_model": a.cfg.AIModel,
		}
	} else {
		for _, k := range publicSettings {
			out[k] = a.setting(k)
		}
	}
	writeJSON(w, 200, out)
}

func (a *App) handlePutSettings(w http.ResponseWriter, r *http.Request, u *User) {
	var req map[string]string
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	for k, v := range req {
		if _, known := defaultSettings[k]; !known || k == "fee_last_run" {
			continue
		}
		if strings.HasPrefix(k, "channels_") {
			v = cleanChannels(v)
		}
		if err := a.setSetting(k, strings.TrimSpace(v)); err != nil {
			serverError(w, err)
			return
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleTestMessage(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Channel string `json:"channel"`
		To      string `json:"to"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	text := fmt.Sprintf("Test message from %s via Epoch School System.", a.setting("school_name"))
	status, err := a.notify.sendDirect(r.Context(), req.Channel, req.To, text)
	if err != nil {
		errJSON(w, 502, "Send failed: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": status})
}
