package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
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
	role TEXT NOT NULL,
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
	section_id INTEGER REFERENCES sections(id) ON DELETE SET NULL,
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
CREATE TABLE IF NOT EXISTS sections(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	head_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	sort INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS subjects(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	code TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS class_subjects(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
	subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
	teacher_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	periods_per_week INTEGER NOT NULL DEFAULT 1,
	UNIQUE(class_id, subject_id)
);
CREATE TABLE IF NOT EXISTS timetable(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
	day INTEGER NOT NULL,
	period INTEGER NOT NULL,
	subject_id INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
	teacher_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	UNIQUE(class_id, day, period)
);
CREATE TABLE IF NOT EXISTS staff_attendance(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	date TEXT NOT NULL,
	status TEXT NOT NULL,
	check_in TEXT NOT NULL DEFAULT '',
	check_out TEXT NOT NULL DEFAULT '',
	marked_by INTEGER,
	UNIQUE(user_id, date)
);
CREATE TABLE IF NOT EXISTS leave_requests(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	leave_type TEXT NOT NULL,
	from_date TEXT NOT NULL,
	to_date TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	reviewed_by INTEGER,
	review_note TEXT NOT NULL DEFAULT '',
	reviewed_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS substitutions(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	date TEXT NOT NULL,
	class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
	period INTEGER NOT NULL,
	subject_id INTEGER,
	absent_teacher_id INTEGER,
	substitute_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT '',
	UNIQUE(date, class_id, period)
);
CREATE TABLE IF NOT EXISTS admissions(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	child_name TEXT NOT NULL,
	dob TEXT NOT NULL DEFAULT '',
	gender TEXT NOT NULL DEFAULT '',
	grade_applying TEXT NOT NULL DEFAULT '',
	parent_name TEXT NOT NULL,
	phone TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	previous_school TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'enquiry',
	follow_up TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	student_id INTEGER REFERENCES students(id) ON DELETE SET NULL,
	created_by INTEGER,
	created_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS books(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	author TEXT NOT NULL DEFAULT '',
	isbn TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	copies INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS loans(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
	issued_at TEXT NOT NULL,
	due_date TEXT NOT NULL,
	returned_at TEXT NOT NULL DEFAULT '',
	last_reminded TEXT NOT NULL DEFAULT '',
	issued_by INTEGER
);
CREATE INDEX IF NOT EXISTS idx_tt_teacher ON timetable(teacher_id, day, period);
CREATE INDEX IF NOT EXISTS idx_staff_att ON staff_attendance(date);
CREATE INDEX IF NOT EXISTS idx_subs_date ON substitutions(date);
CREATE INDEX IF NOT EXISTS idx_leave_user ON leave_requests(user_id, status);
CREATE INDEX IF NOT EXISTS idx_loans_open ON loans(returned_at, due_date);

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
	"school_days":         "5",
	"periods_per_day":     "8",
	"day_start":           "07:40",
	"period_minutes":      "40",
	"break_after":         "4",
	"break_minutes":       "20",
	"staff_late_after":    "07:30",
	"channels_staff":      "app,whatsapp",
	"channels_library":    "app",
}

func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	if err := upgradeSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade: %w", err)
	}
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

// upgradeSchema brings databases from earlier versions up to date before the
// schema runs: roles are no longer restricted by a CHECK constraint (the old
// "teacher" role becomes class or subject teacher) and classes gain a section.
func upgradeSchema(db *sql.DB) error {
	var usersSQL string
	db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&usersSQL)
	if strings.Contains(usersSQL, "CHECK(role IN") {
		ctx := context.Background()
		conn, err := db.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		defer conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		cols := `id,name,email,phone,whatsapp,role,password_hash,language,active,created_at`
		stmts := []string{
			`CREATE TABLE users_new(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE COLLATE NOCASE,
				phone TEXT NOT NULL DEFAULT '', whatsapp TEXT NOT NULL DEFAULT '', role TEXT NOT NULL, password_hash TEXT NOT NULL,
				language TEXT NOT NULL DEFAULT 'English', active INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL DEFAULT '')`,
			`INSERT INTO users_new(` + cols + `) SELECT ` + cols + ` FROM users`,
			`UPDATE users_new SET role = CASE WHEN EXISTS(SELECT 1 FROM classes c WHERE c.teacher_id=users_new.id) THEN 'class_teacher' ELSE 'subject_teacher' END WHERE role='teacher'`,
			`DROP TABLE users`,
			`ALTER TABLE users_new RENAME TO users`,
		}
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("upgrade users: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("Upgraded user roles (teacher -> class/subject teacher)")
	}
	var classesSQL string
	db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='classes'`).Scan(&classesSQL)
	if classesSQL != "" && !strings.Contains(classesSQL, "section_id") {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sections(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE,
			head_id INTEGER REFERENCES users(id) ON DELETE SET NULL, sort INTEGER NOT NULL DEFAULT 0)`); err != nil {
			return err
		}
		if _, err := db.Exec(`ALTER TABLE classes ADD COLUMN section_id INTEGER REFERENCES sections(id) ON DELETE SET NULL`); err != nil {
			return fmt.Errorf("add classes.section_id: %w", err)
		}
	}
	return nil
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

	staff := func(name, email, phone, role string) int64 { return addUser(name, email, phone, role) }
	staff("Lalith Wijesinghe", "director@epoch.lk", "0771000010", "director")
	principal := staff("Dilrukshi Fernando", "principal@epoch.lk", "0771000011", "principal")
	staff("Asanka Rathnayake", "viceprincipal@epoch.lk", "0771000012", "vice_principal")
	headPrimary := staff("Sanduni Perera", "primaryhead@epoch.lk", "0771000013", "section_head")
	headMiddle := staff("Mahesh Kumara", "middlehead@epoch.lk", "0771000014", "section_head")
	t1 := staff("Nirmala Perera", "nirmala@epoch.lk", "0771000001", "class_teacher")
	t2 := staff("Kasun Bandara", "kasun@epoch.lk", "0771000002", "class_teacher")
	t3 := staff("Fathima Rizna", "rizna@epoch.lk", "0771000003", "class_teacher")
	t5 := staff("Tharushi Silva", "tharushi@epoch.lk", "0771000004", "class_teacher")
	ruwan := staff("Ruwan Jayasuriya", "ruwan@epoch.lk", "0771000021", "subject_teacher")
	nadeesha := staff("Nadeesha Karunaratne", "nadeesha@epoch.lk", "0771000022", "subject_teacher")
	imran := staff("Imran Faleel", "imran@epoch.lk", "0771000023", "subject_teacher")
	chamila := staff("Chamila Weerasinghe", "chamila@epoch.lk", "0771000024", "subject_teacher")
	staff("Priyanka Jayawardena", "accountant@epoch.lk", "0771000031", "accountant")
	staff("Hasini Abeysekara", "admissions@epoch.lk", "0771000032", "admissions")
	staff("Malith Gamage", "frontoffice@epoch.lk", "0771000033", "front_office")
	librarian := staff("Kumudu Senanayake", "librarian@epoch.lk", "0771000034", "librarian")

	exec := func(q string, args ...any) int64 {
		res, err := tx.Exec(q, args...)
		if err != nil {
			log.Fatalf("seed: %v (%s)", err, q)
		}
		id, _ := res.LastInsertId()
		return id
	}
	secPrimary := exec(`INSERT INTO sections(name,head_id,sort) VALUES('Primary School',?,1)`, headPrimary)
	secMiddle := exec(`INSERT INTO sections(name,head_id,sort) VALUES('Middle School',?,2)`, headMiddle)
	exec(`INSERT INTO sections(name,sort) VALUES('Upper School',3)`)
	c5 := exec(`INSERT INTO classes(name,section,teacher_id,section_id) VALUES('Grade 5','A',?,?)`, t5, secPrimary)
	c6 := exec(`INSERT INTO classes(name,section,teacher_id,section_id) VALUES('Grade 6','A',?,?)`, t1, secMiddle)
	c7 := exec(`INSERT INTO classes(name,section,teacher_id,section_id) VALUES('Grade 7','B',?,?)`, t2, secMiddle)
	c8 := exec(`INSERT INTO classes(name,section,teacher_id,section_id) VALUES('Grade 8','A',?,?)`, t3, secMiddle)
	r1 := exec(`INSERT INTO routes(name,bus_no,driver_name,driver_phone) VALUES('Route 1 – Matale Town / Gongawela','NC-4512','Sampath Kumara','0772000001')`)
	r2 := exec(`INSERT INTO routes(name,bus_no,driver_name,driver_phone) VALUES('Route 2 – Ukuwela / Matale','ND-7731','Lalith Silva','0772000002')`)

	// Subjects and who teaches them in each class (periods per week).
	subj := map[string]int64{}
	for _, n := range []string{"Mathematics", "English", "Science", "Sinhala", "ICT", "History", "Religion", "Health & PE", "Art", "Tamil"} {
		subj[n] = exec(`INSERT INTO subjects(name,code) VALUES(?,?)`, n, strings.ToUpper(n[:3]))
	}
	classTeacher := map[int64]int64{c5: t5, c6: t1, c7: t2, c8: t3}
	for _, c := range []int64{c5, c6, c7, c8} {
		maths, science := ruwan, imran
		if c == c5 {
			maths, science = t5, t5
		}
		alloc := []struct {
			subject string
			teacher int64
			periods int
		}{
			{"Mathematics", maths, 6}, {"English", nadeesha, 6}, {"Science", science, 5}, {"Sinhala", chamila, 5},
			{"ICT", imran, 2}, {"History", classTeacher[c], 3}, {"Religion", classTeacher[c], 3},
			{"Health & PE", headMiddle, 3}, {"Art", headPrimary, 2}, {"Tamil", chamila, 1},
		}
		for _, x := range alloc {
			exec(`INSERT INTO class_subjects(class_id,subject_id,teacher_id,periods_per_week) VALUES(?,?,?,?)`, c, subj[x.subject], x.teacher, x.periods)
		}
	}

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

	// Teacher attendance for the previous school days.
	var teachers []int64
	rows, _ := tx.Query(`SELECT id FROM users WHERE role IN ` + teachingRolesSQL)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		teachers = append(teachers, id)
	}
	rows.Close()
	d = time.Now()
	for days := 0; days < 5; {
		d = d.AddDate(0, 0, -1)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		days++
		for i, t := range teachers {
			status, checkIn := "present", "07:2"+fmt.Sprint(i%10)
			if (i+days)%9 == 0 {
				status, checkIn = "late", "07:4"+fmt.Sprint(i%10)
			}
			exec(`INSERT INTO staff_attendance(user_id,date,status,check_in,check_out,marked_by) VALUES(?,?,?,?,?,?)`, t, d.Format(dateLayout), status, checkIn, "13:45", principal)
		}
	}
	// Leave: an approved leave today (relief teachers get assigned below) and a pending one.
	exec(`INSERT INTO leave_requests(user_id,leave_type,from_date,to_date,reason,status,reviewed_by,reviewed_at,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		ruwan, "medical", today(), today(), "Fever – doctor advised rest.", "approved", principal, now(), now())
	exec(`INSERT INTO leave_requests(user_id,leave_type,from_date,to_date,reason,status,created_at) VALUES(?,?,?,?,?,?,?)`,
		nadeesha, "casual", in(7), in(8), "Family wedding in Kandy.", "pending", now())

	// Admissions pipeline.
	for _, ad := range [][]string{
		{"Kaveesha Bandara", "Grade 1", "Nuwan Bandara", "0774000001", "enquiry", in(2)},
		{"Mohamed Aathif", "Grade 6", "Fazeel Mohamed", "0774000002", "test_scheduled", in(4)},
		{"Sithumi Perera", "Grade 3", "Dinesh Perera", "0774000003", "offered", in(1)},
		{"Yohan Fernando", "Grade 8", "Shiromi Fernando", "0774000004", "enquiry", ""},
	} {
		exec(`INSERT INTO admissions(child_name,grade_applying,parent_name,phone,status,follow_up,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
			ad[0], ad[1], ad[2], ad[3], ad[4], ad[5], now(), now())
	}

	// Library.
	b1 := exec(`INSERT INTO books(title,author,isbn,category,copies,created_at) VALUES('Madol Doova','Martin Wickramasinghe','9789552100','Sinhala Literature',3,?)`, now())
	b2 := exec(`INSERT INTO books(title,author,isbn,category,copies,created_at) VALUES('Charlotte''s Web','E. B. White','9780064400558','English Fiction',2,?)`, now())
	exec(`INSERT INTO books(title,author,isbn,category,copies,created_at) VALUES('Oxford Student Atlas','Oxford','9780198321620','Reference',5,?)`, now())
	exec(`INSERT INTO books(title,author,isbn,category,copies,created_at) VALUES('Grade 8 Science Workbook','Educational Publications','','Textbooks',10,?)`, now())
	exec(`INSERT INTO loans(book_id,student_id,issued_at,due_date,issued_by) VALUES(?,?,?,?,?)`, b1, studentIDs[8], in(-20), in(-6), librarian)
	exec(`INSERT INTO loans(book_id,student_id,issued_at,due_date,issued_by) VALUES(?,?,?,?,?)`, b2, studentIDs[0], in(-3), in(11), librarian)
	_ = c5

	log.Printf("Seeded demo school data (admin login: %s)", a.cfg.AdminEmail)
	if err := tx.Commit(); err != nil {
		return err
	}
	if res, err := a.generateTimetable(nil); err != nil {
		log.Printf("demo timetable: %v", err)
	} else {
		log.Printf("Demo timetable generated: %d periods placed, %d not placed", res.Placed, len(res.Unplaced))
	}
	a.assignSubstitutions(today())
	return nil
}
