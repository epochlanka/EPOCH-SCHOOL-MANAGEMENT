package main

import (
	"net/http"
	"strings"
)

// Pickup changes: parents tell the school who is collecting their child, that the
// child is not taking the bus, or that they will collect early. Staff responsible
// for the child see the day's list and confirm or decline, and the parent is told.

var pickupKinds = map[string]bool{"collector": true, "no_bus": true, "early": true, "other": true}

// pickupDetails describes a request in the given language.
func pickupDetails(lang, kind, name, relation, phone, at, note string) string {
	var s string
	switch kind {
	case "collector":
		s = L(lang, "pickup.kind.collector", name, relation, phone)
	case "no_bus":
		s = L(lang, "pickup.kind.no_bus")
	case "early":
		s = L(lang, "pickup.kind.early", at)
	default:
		s = note
		note = ""
	}
	if note != "" {
		s += "." + L(lang, "leave.note", note)
	}
	return s
}

// pickupStaff returns staff who should hear about a child's pickup change: the class
// teacher, the head of the child's section, front office and school management.
func (a *App) pickupStaff(studentID int64) []int64 {
	ids, _ := a.queryIDs(`SELECT c.teacher_id FROM students s JOIN classes c ON c.id=s.class_id WHERE s.id=?
		UNION SELECT sec.head_id FROM students s JOIN classes c ON c.id=s.class_id JOIN sections sec ON sec.id=c.section_id WHERE s.id=?
		UNION SELECT id FROM users WHERE active=1 AND role IN ('front_office','principal','vice_principal')`, studentID, studentID)
	return ids
}

