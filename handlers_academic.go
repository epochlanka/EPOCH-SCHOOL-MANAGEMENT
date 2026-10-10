package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ---------- Attendance ----------

func (a *App) handleGetAttendance(w http.ResponseWriter, r *http.Request, u *User) {
	classID := queryInt(r, "class_id")
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		date = today()
	}
	if !a.inClassScope(u, PAttendance, classID) {
		errJSON(w, 403, "You can only mark attendance for your own class")
		return
	}
	rows, err := a.queryMaps(`SELECT s.id AS student_id, s.name, s.admission_no, p.name AS parent_name,
		COALESCE(a.status,'') AS status,
		(SELECT e.time_out FROM early_leaves e WHERE e.student_id=s.id AND e.date=? AND e.voided=0 ORDER BY e.id DESC LIMIT 1) AS left_early
		FROM students s LEFT JOIN attendance a ON a.student_id=s.id AND a.date=?
		LEFT JOIN users p ON p.id=s.parent_id
		WHERE s.class_id=? ORDER BY s.name`, date, date, classID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"date": date, "students": rows})
}

func (a *App) handleSaveAttendance(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassID int64  `json:"class_id"`
		Date    string `json:"date"`
		Records []struct {
			StudentID int64  `json:"student_id"`
			Status    string `json:"status"`
		} `json:"records"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if !validDate(req.Date) {
		errJSON(w, 400, "Invalid date")
		return
	}
	if req.Date > today() {
		errJSON(w, 400, "Attendance cannot be marked for a future date")
		return
	}
	if !a.inClassScope(u, PAttendance, req.ClassID) {
		errJSON(w, 403, "You can only mark attendance for your own class")
		return
	}
	type change struct {
		studentID int64
		status    string
	}
	var changes []change
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	for _, rec := range req.Records {
		if rec.Status != "present" && rec.Status != "absent" && rec.Status != "late" {
			continue
		}
		var inClass int64
		tx.QueryRow(`SELECT COUNT(*) FROM students WHERE id=? AND class_id=?`, rec.StudentID, req.ClassID).Scan(&inClass)
		if inClass == 0 {
			continue
		}
		var prev string
		tx.QueryRow(`SELECT status FROM attendance WHERE student_id=? AND date=?`, rec.StudentID, req.Date).Scan(&prev)
		if _, err := tx.Exec(`INSERT INTO attendance(student_id,date,status,marked_by,created_at) VALUES(?,?,?,?,?)
			ON CONFLICT(student_id,date) DO UPDATE SET status=excluded.status, marked_by=excluded.marked_by`,
			rec.StudentID, req.Date, rec.Status, u.ID, now()); err != nil {
			serverError(w, err)
			return
		}
		if prev != rec.Status {
			changes = append(changes, change{rec.StudentID, rec.Status})
		}
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}

	// Automated alerts to parents for today's attendance.
	alerts := 0
	if req.Date == today() {
		notifyPresent := a.setting("notify_present") == "1"
		school := a.setting("school_name")
		channels := a.notify.channelsFor("attendance")
		for _, c := range changes {
			var name string
			var parentID sql.NullInt64
			if a.db.QueryRow(`SELECT name, parent_id FROM students WHERE id=?`, c.studentID).Scan(&name, &parentID) != nil || !parentID.Valid {
				continue
			}
			status := c.status
			if status == "present" && !notifyPresent {
				continue
			}
			msg := func(lang string) (string, string) {
				if status == "present" {
					return L(lang, "present.title"), L(lang, "present.body", name, dateL(req.Date, lang), school)
				}
				return L(lang, status+".title"), L(lang, status+".body", name, dateL(req.Date, lang))
			}
			if n, err := a.notify.NotifyEach("attendance", []int64{parentID.Int64}, channels, msg); err == nil {
				alerts += n
			} else {
				log.Printf("attendance alert: %v", err)
			}
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "saved": len(req.Records), "alerts_sent": alerts})
}

func (a *App) handleAttendanceReport(w http.ResponseWriter, r *http.Request, u *User) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if !validDate(from) {
		from = time.Now().AddDate(0, 0, -30).Format(dateLayout)
	}
	if !validDate(to) {
		to = today()
	}
	classID := queryInt(r, "class_id")
	scope, scopeArgs := a.scopeSQL(u, PStudentsView, "s.class_id")
	rows, err := a.queryMaps(`SELECT s.id, s.name, s.admission_no, c.name||' - '||c.section AS class_name,
		SUM(a.status='present') AS present, SUM(a.status='absent') AS absent, SUM(a.status='late') AS late, COUNT(a.id) AS total,
		ROUND(100.0*SUM(a.status!='absent')/MAX(COUNT(a.id),1),1) AS percent
		FROM students s LEFT JOIN classes c ON c.id=s.class_id
		LEFT JOIN attendance a ON a.student_id=s.id AND a.date BETWEEN ? AND ?
		WHERE (?=0 OR s.class_id=?)`+scope+` GROUP BY s.id ORDER BY percent, s.name`, append([]any{from, to, classID, classID}, scopeArgs...)...)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"from": from, "to": to, "rows": rows})
}

// ---------- Fees ----------

func (a *App) handleListFees(w http.ResponseWriter, r *http.Request, u *User) {
	status := r.URL.Query().Get("status")
	classID := queryInt(r, "class_id")
	t := today()
	rows, err := a.queryMaps(`SELECT f.*, s.name AS student_name, s.admission_no, c.name||' - '||c.section AS class_name,
		p.name AS parent_name, p.phone AS parent_phone,
		CASE WHEN f.status='unpaid' AND f.due_date < ? THEN 1 ELSE 0 END AS overdue
		FROM fees f JOIN students s ON s.id=f.student_id LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users p ON p.id=s.parent_id
		WHERE (?='' OR f.status=? OR (?='overdue' AND f.status='unpaid' AND f.due_date < ?))
		AND (?=0 OR s.class_id=?)
		ORDER BY f.status DESC, f.due_date`, t, status, status, status, t, classID, classID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleCreateFees(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassID    int64   `json:"class_id"`
		StudentIDs []int64 `json:"student_ids"`
		Title      string  `json:"title"`
		Amount     float64 `json:"amount"`
		DueDate    string  `json:"due_date"`
		Notify     bool    `json:"notify"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" || req.Amount <= 0 || !validDate(req.DueDate) {
		errJSON(w, 400, "Title, a positive amount and a due date are required")
		return
	}
	ids := req.StudentIDs
	if len(ids) == 0 {
		var err error
		ids, err = a.queryIDs(`SELECT id FROM students WHERE (?=0 OR class_id=?)`, req.ClassID, req.ClassID)
		if err != nil {
			serverError(w, err)
			return
		}
	}
	if len(ids) == 0 {
		errJSON(w, 400, "No students selected")
		return
	}
	for _, sid := range ids {
		if _, err := a.db.Exec(`INSERT INTO fees(student_id,title,amount,due_date,created_at) VALUES(?,?,?,?,?)`, sid, req.Title, req.Amount, req.DueDate, now()); err != nil {
			serverError(w, err)
			return
		}
	}
	sent := 0
	if req.Notify {
		parents, _ := a.queryIDs(`SELECT parent_id FROM students WHERE id IN (`+placeholders(len(ids))+`)`, int64Args(ids)...)
		sent, _ = a.notify.NotifyEach("fees", parents, a.notify.channelsFor("fees"), func(lang string) (string, string) {
			return L(lang, "feenew.title"), L(lang, "feenew.body", req.Title, money(req.Amount), dateL(req.DueDate, lang))
		})
	}
	writeJSON(w, 201, map[string]any{"created": len(ids), "notified": sent})
}

