package main

import (
	"net/http"
	"strings"
	"time"
)

// Early leave register: every time a child is collected from school before the end of
// the day, staff record who collected them (with ID), the reason category and details,
// the time and who authorised it. Records are permanent — they cannot be deleted,
// only voided by school management with a reason — and the registered parent is told
// immediately, which matters most when someone else collects the child.

var earlyReasons = map[string]bool{
	"bereavement": true, "illness": true, "family_illness": true, "medical": true,
	"emergency": true, "religious": true, "other": true,
}

var collectorRelations = map[string]bool{
	"mother": true, "father": true, "grandparent": true, "guardian": true,
	"sibling": true, "relative": true, "driver": true, "other": true,
}

func canVoidEarlyLeave(role string) bool {
	return role == "admin" || role == "principal" || role == "vice_principal"
}

func (a *App) handleCreateEarlyLeave(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		StudentID  int64  `json:"student_id"`
		Name       string `json:"collector_name"`
		Relation   string `json:"collector_relation"`
		IDNo       string `json:"collector_id_no"`
		Phone      string `json:"collector_phone"`
		ReasonType string `json:"reason_type"`
		Reason     string `json:"reason"`
		TimeOut    string `json:"time_out"`
		PickupID   int64  `json:"pickup_request_id"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	req.Name, req.IDNo, req.Phone, req.Reason = strings.TrimSpace(req.Name), strings.TrimSpace(req.IDNo), strings.TrimSpace(req.Phone), strings.TrimSpace(req.Reason)
	switch {
	case req.Name == "" || req.IDNo == "":
		errJSON(w, 400, "Enter the name and NIC / ID number of the person collecting")
		return
	case !collectorRelations[req.Relation]:
		errJSON(w, 400, "Choose the collector's relationship to the child")
		return
	case !earlyReasons[req.ReasonType]:
		errJSON(w, 400, "Choose the reason for leaving early")
		return
	case len([]rune(req.Reason)) < 3:
		errJSON(w, 400, "Write the reason — it is kept in the student's record")
		return
	}
	if req.TimeOut == "" {
		req.TimeOut = time.Now().Format("15:04")
	}
	if _, err := time.Parse("15:04", req.TimeOut); err != nil {
		errJSON(w, 400, "Invalid time")
		return
	}
	var classID, parentID int64
	var student, class string
	if err := a.db.QueryRow(`SELECT s.name, COALESCE(s.class_id,0), COALESCE(s.parent_id,0), COALESCE(c.name||' - '||c.section,'')
		FROM students s LEFT JOIN classes c ON c.id=s.class_id WHERE s.id=?`, req.StudentID).Scan(&student, &classID, &parentID, &class); err != nil {
		errJSON(w, 404, "Student not found")
		return
	}
	if !a.inClassScope(u, PGate, classID) {
		errJSON(w, 403, "You can only record early leave for your own classes")
		return
	}
	t := today()
	res, err := a.db.Exec(`INSERT INTO early_leaves(student_id,date,time_out,collector_name,collector_relation,collector_id_no,collector_phone,
		reason_type,reason,recorded_by,pickup_request_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		req.StudentID, t, req.TimeOut, req.Name, req.Relation, req.IDNo, req.Phone, req.ReasonType, req.Reason, u.ID, nullID(req.PickupID), now())
	if err != nil {
		serverError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	// The child was in school today: make sure attendance shows that.
	a.db.Exec(`INSERT OR IGNORE INTO attendance(student_id,date,status,marked_by,created_at) VALUES(?,?,'present',?,?)`, req.StudentID, t, u.ID, now())
	if req.PickupID > 0 {
		a.db.Exec(`UPDATE pickup_requests SET status='acknowledged', handled_by=?, handled_at=? WHERE id=? AND student_id=? AND status='pending'`, u.ID, now(), req.PickupID, req.StudentID)
	}

	// Tell the registered parent (and student account) at once, and the class teacher.
	msg := func(lang string) (string, string) {
		return L(lang, "early.title", student), L(lang, "early.body", student, req.TimeOut, dateL(t, lang), req.Name,
			L(lang, "rel."+req.Relation), L(lang, "early."+req.ReasonType), req.Reason)
	}
	family, _ := a.queryIDs(`SELECT parent_id FROM students WHERE id=? UNION SELECT user_id FROM students WHERE id=?`, req.StudentID, req.StudentID)
	a.notify.NotifyEach("early_leave", family, []string{"app"}, msg)
	var teacher int64
	a.db.QueryRow(`SELECT COALESCE(teacher_id,0) FROM classes WHERE id=?`, classID).Scan(&teacher)
	if teacher > 0 && teacher != u.ID {
		a.notify.NotifyEach("early_leave", []int64{teacher}, []string{"app"}, msg)
	}
	writeJSON(w, 201, map[string]any{"id": id, "parent_notified": parentID > 0})
}

func (a *App) handleListEarlyLeaves(w http.ResponseWriter, r *http.Request, u *User) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if !validDate(from) {
		from = time.Now().AddDate(0, 0, -30).Format(dateLayout)
	}
	if !validDate(to) {
		to = today()
	}
	reason, studentID := q.Get("reason"), queryInt(r, "student_id")
	scope, args := a.scopeSQL(u, PGate, "s.class_id")
	rows, err := a.queryMaps(`SELECT e.*, s.name AS student_name, s.admission_no, c.name||' - '||c.section AS class_name,
		par.name AS parent_name, par.phone AS parent_phone, rb.name AS recorded_by_name, vb.name AS voided_by_name,
		(SELECT COUNT(*) FROM early_leaves x WHERE x.student_id=e.student_id AND x.voided=0 AND x.date>=?) AS term_count
		FROM early_leaves e JOIN students s ON s.id=e.student_id LEFT JOIN classes c ON c.id=s.class_id
		LEFT JOIN users par ON par.id=s.parent_id LEFT JOIN users rb ON rb.id=e.recorded_by LEFT JOIN users vb ON vb.id=e.voided_by
		WHERE e.date BETWEEN ? AND ? AND (?='' OR e.reason_type=?) AND (?=0 OR e.student_id=?)`+scope+`
		ORDER BY e.date DESC, e.time_out DESC LIMIT 500`,
		append([]any{time.Now().AddDate(0, -4, 0).Format(dateLayout), from, to, reason, reason, studentID, studentID}, args...)...)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"from": from, "to": to, "rows": rows, "can_void": canVoidEarlyLeave(u.Role)})
}