func (a *App) handleCreatePickup(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		StudentID int64  `json:"student_id"`
		Date      string `json:"date"`
		Kind      string `json:"kind"`
		Name      string `json:"collector_name"`
		Relation  string `json:"collector_relation"`
		Phone     string `json:"collector_phone"`
		At        string `json:"pickup_time"`
		Note      string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if u.Role != "parent" || !a.canViewStudent(u, req.StudentID) {
		errJSON(w, 403, "Only the child's parent can send a pickup change")
		return
	}
	req.Name, req.Relation, req.Phone, req.Note = strings.TrimSpace(req.Name), strings.TrimSpace(req.Relation), strings.TrimSpace(req.Phone), strings.TrimSpace(req.Note)
	switch {
	case !pickupKinds[req.Kind]:
		errJSON(w, 400, "Choose what is changing")
		return
	case !validDate(req.Date) || req.Date < today():
		errJSON(w, 400, "Choose today or a later date")
		return
	case req.Kind == "collector" && (req.Name == "" || req.Phone == ""):
		errJSON(w, 400, "Enter the name and phone number of the person collecting")
		return
	case req.Kind == "early" && req.At == "":
		errJSON(w, 400, "Enter the pickup time")
		return
	case req.Kind == "other" && req.Note == "":
		errJSON(w, 400, "Describe the change")
		return
	}
	res, err := a.db.Exec(`INSERT INTO pickup_requests(student_id,parent_id,date,kind,collector_name,collector_relation,collector_phone,pickup_time,note,status,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,'pending',?)`, req.StudentID, u.ID, req.Date, req.Kind, req.Name, req.Relation, req.Phone, req.At, req.Note, now())
	if err != nil {
		serverError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	var student, class string
	a.db.QueryRow(`SELECT s.name, COALESCE(c.name||' - '||c.section,'') FROM students s LEFT JOIN classes c ON c.id=s.class_id WHERE s.id=?`, req.StudentID).Scan(&student, &class)
	a.notify.NotifyEach("pickup", a.pickupStaff(req.StudentID), []string{"app"}, func(lang string) (string, string) {
		return L(lang, "pickup.new.title", student),
			L(lang, "pickup.new.body", student, class, dateL(req.Date, lang), pickupDetails(lang, req.Kind, req.Name, req.Relation, req.Phone, req.At, req.Note))
	})
	writeJSON(w, 201, map[string]any{"id": id})
}

// handleListPickups: parents see their own requests; staff see a day's requests for
// the classes they work with.
func (a *App) handleListPickups(w http.ResponseWriter, r *http.Request, u *User) {
	base := `SELECT p.*, s.name AS student_name, s.admission_no, c.name||' - '||c.section AS class_name,
		rt.name AS route_name, rt.bus_no, par.name AS parent_name, par.phone AS parent_phone, h.name AS handled_by_name
		FROM pickup_requests p JOIN students s ON s.id=p.student_id LEFT JOIN classes c ON c.id=s.class_id
		LEFT JOIN routes rt ON rt.id=s.route_id LEFT JOIN users par ON par.id=p.parent_id LEFT JOIN users h ON h.id=p.handled_by `
	var rows []map[string]any
	var err error
	if !isStaff(u.Role) {
		rows, err = a.queryMaps(base+`WHERE p.parent_id=? ORDER BY p.date DESC, p.id DESC LIMIT 50`, u.ID)
	} else {
		if !hasPerm(u.Role, PStudentsView) {
			errJSON(w, 403, "You do not have permission to do this")
			return
		}
		date := r.URL.Query().Get("date")
		if !validDate(date) {
			date = today()
		}
		scope, args := a.scopeSQL(u, PStudentsView, "s.class_id")
		rows, err = a.queryMaps(base+`WHERE p.date=? AND p.status<>'cancelled'`+scope+` ORDER BY p.status='pending' DESC, c.name, s.name`, append([]any{date}, args...)...)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleRespondPickup(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var req struct {
		Status string `json:"status"` // acknowledged | declined
		Note   string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.Status != "acknowledged" && req.Status != "declined" {
		errJSON(w, 400, "Status must be acknowledged or declined")
		return
	}
	var studentID, parentID, classID int64
	var date, student string
	if err := a.db.QueryRow(`SELECT p.student_id, p.parent_id, p.date, s.name, COALESCE(s.class_id,0) FROM pickup_requests p JOIN students s ON s.id=p.student_id WHERE p.id=? AND p.status<>'cancelled'`, id).
		Scan(&studentID, &parentID, &date, &student, &classID); err != nil {
		errJSON(w, 404, "Pickup change not found")
		return
	}
	if !a.inClassScope(u, PStudentsView, classID) {
		errJSON(w, 403, "Not your class")
		return
	}
	note := strings.TrimSpace(req.Note)
	a.db.Exec(`UPDATE pickup_requests SET status=?, response=?, handled_by=?, handled_at=? WHERE id=?`, req.Status, note, u.ID, now(), id)
	key := "pickup.ack"
	if req.Status == "declined" {
		key = "pickup.declined"
	}
	a.notify.NotifyEach("pickup", []int64{parentID}, []string{"app", "whatsapp"}, func(lang string) (string, string) {
		extra := ""
		if note != "" {
			extra = L(lang, "leave.note", note)
		}
		return L(lang, key+".title"), L(lang, key+".body", student, dateL(date, lang), extra)
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleCancelPickup(w http.ResponseWriter, r *http.Request, u *User) {
	res, _ := a.db.Exec(`UPDATE pickup_requests SET status='cancelled' WHERE id=? AND parent_id=? AND status IN ('pending','acknowledged') AND date>=?`, pathID(r, "id"), u.ID, today())
	if n, _ := res.RowsAffected(); n == 0 {
		errJSON(w, 404, "This pickup change can no longer be cancelled")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// pendingPickups counts today's unconfirmed pickup changes a staff member can act on.
func (a *App) pendingPickups(u *User) float64 {
	if !isStaff(u.Role) || !hasPerm(u.Role, PStudentsView) {
		return 0
	}
	scope, args := a.scopeSQL(u, PStudentsView, "s.class_id")
	return a.scalar(`SELECT COUNT(*) FROM pickup_requests p JOIN students s ON s.id=p.student_id WHERE p.date=? AND p.status='pending'`+scope,
		append([]any{today()}, args...)...)
}
