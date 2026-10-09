package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---------- Admissions ----------

var admissionStatuses = []string{"enquiry", "test_scheduled", "offered", "enrolled", "rejected", "withdrawn"}

func (a *App) handleListAdmissions(w http.ResponseWriter, r *http.Request, u *User) {
	status := r.URL.Query().Get("status")
	q := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	rows, err := a.queryMaps(`SELECT ad.*, s.admission_no FROM admissions ad LEFT JOIN students s ON s.id=ad.student_id
		WHERE (?='' OR ad.status=?) AND (ad.child_name LIKE ? OR ad.parent_name LIKE ? OR ad.phone LIKE ?)
		ORDER BY CASE ad.status WHEN 'enquiry' THEN 0 WHEN 'test_scheduled' THEN 1 WHEN 'offered' THEN 2 ELSE 3 END, ad.id DESC`,
		status, status, q, q, q)
	if err != nil {
		serverError(w, err)
		return
	}
	counts, _ := a.queryMaps(`SELECT status, COUNT(*) AS n FROM admissions GROUP BY status`)
	writeJSON(w, 200, map[string]any{"rows": rows, "counts": counts})
}

func (a *App) handleSaveAdmission(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ChildName      string `json:"child_name"`
		DOB            string `json:"dob"`
		Gender         string `json:"gender"`
		GradeApplying  string `json:"grade_applying"`
		ParentName     string `json:"parent_name"`
		Phone          string `json:"phone"`
		Email          string `json:"email"`
		PreviousSchool string `json:"previous_school"`
		Status         string `json:"status"`
		FollowUp       string `json:"follow_up"`
		Notes          string `json:"notes"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.ChildName) == "" || strings.TrimSpace(req.ParentName) == "" {
		errJSON(w, 400, "Child and parent names are required")
		return
	}
	if req.Status == "" {
		req.Status = "enquiry"
	}
	valid := false
	for _, s := range admissionStatuses {
		valid = valid || s == req.Status
	}
	if !valid || req.Status == "enrolled" {
		errJSON(w, 400, "Invalid status (use Enrol to admit a student)")
		return
	}
	var err error
	if id := pathID(r, "id"); id > 0 {
		_, err = a.db.Exec(`UPDATE admissions SET child_name=?,dob=?,gender=?,grade_applying=?,parent_name=?,phone=?,email=?,previous_school=?,
			status=?,follow_up=?,notes=?,updated_at=? WHERE id=? AND status<>'enrolled'`,
			req.ChildName, req.DOB, req.Gender, req.GradeApplying, req.ParentName, req.Phone, req.Email, req.PreviousSchool,
			req.Status, req.FollowUp, req.Notes, now(), id)
	} else {
		_, err = a.db.Exec(`INSERT INTO admissions(child_name,dob,gender,grade_applying,parent_name,phone,email,previous_school,status,follow_up,notes,created_by,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, req.ChildName, req.DOB, req.Gender, req.GradeApplying, req.ParentName, req.Phone, req.Email,
			req.PreviousSchool, req.Status, req.FollowUp, req.Notes, u.ID, now(), now())
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleEnrol turns an application into a student and a parent login.
func (a *App) handleEnrol(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var req struct {
		ClassID        int64  `json:"class_id"`
		ParentEmail    string `json:"parent_email"`
		ParentPassword string `json:"parent_password"`
		Welcome        bool   `json:"welcome"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	var child, dob, gender, parent, phone, status string
	if err := a.db.QueryRow(`SELECT child_name,dob,gender,parent_name,phone,status FROM admissions WHERE id=?`, id).
		Scan(&child, &dob, &gender, &parent, &phone, &status); err != nil {
		errJSON(w, 404, "Application not found")
		return
	}
	if status == "enrolled" {
		errJSON(w, 400, "Already enrolled")
		return
	}
	if req.ClassID == 0 || !strings.Contains(req.ParentEmail, "@") {
		errJSON(w, 400, "Choose a class and the parent's login email")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	var parentID int64
	if err := tx.QueryRow(`SELECT id FROM users WHERE email=? AND role='parent'`, strings.TrimSpace(req.ParentEmail)).Scan(&parentID); err == sql.ErrNoRows {
		if len(req.ParentPassword) < 6 {
			errJSON(w, 400, "Set a parent password of at least 6 characters")
			return
		}
		hash, _ := hashPassword(req.ParentPassword)
		res, err := tx.Exec(`INSERT INTO users(name,email,phone,whatsapp,role,password_hash,created_at) VALUES(?,?,?,?,?,?,?)`,
			parent, strings.TrimSpace(req.ParentEmail), phone, phone, "parent", hash, now())
		if err != nil {
			errJSON(w, 409, "That email belongs to another account")
			return
		}
		parentID, _ = res.LastInsertId()
	}
	var maxID int64
	tx.QueryRow(`SELECT COALESCE(MAX(id),0) FROM students`).Scan(&maxID)
	res, err := tx.Exec(`INSERT INTO students(admission_no,name,class_id,parent_id,gender,dob,created_at) VALUES(?,?,?,?,?,?,?)`,
		fmt.Sprintf("EP%04d", 1001+maxID), child, req.ClassID, parentID, gender, dob, now())
	if err != nil {
		serverError(w, err)
		return
	}
	sid, _ := res.LastInsertId()
	tx.Exec(`UPDATE admissions SET status='enrolled', student_id=?, updated_at=? WHERE id=?`, sid, now(), id)
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	if req.Welcome {
		body := fmt.Sprintf("Dear Parent, welcome to %s! %s has been enrolled in %s. Download the school app and sign in with %s to receive attendance, homework and fee updates.",
			a.setting("school_name"), child, a.classLabel(req.ClassID), req.ParentEmail)
		a.notify.Notify("announcement", "Welcome to "+a.setting("school_name"), body, []int64{parentID}, []string{"app", "sms", "whatsapp"})
	}
	writeJSON(w, 200, map[string]any{"student_id": sid})
}

func (a *App) handleDeleteAdmission(w http.ResponseWriter, r *http.Request, u *User) {
	a.db.Exec(`DELETE FROM admissions WHERE id=?`, pathID(r, "id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Library ----------

func (a *App) handleListBooks(w http.ResponseWriter, r *http.Request, u *User) {
	q := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	rows, err := a.queryMaps(`SELECT b.*, b.copies - (SELECT COUNT(*) FROM loans l WHERE l.book_id=b.id AND l.returned_at='') AS available
		FROM books b WHERE b.title LIKE ? OR b.author LIKE ? OR b.isbn LIKE ? OR b.category LIKE ? ORDER BY b.title`, q, q, q, q)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveBook(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Title    string `json:"title"`
		Author   string `json:"author"`
		ISBN     string `json:"isbn"`
		Category string `json:"category"`
		Copies   int    `json:"copies"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" || req.Copies < 1 {
		errJSON(w, 400, "Title and at least 1 copy are required")
		return
	}
	var err error
	if id := pathID(r, "id"); id > 0 {
		_, err = a.db.Exec(`UPDATE books SET title=?,author=?,isbn=?,category=?,copies=? WHERE id=?`, req.Title, req.Author, req.ISBN, req.Category, req.Copies, id)
	} else {
		_, err = a.db.Exec(`INSERT INTO books(title,author,isbn,category,copies,created_at) VALUES(?,?,?,?,?,?)`, req.Title, req.Author, req.ISBN, req.Category, req.Copies, now())
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteBook(w http.ResponseWriter, r *http.Request, u *User) {
	a.db.Exec(`DELETE FROM books WHERE id=?`, pathID(r, "id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleListLoans(w http.ResponseWriter, r *http.Request, u *User) {
	status := r.URL.Query().Get("status") // active | overdue | returned | ""
	t := today()
	rows, err := a.queryMaps(`SELECT l.*, b.title, s.name AS student_name, s.admission_no, c.name||' - '||c.section AS class_name,
		CASE WHEN l.returned_at='' AND l.due_date<? THEN 1 ELSE 0 END AS overdue
		FROM loans l JOIN books b ON b.id=l.book_id JOIN students s ON s.id=l.student_id LEFT JOIN classes c ON c.id=s.class_id
		WHERE (?='' OR (?='active' AND l.returned_at='') OR (?='overdue' AND l.returned_at='' AND l.due_date<?) OR (?='returned' AND l.returned_at<>''))
		ORDER BY l.returned_at<>'', l.due_date LIMIT 300`, t, status, status, status, t, status)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleIssueBook(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		BookID    int64  `json:"book_id"`
		StudentID int64  `json:"student_id"`
		DueDate   string `json:"due_date"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.DueDate == "" {
		req.DueDate = time.Now().AddDate(0, 0, 14).Format(dateLayout)
	}
	if req.BookID == 0 || req.StudentID == 0 || !validDate(req.DueDate) {
		errJSON(w, 400, "Choose a book, a student and a due date")
		return
	}
	if a.scalar(`SELECT copies - (SELECT COUNT(*) FROM loans WHERE book_id=? AND returned_at='') FROM books WHERE id=?`, req.BookID, req.BookID) <= 0 {
		errJSON(w, 409, "No copies of this book are available")
		return
	}
	if _, err := a.db.Exec(`INSERT INTO loans(book_id,student_id,issued_at,due_date,issued_by) VALUES(?,?,?,?,?)`, req.BookID, req.StudentID, today(), req.DueDate, u.ID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]bool{"ok": true})
}

func (a *App) handleReturnBook(w http.ResponseWriter, r *http.Request, u *User) {
	a.db.Exec(`UPDATE loans SET returned_at=? WHERE id=? AND returned_at=''`, today(), pathID(r, "id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleRemindOverdueBooks(w http.ResponseWriter, r *http.Request, u *User) {
	t := today()
	rows, err := a.db.Query(`SELECT l.id, b.title, l.due_date, s.name, s.parent_id, s.user_id FROM loans l JOIN books b ON b.id=l.book_id
		JOIN students s ON s.id=l.student_id WHERE l.returned_at='' AND l.due_date<? AND l.last_reminded<>?`, t, t)
	if err != nil {
		serverError(w, err)
		return
	}
	type od struct {
		id                  int64
		title, due, student string
		parent, user        sql.NullInt64
	}
	var list []od
	for rows.Next() {
		var x od
		if rows.Scan(&x.id, &x.title, &x.due, &x.student, &x.parent, &x.user) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		body := fmt.Sprintf("Dear Parent, the library book \"%s\" borrowed by %s was due on %s. Please return it to the library.", x.title, x.student, prettyDate(x.due))
		a.notify.Notify("library", "Library book overdue", body, []int64{x.parent.Int64, x.user.Int64}, a.notify.channelsFor("library"))
		a.db.Exec(`UPDATE loans SET last_reminded=? WHERE id=?`, t, x.id)
	}
	writeJSON(w, 200, map[string]int{"reminded": len(list)})
}