func (a *App) handlePayFee(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var title string
	var amount float64
	var name string
	var parentID sql.NullInt64
	err := a.db.QueryRow(`SELECT f.title,f.amount,s.name,s.parent_id FROM fees f JOIN students s ON s.id=f.student_id WHERE f.id=? AND f.status='unpaid'`, id).
		Scan(&title, &amount, &name, &parentID)
	if err != nil {
		errJSON(w, 404, "Unpaid fee not found")
		return
	}
	if _, err := a.db.Exec(`UPDATE fees SET status='paid', paid_at=? WHERE id=?`, now(), id); err != nil {
		serverError(w, err)
		return
	}
	if parentID.Valid {
		a.notify.NotifyEach("fees", []int64{parentID.Int64}, []string{"app", "sms"}, func(lang string) (string, string) {
			return L(lang, "paid.title"), L(lang, "paid.body", money(amount), title, name)
		})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleRemindFee(w http.ResponseWriter, r *http.Request, u *User) {
	n, err := a.sendFeeReminders(pathID(r, "id"), true)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]int{"reminded": n})
}

func (a *App) handleRemindDue(w http.ResponseWriter, r *http.Request, u *User) {
	n, err := a.sendFeeReminders(0, true)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]int{"reminded": n})
}

// sendFeeReminders reminds parents about unpaid fees. With feeID=0 it covers every
// fee that is overdue or due within the configured window. Unless force is set, a
// fee is reminded at most once per day.
func (a *App) sendFeeReminders(feeID int64, force bool) (int, error) {
	days := 3
	fmt.Sscan(a.setting("fee_reminder_days"), &days)
	t := today()
	horizon := time.Now().AddDate(0, 0, days).Format(dateLayout)
	q := `SELECT f.id,f.title,f.amount,f.due_date,s.name,s.parent_id FROM fees f JOIN students s ON s.id=f.student_id
		WHERE f.status='unpaid' AND s.parent_id IS NOT NULL`
	var args []any
	if feeID > 0 {
		q += ` AND f.id=?`
		args = append(args, feeID)
	} else {
		q += ` AND f.due_date <= ?`
		args = append(args, horizon)
		if !force {
			q += ` AND f.last_reminded <> ?`
			args = append(args, t)
		}
	}
	type fee struct {
		id               int64
		title, due, name string
		amount           float64
		parent           int64
	}
	rows, err := a.db.Query(q, args...)
	if err != nil {
		return 0, err
	}
	var list []fee
	for rows.Next() {
		var f fee
		if err := rows.Scan(&f.id, &f.title, &f.amount, &f.due, &f.name, &f.parent); err == nil {
			list = append(list, f)
		}
	}
	rows.Close()

	channels := a.notify.channelsFor("fees")
	count := 0
	for _, f := range list {
		key := "feedue"
		if f.due < t {
			key = "feeover"
		}
		f := f
		msg := func(lang string) (string, string) {
			return L(lang, key+".title"), L(lang, key+".body", f.title, money(f.amount), f.name, dateL(f.due, lang))
		}
		if _, err := a.notify.NotifyEach("fees", []int64{f.parent}, channels, msg); err != nil {
			return count, err
		}
		a.db.Exec(`UPDATE fees SET last_reminded=? WHERE id=?`, t, f.id)
		count++
	}
	return count, nil
}

