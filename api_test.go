package main

// End-to-end test against a real PostgreSQL database: it seeds the demo school in a
// throwaway schema, then signs in as each role and calls every API endpoint and the
// main workflows. Set TEST_DATABASE_URL to run it, e.g.
//
//	TEST_DATABASE_URL=postgres:///epoch_test go test ./...

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := openDB(url + sep + "search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		admin.Close()
	})
	cfg := Config{AdminEmail: "admin@epoch.lk", AdminPassword: "epoch123", SeedDemo: true, AIModel: "test",
		DatabaseURL: url, SMSProvider: "log", WAProvider: "log", CountryCode: "94"}
	store := &storage{file: filepath.Join(t.TempDir(), "config.json"), cfg: storeConfig{BackupKeep: 14}}
	app := &App{db: db, cfg: cfg, limiter: newLoginLimiter(), store: store}
	app.notify = newNotifier(app)
	app.ai = newAI(app)
	if err := app.seed(); err != nil {
		t.Fatal(err)
	}
	return app
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base, email string) *client {
	jar, _ := cookiejar.New(nil)
	c := &client{t, base, &http.Client{Jar: jar}}
	if code, body := c.do("POST", "/api/login", map[string]string{"email": email, "password": "epoch123"}); code != 200 {
		t.Fatalf("login %s: %d %s", email, code, body)
	}
	return c
}

// do sends a request and fails the test on any server error.
func (c *client) do(method, path string, body any) (int, string) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil || method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 500 {
		c.t.Errorf("%s %s -> %d %s", method, path, res.StatusCode, b)
	}
	return res.StatusCode, string(b)
}

// ok expects a 2xx response.
func (c *client) ok(method, path string, body any) string {
	c.t.Helper()
	code, b := c.do(method, path, body)
	if code < 200 || code > 299 {
		c.t.Errorf("%s %s -> %d %s", method, path, code, b)
	}
	return b
}

