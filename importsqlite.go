package main

// Import from SQLite: earlier versions stored everything in an SQLite file. On the first
// start with a brand-new PostgreSQL database that file is copied in automatically, and
// `epoch-school import-sqlite <file>` does the same by hand.

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

const legacyDBPath = "data/epoch.db" // where the oldest versions kept the database (inside the project folder)

// importTables lists every table in an order that satisfies foreign keys.
// Sessions are left out, so everyone signs in again after the move.
var importTables = []string{
	"users", "settings", "sections", "classes", "routes", "students", "attendance", "fees", "homework",
	"exams", "results", "bus_updates", "announcements", "notifications", "push_subscriptions", "deliveries",
	"messages", "subjects", "class_subjects", "timetable", "staff_attendance", "leave_requests",
	"substitutions", "admissions", "books", "loans", "pickup_requests", "progress_summaries", "early_leaves",
}

// legacySQLiteFile finds the SQLite database an earlier version was using, if any.
func legacySQLiteFile(cfg Config, store *storage) string {
	for _, p := range []string{cfg.DBPath, store.get().DBPath, filepath.Join(appDataDir(), "epoch.db"), legacyDBPath} {
		if p != "" && fileExists(p) {
			if abs, err := filepath.Abs(p); err == nil {
				return abs
			}
			return p
		}
	}
	return ""
}

// importSQLite copies all data from the SQLite file at path into db, which must have no users yet.
// It returns the number of rows copied per table.
func importSQLite(db *DB, path string) (map[string]int, error) {
	var users int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		return nil, err
	}
	if users > 0 {
		return nil, errors.New("the PostgreSQL database already has data; import only into an empty database")
	}
	src, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	defer src.Close()
	if err := src.Ping(); err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	counts := map[string]int{}
	for _, table := range importTables {
		n, err := copyTable(src, tx, table)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", table, err)
		}
		counts[table] = n
	}
	// Very old versions had a single "teacher" role.
	if _, err := tx.Exec(`UPDATE users SET role = CASE WHEN EXISTS(SELECT 1 FROM classes c WHERE c.teacher_id=users.id)
		THEN 'class_teacher' ELSE 'subject_teacher' END WHERE role='teacher'`); err != nil {
		return nil, err
	}
	// New rows must get ids after the imported ones.
	for _, table := range importTables {
		var hasID bool
		tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=? AND column_name='id')`, table).Scan(&hasID)
		if !hasID {
			continue
		}
		if _, err := tx.Exec(`SELECT setval(pg_get_serial_sequence('` + table + `','id'), COALESCE((SELECT MAX(id) FROM ` + table + `),0)+1, false)`); err != nil {
			return nil, fmt.Errorf("%s ids: %w", table, err)
		}
	}
	return counts, tx.Commit()
}

// copyTable copies the columns a table has in both databases.
func copyTable(src *sql.DB, tx *Tx, table string) (int, error) {
	var exists int
	src.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&exists)
	if exists == 0 {
		return 0, nil // this table was added after that version
	}
	srcCols := map[string]bool{}
	rows, err := src.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		srcCols[c] = true
	}
	rows.Close()

	type column struct{ name, kind string }
	var cols []column
	rows, err = tx.Query(`SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name=? ORDER BY ordinal_position`, table)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var c column
		rows.Scan(&c.name, &c.kind)
		if srcCols[c.name] {
			cols = append(cols, c)
		}
	}
	rows.Close()
	if len(cols) == 0 {
		return 0, nil
	}

	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.name
	}
	list := strings.Join(names, ",")
	// Rows go in batches, one statement per batch, so the import is quick even when
	// the database server is far away.
	batchSize := max(1, 1000/len(cols))
	var pending []any
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		row := "(" + placeholders(len(cols)) + ")"
		q := `INSERT INTO ` + table + `(` + list + `) VALUES ` + strings.TrimSuffix(strings.Repeat(row+",", len(pending)/len(cols)), ",")
		if table == "settings" {
			q += ` ON CONFLICT(key) DO UPDATE SET value=excluded.value` // defaults are already there
		}
		_, err := tx.Tx.Exec(rebind(q), pending...)
		pending = pending[:0]
		return err
	}

	srcRows, err := src.Query(`SELECT ` + list + ` FROM ` + table)
	if err != nil {
		return 0, err
	}
	defer srcRows.Close()
	n := 0
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for srcRows.Next() {
		if err := srcRows.Scan(ptrs...); err != nil {
			return n, err
		}
		for i, c := range cols {
			pending = append(pending, convertValue(vals[i], c.kind))
		}
		n++
		if n%batchSize == 0 {
			if err := flush(); err != nil {
				return n, fmt.Errorf("rows up to %d: %w", n, err)
			}
		}
	}
	if err := srcRows.Err(); err != nil {
		return n, err
	}
	if err := flush(); err != nil {
		return n, fmt.Errorf("rows up to %d: %w", n, err)
	}
	return n, nil
}

// convertValue adapts SQLite's loosely typed values to the PostgreSQL column type.
func convertValue(v any, kind string) any {
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if v == nil {
		return nil
	}
	switch kind {
	case "text":
		switch x := v.(type) {
		case string:
			return x
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		default:
			return fmt.Sprint(x)
		}
	case "bigint", "integer":
		switch x := v.(type) {
		case float64:
			return int64(x)
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
				return n
			}
			if x == "" {
				return nil
			}
		}
	case "double precision":
		switch x := v.(type) {
		case int64:
			return float64(x)
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
				return f
			}
		}
	}
	return v
}

// importLegacyOnFirstStart copies an earlier version's SQLite data into a brand-new
// PostgreSQL database. A database that was emptied on purpose is left empty.
func importLegacyOnFirstStart(db *DB, cfg Config, store *storage) {
	if !db.fresh {
		return
	}
	path := legacySQLiteFile(cfg, store)
	if path == "" {
		return
	}
	log.Printf("Importing data from the SQLite database %s ...", path)
	counts, err := importSQLite(db, path)
	if err != nil {
		store.setNotice("Could not import the earlier SQLite database " + path + ": " + err.Error())
		log.Printf("SQLite import failed: %v", err)
		return
	}
	msg := fmt.Sprintf("Imported %d users, %d students and %d attendance records from the earlier SQLite database %s. That file was not changed and can be kept as a backup.",
		counts["users"], counts["students"], counts["attendance"], path)
	store.setNotice(msg)
	log.Print(msg)
}