func (a *App) handleDeleteFee(w http.ResponseWriter, r *http.Request, u *User) {
	if _, err := a.db.Exec(`DELETE FROM fees WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) examInScope(u *User, examID int64) bool {
	var classID int64
	a.db.QueryRow(`SELECT class_id FROM exams WHERE id=?`, examID).Scan(&classID)
	return a.inClassScope(u, PAcademic, classID)
}

// classAudience returns parents and student accounts for a class.
func (a *App) classAudience(classID int64) []int64 {
	ids, _ := a.queryIDs(`SELECT parent_id FROM students WHERE class_id=? UNION SELECT user_id FROM students WHERE class_id=?`, classID, classID)
	return ids
}

func (a *App) classLabel(classID int64) string {
	var s string
	a.db.QueryRow(`SELECT name||' - '||section FROM classes WHERE id=?`, classID).Scan(&s)
	return s
}

// ---------- Homework ----------

func (a *App) handleListHomework(w http.ResponseWriter, r *http.Request, u *User) {
	classID := queryInt(r, "class_id")
	scope, scopeArgs := a.scopeSQL(u, PAcademic, "h.class_id")
	rows, err := a.queryMaps(`SELECT h.*, c.name||' - '||c.section AS class_name, t.name AS teacher_name
		FROM homework h JOIN classes c ON c.id=h.class_id LEFT JOIN users t ON t.id=h.teacher_id
		WHERE (?=0 OR h.class_id=?)`+scope+` ORDER BY h.due_date DESC, h.id DESC LIMIT 200`, append([]any{classID, classID}, scopeArgs...)...)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleCreateHomework(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassID     int64  `json:"class_id"`
		Subject     string `json:"subject"`
		Title       string `json:"title"`
		Description string `json:"description"`
		DueDate     string `json:"due_date"`
		Notify      bool   `json:"notify"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.ClassID == 0 || strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Title) == "" || !validDate(req.DueDate) {
		errJSON(w, 400, "Class, subject, title and due date are required")
		return
	}
	if !a.inClassScope(u, PAcademic, req.ClassID) {
		errJSON(w, 403, "You can only set homework for classes you teach")
		return
	}
	if _, err := a.db.Exec(`INSERT INTO homework(class_id,subject,title,description,due_date,teacher_id,created_at) VALUES(?,?,?,?,?,?,?)`,
		req.ClassID, req.Subject, req.Title, req.Description, req.DueDate, u.ID, now()); err != nil {
		serverError(w, err)
		return
	}
	sent := 0
	if req.Notify {
		class := a.classLabel(req.ClassID)
		sent, _ = a.notify.NotifyEach("homework", a.classAudience(req.ClassID), a.notify.channelsFor("homework"), func(lang string) (string, string) {
			return L(lang, "hw.title"), joinNonEmpty(L(lang, "hw.body", req.Subject, class, req.Title, dateL(req.DueDate, lang)), req.Description)
		})
	}
	writeJSON(w, 201, map[string]any{"ok": true, "notified": sent})
}

