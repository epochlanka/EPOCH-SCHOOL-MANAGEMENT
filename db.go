package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE COLLATE NOCASE,
	phone TEXT NOT NULL DEFAULT '',
	whatsapp TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL CHECK(role IN ('admin','teacher','parent','student')),
	password_hash TEXT NOT NULL,
	language TEXT NOT NULL DEFAULT 'English',
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sessions(
	token TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS classes(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	section TEXT NOT NULL DEFAULT '',
	teacher_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(name, section)
);
CREATE TABLE IF NOT EXISTS routes(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	bus_no TEXT NOT NULL DEFAULT '',
	driver_name TEXT NOT NULL DEFAULT '',
	driver_phone TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS students(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	admission_no TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	class_id INTEGER REFERENCES classes(id) ON DELETE SET NULL,
	parent_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	route_id INTEGER REFERENCES routes(id) ON DELETE SET NULL,
	gender TEXT NOT NULL DEFAULT '',
	dob TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS attendance(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
	date TEXT NOT NULL,
	status TEXT NOT NULL CHECK(status IN ('present','absent','late')),
	marked_by INTEGER,
	created_at TEXT NOT NULL DEFAULT '',
	UNIQUE(student_id, date)
);
CREATE TABLE IF NOT EXISTS fees(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	amount REAL NOT NULL,
	due_date TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'unpaid' CHECK(status IN ('unpaid','paid')),
	paid_at TEXT NOT NULL DEFAULT '',
	last_reminded TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS homework(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
	subject TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	due_date TEXT NOT NULL,
	teacher_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS exams(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
	subject TEXT NOT NULL,
	title TEXT NOT NULL,
	exam_date TEXT NOT NULL,
	exam_time TEXT NOT NULL DEFAULT '',
	max_marks REAL NOT NULL DEFAULT 100,
	published INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS results(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	exam_id INTEGER NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
	student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
	marks REAL NOT NULL,
	remarks TEXT NOT NULL DEFAULT '',
	UNIQUE(exam_id, student_id)
);
CREATE TABLE IF NOT EXISTS bus_updates(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	route_id INTEGER NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
	message TEXT NOT NULL,
	created_by INTEGER,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS announcements(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	body TEXT NOT NULL,
	audience TEXT NOT NULL,
	audience_id INTEGER,
	channels TEXT NOT NULL,
	recipients INTEGER NOT NULL DEFAULT 0,
	created_by INTEGER,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS notifications(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	category TEXT NOT NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL,
	is_read INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS deliveries(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	channel TEXT NOT NULL,
	recipient TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT '',
	sent_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS messages(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	sender_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	body TEXT NOT NULL,
	is_read INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY, value TEXT NOT NULL);

CREATE INDEX IF NOT EXISTS idx_att_date ON attendance(date);
CREATE INDEX IF NOT EXISTS idx_students_class ON students(class_id);
CREATE INDEX IF NOT EXISTS idx_students_parent ON students(parent_id);
CREATE INDEX IF NOT EXISTS idx_fees_student ON fees(student_id);
CREATE INDEX IF NOT EXISTS idx_notif_user ON notifications(user_id, is_read);
CREATE INDEX IF NOT EXISTS idx_deliv_created ON deliveries(created_at);
CREATE INDEX IF NOT EXISTS idx_msg_pair ON messages(sender_id, recipient_id);
`

var defaultSettings = map[string]string{
	"school_name":         "Epoch International School",
	"school_phone":        "076 843 7970",
	"school_email":        "epochadminlk@gmail.com",
	"school_address":      "Gongawela Bus Station Complex, Matale",
	"notify_present":      "0",
	"channels_attendance": "app,sms,whatsapp",
	"channels_fees":       "app,sms,whatsapp",
	"channels_homework":   "app",
	"channels_exams":      "app,whatsapp",
	"channels_results":    "app,whatsapp",
	"channels_transport":  "app,sms",
	"channels_messages":   "app",
	"fee_reminder_days":   "3",
	"fee_reminder_hour":   "9",
	"fee_last_run":        "",
}

func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for k, v := range defaultSettings {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)`, k, v); err != nil {
			return nil, err
		}
	}
	return db, nil
}

func (a *App) setting(key string) string {
	var v string
	if err := a.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return defaultSettings[key]
	}
	return v
}

func (a *App) setSetting(key, value string) error {
	_, err := a.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// seed creates the first admin and, when SEED_DEMO=true, a realistic demo school.
func (a *App) seed() error {
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n > 0 {
		return err
	}
	hash, err := hashPassword(a.cfg.AdminPassword)
	if err != nil {
		return err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	addUser := func(name, email, phone, role string) int64 {
		res, err := tx.Exec(`INSERT INTO users(name,email,phone,whatsapp,role,password_hash,created_at) VALUES(?,?,?,?,?,?,?)`,
			name, email, phone, phone, role, hash, now())
		if err != nil {
			log.Fatalf("seed user %s: %v", email, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	addUser("School Administrator", a.cfg.AdminEmail, "0768437970", "admin")
	if !a.cfg.SeedDemo {
		log.Printf("Created admin account %s", a.cfg.AdminEmail)
		return tx.Commit()
	}

	t1 := addUser("Nirmala Perera", "nirmala@epoch.lk", "0771000001", "teacher")
	t2 := addUser("Kasun Bandara", "kasun@epoch.lk", "0771000002", "teacher")
	t3 := addUser("Fathima Rizna", "rizna@epoch.lk", "0771000003", "teacher")

	exec := func(q string, args ...any) int64 {
		res, err := tx.Exec(q, args...)
		if err != nil {
			log.Fatalf("seed: %v (%s)", err, q)
		}
		id, _ := res.LastInsertId()
		return id
	}
	c6 := exec(`INSERT INTO classes(name,section,teacher_id) VALUES('Grade 6','A',?)`, t1)
	c7 := exec(`INSERT INTO classes(name,section,teacher_id) VALUES('Grade 7','B',?)`, t2)
	c8 := exec(`INSERT INTO classes(name,section,teacher_id) VALUES('Grade 8','A',?)`, t3)
	r1 := exec(`INSERT INTO routes(name,bus_no,driver_name,driver_phone) VALUES('Route 1 – Matale Town / Gongawela','NC-4512','Sampath Kumara','0772000001')`)
	r2 := exec(`INSERT INTO routes(name,bus_no,driver_name,driver_phone) VALUES('Route 2 – Ukuwela / Matale','ND-7731','Lalith Silva','0772000002')`)

	type kid struct {
		student, parent, email, phone, gender string
		class, route                          int64
	}
	kids := []kid{
		{"Senuli Jayasinghe", "Sunil Jayasinghe", "sunil@example.lk", "0773000001", "F", c6, r1},
		{"Ahamed Rifkhan", "Mohamed Rifkhan", "rifkhan@example.lk", "0773000002", "M", c6, r2},
		{"Tharindu Herath", "Kumari Herath", "kumari@example.lk", "0773000003", "M", c6, r1},
		{"Dinithi Dissanayake", "Ranjith Dissanayake", "ranjith@example.lk", "0773000004", "F", c7, r2},
		{"Kavindu Rajapaksha", "Shalini Rajapaksha", "shalini@example.lk", "0773000005", "M", c7, r1},
		{"Nethmi Wickramasinghe", "Anura Wickramasinghe", "anura@example.lk", "0773000006", "F", c7, 0},
		{"Yasiru Gunawardena", "Chamari Gunawardena", "chamari@example.lk", "0773000007", "M", c8, r2},
		{"Hiruni Senanayake", "Pradeep Senanayake", "pradeep@example.lk", "0773000008", "F", c8, r1},
		{"Aarav Sharma", "Ravi Sharma", "parent@epoch.lk", "0773000009", "M", c8, r1},
	}
	var studentIDs []int64
	for i, k := range kids {
		pid := addUser(k.parent, k.email, k.phone, "parent")
		var uid any
		if k.student == "Aarav Sharma" {
			uid = addUser(k.student, "student@epoch.lk", "", "student")
		}
		sid := exec(`INSERT INTO students(admission_no,name,class_id,parent_id,user_id,route_id,gender,created_at) VALUES(?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("EP%04d", 1001+i), k.student, k.class, pid, uid, nullID(k.route), k.gender, now())
		studentIDs = append(studentIDs, sid)
	}

	// Attendance for the previous 10 school days.
	d := time.Now()
	for days := 0; days < 10; {
		d = d.AddDate(0, 0, -1)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		days++
		for i, sid := range studentIDs {
			status := "present"
			if (i*7+days*3)%17 == 0 {
				status = "absent"
			} else if (i+days)%11 == 0 {
				status = "late"
			}
			exec(`INSERT INTO attendance(student_id,date,status,marked_by,created_at) VALUES(?,?,?,?,?)`, sid, d.Format(dateLayout), status, t1, now())
		}
	}

	due := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	overdue := time.Now().AddDate(0, 0, -5).Format(dateLayout)
	for i, sid := range studentIDs {
		exec(`INSERT INTO fees(student_id,title,amount,due_date,created_at) VALUES(?,?,?,?,?)`, sid, "Term 3 Facility Fee", 12000, due, now())
		if i%3 == 0 {
			exec(`INSERT INTO fees(student_id,title,amount,due_date,created_at) VALUES(?,?,?,?,?)`, sid, "Sports Meet Contribution", 2500, overdue, now())
		}
		if i%2 == 0 {
			exec(`INSERT INTO fees(student_id,title,amount,due_date,status,paid_at,created_at) VALUES(?,?,?,?,'paid',?,?)`, sid, "Term 2 Facility Fee", 12000, time.Now().AddDate(0, -2, 0).Format(dateLayout), now(), now())
		}
	}

	in := func(days int) string { return time.Now().AddDate(0, 0, days).Format(dateLayout) }
	exec(`INSERT INTO homework(class_id,subject,title,description,due_date,teacher_id,created_at) VALUES(?,?,?,?,?,?,?)`, c8, "Mathematics", "Complete Exercise 5.2", "Questions 1–15 on page 84. Show all working.", in(1), t3, now())
	exec(`INSERT INTO homework(class_id,subject,title,description,due_date,teacher_id,created_at) VALUES(?,?,?,?,?,?,?)`, c8, "English", "Essay: My Village", "Write a 300-word essay describing your village.", in(3), t3, now())
	exec(`INSERT INTO homework(class_id,subject,title,description,due_date,teacher_id,created_at) VALUES(?,?,?,?,?,?,?)`, c6, "Science", "Plant Cell Diagram", "Draw and label a plant cell.", in(2), t1, now())
	exec(`INSERT INTO homework(class_id,subject,title,description,due_date,teacher_id,created_at) VALUES(?,?,?,?,?,?,?)`, c7, "Sinhala", "Poem recitation", "Memorise the poem on page 22.", in(4), t2, now())

	exec(`INSERT INTO exams(class_id,subject,title,exam_date,exam_time,max_marks,created_at) VALUES(?,?,?,?,?,?,?)`, c8, "Science", "Unit Test", in(6), "10:00", 50, now())
	e2 := exec(`INSERT INTO exams(class_id,subject,title,exam_date,exam_time,max_marks,published,created_at) VALUES(?,?,?,?,?,?,1,?)`, c8, "Mathematics", "Second Term Test", in(-20), "08:30", 100, now())
	exec(`INSERT INTO exams(class_id,subject,title,exam_date,exam_time,max_marks,created_at) VALUES(?,?,?,?,?,?,?)`, c6, "English", "Monthly Test", in(9), "09:00", 50, now())
	for i, sid := range studentIDs[6:] {
		exec(`INSERT INTO results(exam_id,student_id,marks,remarks) VALUES(?,?,?,?)`, e2, sid, 62+i*11, "Good effort")
	}

	exec(`INSERT INTO bus_updates(route_id,message,created_by,created_at) VALUES(?,?,?,?)`, r1, "Bus NC-4512 is running 10 mins late. Current location: near Matale Clock Tower.", t1, now())
	exec(`INSERT INTO announcements(title,body,audience,channels,recipients,created_by,created_at) VALUES(?,?,?,?,?,?,?)`,
		"School closed for Annual Function", "School will remain closed on Friday on account of the Annual Function. Parents are warmly invited to attend at 3.00 pm.", "parents", "app,sms", len(kids), 1, now())

	log.Printf("Seeded demo school data (admin login: %s)", a.cfg.AdminEmail)
	return tx.Commit()
}