// handleEarlyLeaveReturn records that the child came back to school later the same day.
func (a *App) handleEarlyLeaveReturn(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Time string `json:"time"`
	}
	readJSON(r, &req)
	if req.Time == "" {
		req.Time = time.Now().Format("15:04")
	}
	if !a.earlyLeaveInScope(u, pathID(r, "id")) {
		errJSON(w, 403, "Not your class")
		return
	}
	a.db.Exec(`UPDATE early_leaves SET returned_at=? WHERE id=? AND voided=0`, req.Time, pathID(r, "id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleVoidEarlyLeave marks a record as entered in error. The record itself is kept.
func (a *App) handleVoidEarlyLeave(w http.ResponseWriter, r *http.Request, u *User) {
	if !canVoidEarlyLeave(u.Role) {
		errJSON(w, 403, "Only the principal or an administrator can void a record")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil || len(strings.TrimSpace(req.Reason)) < 3 {
		errJSON(w, 400, "Give the reason for voiding this record")
		return
	}
	res, _ := a.db.Exec(`UPDATE early_leaves SET voided=1, void_reason=?, voided_by=?, voided_at=? WHERE id=? AND voided=0`,
		strings.TrimSpace(req.Reason), u.ID, now(), pathID(r, "id"))
	if n, _ := res.RowsAffected(); n == 0 {
		errJSON(w, 404, "Record not found or already voided")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) earlyLeaveInScope(u *User, id int64) bool {
	var classID int64
	a.db.QueryRow(`SELECT COALESCE(s.class_id,0) FROM early_leaves e JOIN students s ON s.id=e.student_id WHERE e.id=?`, id).Scan(&classID)
	return a.inClassScope(u, PGate, classID)
}