func (a *App) handleDeleteHomework(w http.ResponseWriter, r *http.Request, u *User) {
	var classID int64
	a.db.QueryRow(`SELECT class_id FROM homework WHERE id=?`, pathID(r, "id")).Scan(&classID)
	if !a.inClassScope(u, PAcademic, classID) {
		errJSON(w, 403, "Not your class")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM homework WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Exams & results ----------

func (a *App) handleListExams(w http.ResponseWriter, r *http.Request, u *User) {
	classID := queryInt(r, "class_id")
	scope, scopeArgs := a.scopeSQL(u, PAcademic, "e.class_id")
	rows, err := a.queryMaps(`SELECT e.*, c.name||' - '||c.section AS class_name,
		(SELECT COUNT(*) FROM results x WHERE x.exam_id=e.id) AS result_count,
		(SELECT ROUND(AVG(marks),1) FROM results x WHERE x.exam_id=e.id) AS average
		FROM exams e JOIN classes c ON c.id=e.class_id WHERE (?=0 OR e.class_id=?)`+scope+`
		ORDER BY e.exam_date DESC`, append([]any{classID, classID}, scopeArgs...)...)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleCreateExam(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassID  int64   `json:"class_id"`
		Subject  string  `json:"subject"`
		Title    string  `json:"title"`
		ExamDate string  `json:"exam_date"`
		ExamTime string  `json:"exam_time"`
		MaxMarks float64 `json:"max_marks"`
		Notify   bool    `json:"notify"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.ClassID == 0 || strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Title) == "" || !validDate(req.ExamDate) {
		errJSON(w, 400, "Class, subject, title and exam date are required")
		return
	}
	if !a.inClassScope(u, PAcademic, req.ClassID) {
		errJSON(w, 403, "You can only schedule exams for classes you teach")
		return
	}
	if req.MaxMarks <= 0 {
		req.MaxMarks = 100
	}
	if _, err := a.db.Exec(`INSERT INTO exams(class_id,subject,title,exam_date,exam_time,max_marks,created_at) VALUES(?,?,?,?,?,?,?)`,
		req.ClassID, req.Subject, req.Title, req.ExamDate, req.ExamTime, req.MaxMarks, now()); err != nil {
		serverError(w, err)
		return
	}
	sent := 0
	if req.Notify {
		class := a.classLabel(req.ClassID)
		sent, _ = a.notify.NotifyEach("exams", a.classAudience(req.ClassID), a.notify.channelsFor("exams"), func(lang string) (string, string) {
			body := L(lang, "exam.body", req.Title, req.Subject, class, dateL(req.ExamDate, lang))
			if req.ExamTime != "" {
				body += L(lang, "exam.time", req.ExamTime)
			}
			return L(lang, "exam.title"), body + "."
		})
	}
	writeJSON(w, 201, map[string]any{"ok": true, "notified": sent})
}

func (a *App) handleDeleteExam(w http.ResponseWriter, r *http.Request, u *User) {
	if !a.examInScope(u, pathID(r, "id")) {
		errJSON(w, 403, "Not your class")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM exams WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleGetResults(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	if !a.examInScope(u, id) {
		errJSON(w, 403, "Not your class")
		return
	}
	exam, err := a.queryOne(`SELECT e.*, c.name||' - '||c.section AS class_name FROM exams e JOIN classes c ON c.id=e.class_id WHERE e.id=?`, id)
	if err != nil {
		errJSON(w, 404, "Exam not found")
		return
	}
	rows, err := a.queryMaps(`SELECT s.id AS student_id, s.name, s.admission_no, x.marks, COALESCE(x.remarks,'') AS remarks
		FROM students s LEFT JOIN results x ON x.student_id=s.id AND x.exam_id=?
		WHERE s.class_id=? ORDER BY s.name`, id, exam["class_id"])
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"exam": exam, "students": rows})
}

func (a *App) handleSaveResults(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var req struct {
		Publish bool `json:"publish"`
		Records []struct {
			StudentID int64    `json:"student_id"`
			Marks     *float64 `json:"marks"`
			Remarks   string   `json:"remarks"`
		} `json:"records"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if !a.examInScope(u, id) {
		errJSON(w, 403, "Not your class")
		return
	}
	var subject, title string
	var maxMarks float64
	var published int
	if err := a.db.QueryRow(`SELECT subject,title,max_marks,published FROM exams WHERE id=?`, id).Scan(&subject, &title, &maxMarks, &published); err != nil {
		errJSON(w, 404, "Exam not found")
		return
	}
	for _, rec := range req.Records {
		if rec.Marks == nil {
			continue
		}
		if *rec.Marks < 0 || *rec.Marks > maxMarks {
			errJSON(w, 400, fmt.Sprintf("Marks must be between 0 and %g", maxMarks))
			return
		}
		if _, err := a.db.Exec(`INSERT INTO results(exam_id,student_id,marks,remarks) VALUES(?,?,?,?)
			ON CONFLICT(exam_id,student_id) DO UPDATE SET marks=excluded.marks, remarks=excluded.remarks`,
			id, rec.StudentID, *rec.Marks, rec.Remarks); err != nil {
			serverError(w, err)
			return
		}
	}
	sent := 0
	if req.Publish {
		a.db.Exec(`UPDATE exams SET published=1 WHERE id=?`, id)
		rows, err := a.db.Query(`SELECT s.name, s.parent_id, s.user_id, x.marks FROM results x JOIN students s ON s.id=x.student_id WHERE x.exam_id=?`, id)
		if err != nil {
			serverError(w, err)
			return
		}
		type res struct {
			name           string
			parent, userID sql.NullInt64
			marks          float64
		}
		var list []res
		for rows.Next() {
			var x res
			if rows.Scan(&x.name, &x.parent, &x.userID, &x.marks) == nil {
				list = append(list, x)
			}
		}
		rows.Close()
		channels := a.notify.channelsFor("results")
		for _, x := range list {
			pct := x.marks * 100 / maxMarks
			x := x
			n, _ := a.notify.NotifyEach("results", []int64{x.parent.Int64, x.userID.Int64}, channels, func(lang string) (string, string) {
				return L(lang, "result.title"), L(lang, "result.body", x.name, subject, title,
					fmt.Sprintf("%g", x.marks), fmt.Sprintf("%g", maxMarks), fmt.Sprintf("%.0f", pct), grade(pct))
			})
			sent += n
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "notified": sent})
}