func (a *App) id(t *testing.T, q string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := a.db.QueryRow(q, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return id
}

func TestAPIOnPostgres(t *testing.T) {
	app := testApp(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	class8 := app.id(t, `SELECT id FROM classes WHERE name='Grade 8'`)
	aarav := app.id(t, `SELECT id FROM students WHERE admission_no='EP1009'`)
	parent := app.id(t, `SELECT id FROM users WHERE email='parent@epoch.lk'`)
	rizna := app.id(t, `SELECT id FROM users WHERE email='rizna@epoch.lk'`)
	exam := app.id(t, `SELECT id FROM exams WHERE published=1 ORDER BY id LIMIT 1`)
	route := app.id(t, `SELECT id FROM routes ORDER BY id LIMIT 1`)
	today := today()

	// Every read endpoint, as the admin and as the principal.
	for _, email := range []string{"admin@epoch.lk", "principal@epoch.lk"} {
		c := newClient(t, srv.URL, email)
		for _, p := range []string{
			"/api/me", "/api/dashboard", "/api/users", "/api/users?role=parent", "/api/staff", "/api/sections", "/api/classes",
			"/api/classes?scope=attendance", "/api/routes", "/api/students", "/api/students?q=aar&class_id=" + fmt.Sprint(class8),
			"/api/settings", "/api/subjects", "/api/class-subjects", "/api/class-subjects?class_id=" + fmt.Sprint(class8),
			"/api/timetable?class_id=" + fmt.Sprint(class8), "/api/timetable?teacher_id=" + fmt.Sprint(rizna), "/api/workspace",
			"/api/staff-attendance", "/api/leave", "/api/leave?status=pending", "/api/substitutions",
			"/api/substitutions/free-teachers?date=" + today + "&period=3", "/api/pickups", "/api/progress/" + fmt.Sprint(aarav),
			"/api/early-leaves", "/api/admissions", "/api/admissions?q=ka", "/api/books", "/api/books?q=web", "/api/loans",
			"/api/loans?status=overdue", "/api/attendance?class_id=" + fmt.Sprint(class8), "/api/attendance/report",
			"/api/fees", "/api/fees?status=overdue", "/api/homework", "/api/exams", "/api/exams/" + fmt.Sprint(exam) + "/results",
			"/api/bus-updates", "/api/announcements", "/api/notifications", "/api/messages", "/api/messages/contacts",
			"/api/messages/thread/" + fmt.Sprint(parent), "/api/reports/communications", "/api/portal/" + fmt.Sprint(aarav),
			"/api/ai/status", "/api/push/key",
		} {
			c.ok("GET", p, nil)
		}
	}
	admin := newClient(t, srv.URL, "admin@epoch.lk")
	admin.ok("GET", "/api/system/storage", nil)

	// Every role can load its own start data.
	for _, email := range []string{"director@epoch.lk", "viceprincipal@epoch.lk", "middlehead@epoch.lk", "rizna@epoch.lk",
		"ruwan@epoch.lk", "accountant@epoch.lk", "admissions@epoch.lk", "frontoffice@epoch.lk", "librarian@epoch.lk",
		"parent@epoch.lk", "student@epoch.lk", "rifkhan@example.lk"} {
		c := newClient(t, srv.URL, email)
		c.ok("GET", "/api/me", nil)
		c.do("GET", "/api/dashboard", nil)
		c.ok("GET", "/api/notifications", nil)
		c.ok("GET", "/api/messages/contacts", nil)
	}
	// Email sign-in ignores letter case.
	newClient(t, srv.URL, "Principal@EPOCH.lk")

	// ---- Workflows ----
	teacher := newClient(t, srv.URL, "rizna@epoch.lk") // class teacher of Grade 8 - A
	var att struct {
		Students []map[string]any `json:"students"`
	}
	json.Unmarshal([]byte(teacher.ok("GET", "/api/attendance?class_id="+fmt.Sprint(class8), nil)), &att)
	var recs []map[string]any
	for i, s := range att.Students {
		st := "present"
		if i == 0 {
			st = "absent"
		}
		recs = append(recs, map[string]any{"student_id": s["student_id"], "status": st})
	}
	teacher.ok("POST", "/api/attendance", map[string]any{"class_id": class8, "date": today, "records": recs})
	teacher.ok("POST", "/api/homework", map[string]any{"class_id": class8, "subject": "Science", "title": "Read ch 4", "due_date": today, "notify": true})
	teacher.ok("POST", "/api/exams", map[string]any{"class_id": class8, "subject": "ICT", "title": "Quiz", "exam_date": today, "max_marks": 20, "notify": true})
	quiz := app.id(t, `SELECT id FROM exams WHERE title='Quiz'`)
	teacher.ok("POST", fmt.Sprintf("/api/exams/%d/results", quiz), map[string]any{"publish": true,
		"records": []map[string]any{{"student_id": aarav, "marks": 17.5, "remarks": "Good"}}})
	teacher.ok("POST", "/api/announcements", map[string]any{"title": "Trip", "body": "Bring lunch", "audience": "class", "audience_id": class8, "channels": []string{"app"}})
	teacher.ok("POST", "/api/staff-attendance/check-in", map[string]string{"action": "in"})
	teacher.ok("POST", "/api/staff-attendance/check-in", map[string]string{"action": "out"})
	teacher.ok("POST", "/api/leave", map[string]string{"leave_type": "casual", "from_date": time.Now().AddDate(0, 0, 14).Format(dateLayout), "reason": "Wedding"})
	leave := app.id(t, `SELECT id FROM leave_requests WHERE user_id=? AND status='pending'`, rizna)

	principal := newClient(t, srv.URL, "principal@epoch.lk")
	principal.ok("POST", fmt.Sprintf("/api/leave/%d/decision", leave), map[string]string{"status": "approved"})
	principal.ok("POST", "/api/substitutions/auto", map[string]string{"date": today})
	principal.ok("POST", "/api/staff-attendance", map[string]any{"date": today, "records": []map[string]any{{"user_id": rizna, "status": "absent"}}})
	if sub, err := app.queryOne(`SELECT id FROM substitutions WHERE date=? ORDER BY id LIMIT 1`, today); err == nil {
		principal.do("PUT", fmt.Sprintf("/api/substitutions/%d", sub["id"]), map[string]any{"substitute_id": 0})
	}
	principal.ok("POST", "/api/timetable/generate", map[string]any{"class_ids": []int64{class8}})
	subject := app.id(t, `SELECT id FROM subjects WHERE name='Art'`)
	principal.ok("PUT", "/api/timetable/slot", map[string]any{"class_id": class8, "day": 1, "period": 1, "subject_id": subject})
	principal.ok("POST", "/api/class-subjects", map[string]any{"class_id": class8, "subject_id": subject, "teacher_id": rizna, "periods_per_week": 2})
	principal.ok("POST", "/api/subjects", map[string]string{"name": "Music", "code": "MUS"})
	if code, _ := principal.do("POST", "/api/subjects", map[string]string{"name": "music"}); code != 409 {
		t.Errorf("duplicate subject (any case) should be 409, got %d", code)
	}
	principal.ok("POST", "/api/sections", map[string]any{"name": "Advanced Level", "sort": 4})
	principal.ok("POST", "/api/classes", map[string]any{"name": "Grade 9", "section": "A", "teacher_id": rizna})
	if code, _ := principal.do("POST", "/api/classes", map[string]any{"name": "Grade 9", "section": "A"}); code != 409 {
		t.Errorf("duplicate class should be 409, got %d", code)
	}
	principal.ok("POST", "/api/routes", map[string]string{"name": "Route 3"})
	principal.ok("POST", fmt.Sprintf("/api/routes/%d/updates", route), map[string]string{"message": "Running late"})
	principal.ok("POST", "/api/students", map[string]any{"name": "New Kid", "class_id": class8, "parent_email": "newparent@example.lk",
		"parent_name": "New Parent", "parent_password": "secret12"})
	principal.ok("POST", "/api/announcements", map[string]any{"title": "Holiday", "body": "Closed", "audience": "all", "channels": []string{"app"}})

	acc := newClient(t, srv.URL, "accountant@epoch.lk")
	acc.ok("POST", "/api/fees", map[string]any{"class_id": class8, "title": "Lab fee", "amount": 1500.5, "due_date": today, "notify": true})
	fee := app.id(t, `SELECT id FROM fees WHERE title='Lab fee' ORDER BY id LIMIT 1`)
	acc.ok("POST", fmt.Sprintf("/api/fees/%d/remind", fee), nil)
	acc.ok("POST", fmt.Sprintf("/api/fees/%d/pay", fee), nil)
	acc.ok("POST", "/api/fees/remind-due", nil)

	par := newClient(t, srv.URL, "parent@epoch.lk")
	par.ok("POST", "/api/pickups", map[string]any{"student_id": aarav, "date": today, "kind": "collector", "collector_name": "Uncle Tom",
		"collector_relation": "Uncle", "collector_phone": "0771234567"})
	par.ok("POST", "/api/messages", map[string]any{"to": rizna, "body": "Hello teacher"})
	par.ok("GET", "/api/messages", nil)
	par.ok("GET", fmt.Sprintf("/api/messages/thread/%d", rizna), nil)
	par.ok("POST", "/api/notifications/read-all", nil)
	par.ok("PUT", "/api/me/language", map[string]string{"language": "Tamil"})
	pickup := app.id(t, `SELECT id FROM pickup_requests WHERE collector_name='Uncle Tom'`)

	front := newClient(t, srv.URL, "frontoffice@epoch.lk")
	front.ok("POST", fmt.Sprintf("/api/pickups/%d/respond", pickup), map[string]string{"status": "acknowledged"})
	front.ok("POST", "/api/early-leaves", map[string]any{"student_id": aarav, "collector_name": "Kamal Sharma", "collector_relation": "grandparent",
		"collector_id_no": "195512345678", "reason_type": "bereavement", "reason": "Funeral", "time_out": "10:30"})
	early := app.id(t, `SELECT id FROM early_leaves ORDER BY id DESC LIMIT 1`)
	front.ok("POST", fmt.Sprintf("/api/early-leaves/%d/return", early), map[string]string{"time": "13:00"})
	principal.ok("POST", fmt.Sprintf("/api/early-leaves/%d/void", early), map[string]string{"reason": "Entered twice by mistake"})

	adm := newClient(t, srv.URL, "admissions@epoch.lk")
	adm.ok("POST", "/api/admissions", map[string]string{"child_name": "Test Child", "parent_name": "Test Parent", "phone": "0770000000", "status": "offered"})
	admission := app.id(t, `SELECT id FROM admissions WHERE child_name='Test Child'`)
	adm.ok("POST", fmt.Sprintf("/api/admissions/%d/enrol", admission), map[string]any{"class_id": class8, "parent_email": "TestParent@example.lk", "parent_password": "secret12"})

	lib := newClient(t, srv.URL, "librarian@epoch.lk")
	lib.ok("POST", "/api/books", map[string]any{"title": "New Book", "copies": 2})
	book := app.id(t, `SELECT id FROM books WHERE title='New Book'`)
	lib.ok("POST", "/api/loans", map[string]any{"book_id": book, "student_id": aarav, "due_date": today})
	loan := app.id(t, `SELECT id FROM loans WHERE book_id=?`, book)
	lib.ok("POST", fmt.Sprintf("/api/loans/%d/return", loan), nil)
	lib.ok("POST", "/api/loans/remind-overdue", nil)

	admin.ok("POST", "/api/users", map[string]string{"name": "Extra Teacher", "email": "extra@epoch.lk", "role": "subject_teacher", "password": "secret12"})
	if code, _ := admin.do("POST", "/api/users", map[string]string{"name": "Dup", "email": "EXTRA@epoch.lk", "role": "parent", "password": "secret12"}); code != 409 {
		t.Errorf("duplicate email (any case) should be 409, got %d", code)
	}
	admin.ok("PUT", "/api/settings", map[string]string{"school_name": "Epoch Test School"})
	admin.ok("GET", "/api/reports/communications?channel=app", nil)

	// Re-read everything after the changes.
	for _, p := range []string{"/api/dashboard", "/api/attendance/report", "/api/exams", "/api/portal/" + fmt.Sprint(aarav),
		"/api/progress/" + fmt.Sprint(aarav), "/api/early-leaves", "/api/substitutions", "/api/leave", "/api/students"} {
		principal.ok("GET", p, nil)
	}

	// JSON numbers stay numbers (not strings) after the move to PostgreSQL.
	var classes []map[string]any
	json.Unmarshal([]byte(principal.ok("GET", "/api/classes", nil)), &classes)
	if _, ok := classes[0]["periods"].(float64); !ok {
		t.Errorf("classes.periods should be a number, got %T", classes[0]["periods"])
	}
	var portal map[string]any
	json.Unmarshal([]byte(par.ok("GET", "/api/portal/"+fmt.Sprint(aarav), nil)), &portal)
	for _, r := range portal["results"].([]any) {
		if v := r.(map[string]any)["class_average"]; v != nil {
			if _, ok := v.(float64); !ok {
				t.Errorf("class_average should be a number, got %T", v)
			}
		}
	}
}

// TestImportSQLite copies an SQLite database from an earlier version into a throwaway
// schema. Set TEST_SQLITE_FILE to the file (it is opened read-only).
func TestImportSQLite(t *testing.T) {
	file := os.Getenv("TEST_SQLITE_FILE")
	if file == "" {
		t.Skip("TEST_SQLITE_FILE not set")
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("import_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := openDB(url + sep + "search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	counts, err := importSQLite(db, file)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := sql.Open("sqlite", "file:"+file+"?mode=ro")
	defer src.Close()
	for _, table := range importTables {
		var want, got int
		if src.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&want) != nil {
			continue // table not in that version
		}
		db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got)
		if table != "settings" && got != want {
			t.Errorf("%s: %d rows in SQLite, %d imported", table, want, got)
		}
		t.Logf("%-20s %d", table, counts[table])
	}
	// New rows get fresh ids after the imported ones.
	if _, err := db.InsertID(`INSERT INTO routes(name) VALUES('after import')`); err != nil {
		t.Errorf("insert after import: %v", err)
	}
}
