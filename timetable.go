package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------- Timetable configuration ----------

type ttConfig struct {
	Days          int      `json:"days"`
	Periods       int      `json:"periods"`
	Start         string   `json:"start"`
	PeriodMinutes int      `json:"period_minutes"`
	BreakAfter    int      `json:"break_after"`
	BreakMinutes  int      `json:"break_minutes"`
	Times         []string `json:"times"`
	DayNames      []string `json:"day_names"`
}

func (a *App) timetableConfig() ttConfig {
	atoi := func(k string, def, lo, hi int) int {
		v, err := strconv.Atoi(a.setting(k))
		if err != nil || v < lo || v > hi {
			return def
		}
		return v
	}
	c := ttConfig{
		Days:          atoi("school_days", 5, 1, 6),
		Periods:       atoi("periods_per_day", 8, 1, 12),
		Start:         a.setting("day_start"),
		PeriodMinutes: atoi("period_minutes", 40, 10, 120),
		BreakAfter:    atoi("break_after", 4, 0, 12),
		BreakMinutes:  atoi("break_minutes", 20, 0, 120),
		DayNames:      []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	}
	c.DayNames = c.DayNames[:c.Days]
	start, err := time.Parse("15:04", c.Start)
	if err != nil {
		start, _ = time.Parse("15:04", "07:40")
	}
	t := start
	for p := 1; p <= c.Periods; p++ {
		end := t.Add(time.Duration(c.PeriodMinutes) * time.Minute)
		c.Times = append(c.Times, t.Format("15:04")+"–"+end.Format("15:04"))
		t = end
		if p == c.BreakAfter {
			t = t.Add(time.Duration(c.BreakMinutes) * time.Minute)
		}
	}
	return c
}

// schoolDay returns the timetable day (1 = Monday) for a date, or 0 if there is no school.
func (c ttConfig) schoolDay(date string) int {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return 0
	}
	wd := int(t.Weekday()) // Sunday = 0
	if wd == 0 || wd > c.Days {
		return 0
	}
	return wd
}

func (c ttConfig) periodTime(p int) string {
	if p >= 1 && p <= len(c.Times) {
		return c.Times[p-1]
	}
	return ""
}

// ---------- Generator ----------

type genResult struct {
	Placed   int      `json:"placed"` // lessons placed
	Unplaced []string `json:"unplaced"`
	Warnings []string `json:"warnings"`
}

type lesson struct {
	classID, subjectID, teacherID int64
	label                         string
}

type slotKey struct {
	id     int64
	day, p int
}

// generateTimetable builds a clash-free weekly timetable for the given classes
// (nil = all classes) from their subject allocations. Periods of classes that are
// not regenerated are kept and their teachers treated as busy. It tries many
// randomised orders and keeps the arrangement that places the most lessons,
// spreading each subject across the week (at most two periods a day).
func (a *App) generateTimetable(classIDs []int64) (genResult, error) {
	cfg := a.timetableConfig()
	var res genResult
	if len(classIDs) == 0 {
		var err error
		if classIDs, err = a.queryIDs(`SELECT id FROM classes`); err != nil {
			return res, err
		}
	}
	if len(classIDs) == 0 {
		return res, fmt.Errorf("no classes to schedule")
	}
	inList := placeholders(len(classIDs))
	rows, err := a.db.Query(`SELECT cs.class_id, cs.subject_id, COALESCE(cs.teacher_id,0), cs.periods_per_week,
		c.name||' '||c.section, s.name FROM class_subjects cs JOIN classes c ON c.id=cs.class_id JOIN subjects s ON s.id=cs.subject_id
		WHERE cs.class_id IN (`+inList+`)`, int64Args(classIDs)...)
	if err != nil {
		return res, err
	}
	var lessons []lesson
	perClass := map[int64]int{}
	classNames := map[int64]string{}
	teacherLoad := map[int64]int{}
	for rows.Next() {
		var l lesson
		var n int
		var cname, sname string
		if err := rows.Scan(&l.classID, &l.subjectID, &l.teacherID, &n, &cname, &sname); err != nil {
			rows.Close()
			return res, err
		}
		l.label = cname + " – " + sname
		classNames[l.classID] = cname
		for i := 0; i < n; i++ {
			lessons = append(lessons, l)
		}
		perClass[l.classID] += n
		teacherLoad[l.teacherID] += n
	}
	rows.Close()
	if len(lessons) == 0 {
		return res, fmt.Errorf("no subjects are allocated yet; add subjects and teachers to each class first")
	}
	slots := cfg.Days * cfg.Periods
	for c, n := range perClass {
		if n > slots {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s has %d periods allocated but only %d slots a week", classNames[c], n, slots))
		}
	}

	// Teachers already busy in classes we are not regenerating.
	fixedBusy := map[slotKey]bool{}
	busyRows, err := a.db.Query(`SELECT teacher_id, day, period FROM timetable WHERE teacher_id IS NOT NULL AND class_id NOT IN (`+inList+`)`, int64Args(classIDs)...)
	if err != nil {
		return res, err
	}
	for busyRows.Next() {
		var k slotKey
		busyRows.Scan(&k.id, &k.day, &k.p)
		fixedBusy[k] = true
		teacherLoad[k.id]++
	}
	busyRows.Close()

	type placement struct {
		l      lesson
		day, p int
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var best []placement
	var bestMissing []lesson
	for attempt := 0; attempt < 60; attempt++ {
		order := append([]lesson(nil), lessons...)
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		// Most constrained first: busiest teachers, then fullest classes.
		sort.SliceStable(order, func(i, j int) bool {
			if teacherLoad[order[i].teacherID] != teacherLoad[order[j].teacherID] {
				return teacherLoad[order[i].teacherID] > teacherLoad[order[j].teacherID]
			}
			return perClass[order[i].classID] > perClass[order[j].classID]
		})
		classBusy := map[slotKey]bool{}
		teacherBusy := map[slotKey]bool{}
		for k := range fixedBusy {
			teacherBusy[k] = true
		}
		subjDay := map[[3]int64]int{} // class, subject, day -> count
		teacherDay := map[slotKey]int{}
		var placed []placement
		var missing []lesson
		for _, l := range order {
			bestScore, bd, bp := 1<<30, 0, 0
			for d := 1; d <= cfg.Days; d++ {
				for p := 1; p <= cfg.Periods; p++ {
					if classBusy[slotKey{l.classID, d, p}] || (l.teacherID != 0 && teacherBusy[slotKey{l.teacherID, d, p}]) {
						continue
					}
					same := subjDay[[3]int64{l.classID, l.subjectID, int64(d)}]
					score := same*20 + rng.Intn(6)
					if same >= 2 {
						score += 400
					}
					if l.teacherID != 0 {
						score += teacherDay[slotKey{l.teacherID, d, 0}] * 2
					}
					if score < bestScore {
						bestScore, bd, bp = score, d, p
					}
				}
			}
			if bd == 0 {
				missing = append(missing, l)
				continue
			}
			classBusy[slotKey{l.classID, bd, bp}] = true
			if l.teacherID != 0 {
				teacherBusy[slotKey{l.teacherID, bd, bp}] = true
				teacherDay[slotKey{l.teacherID, bd, 0}]++
			}
			subjDay[[3]int64{l.classID, l.subjectID, int64(bd)}]++
			placed = append(placed, placement{l, bd, bp})
		}
		if best == nil || len(missing) < len(bestMissing) {
			best, bestMissing = placed, missing
		}
		if len(missing) == 0 {
			break
		}
	}

	tx, err := a.db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM timetable WHERE class_id IN (`+inList+`)`, int64Args(classIDs)...); err != nil {
		return res, err
	}
	for _, pl := range best {
		if _, err := tx.Exec(`INSERT INTO timetable(class_id,day,period,subject_id,teacher_id) VALUES(?,?,?,?,?)`,
			pl.l.classID, pl.day, pl.p, pl.l.subjectID, nullID(pl.l.teacherID)); err != nil {
			return res, err
		}
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	res.Placed = len(best)
	counts := map[string]int{}
	for _, l := range bestMissing {
		counts[l.label]++
	}
	for label, n := range counts {
		res.Unplaced = append(res.Unplaced, fmt.Sprintf("%s (%d period%s)", label, n, plural(n)))
	}
	sort.Strings(res.Unplaced)
	if res.Unplaced == nil {
		res.Unplaced = []string{}
	}
	if res.Warnings == nil {
		res.Warnings = []string{}
	}
	return res, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ---------- Handlers: sections, subjects, allocation, timetable ----------

func (a *App) handleListSections(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT s.id,s.name,s.sort,s.head_id,h.name AS head_name,
		(SELECT COUNT(*) FROM classes c WHERE c.section_id=s.id) AS class_count,
		(SELECT COUNT(*) FROM students st JOIN classes c ON c.id=st.class_id WHERE c.section_id=s.id) AS student_count
		FROM sections s LEFT JOIN users h ON h.id=s.head_id ORDER BY s.sort, s.name`)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveSection(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Name   string `json:"name"`
		HeadID int64  `json:"head_id"`
		Sort   int    `json:"sort"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		errJSON(w, 400, "Section name is required")
		return
	}
	var err error
	if id := pathID(r, "id"); id > 0 {
		_, err = a.db.Exec(`UPDATE sections SET name=?,head_id=?,sort=? WHERE id=?`, req.Name, nullID(req.HeadID), req.Sort, id)
	} else {
		_, err = a.db.Exec(`INSERT INTO sections(name,head_id,sort) VALUES(?,?,?)`, req.Name, nullID(req.HeadID), req.Sort)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			errJSON(w, 409, "A section with this name already exists")
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteSection(w http.ResponseWriter, r *http.Request, u *User) {
	if _, err := a.db.Exec(`DELETE FROM sections WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleListSubjects(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.queryMaps(`SELECT s.*, (SELECT COUNT(*) FROM class_subjects cs WHERE cs.subject_id=s.id) AS class_count FROM subjects s ORDER BY s.name`)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveSubject(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		errJSON(w, 400, "Subject name is required")
		return
	}
	var err error
	if id := pathID(r, "id"); id > 0 {
		_, err = a.db.Exec(`UPDATE subjects SET name=?,code=? WHERE id=?`, req.Name, req.Code, id)
	} else {
		_, err = a.db.Exec(`INSERT INTO subjects(name,code) VALUES(?,?)`, req.Name, req.Code)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			errJSON(w, 409, "This subject already exists")
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteSubject(w http.ResponseWriter, r *http.Request, u *User) {
	if _, err := a.db.Exec(`DELETE FROM subjects WHERE id=?`, pathID(r, "id")); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleListClassSubjects(w http.ResponseWriter, r *http.Request, u *User) {
	classID := queryInt(r, "class_id")
	teacherID := queryInt(r, "teacher_id")
	rows, err := a.queryMaps(`SELECT cs.*, s.name AS subject_name, t.name AS teacher_name, c.name||' - '||c.section AS class_name
		FROM class_subjects cs JOIN subjects s ON s.id=cs.subject_id JOIN classes c ON c.id=cs.class_id LEFT JOIN users t ON t.id=cs.teacher_id
		WHERE (?=0 OR cs.class_id=?) AND (?=0 OR cs.teacher_id=?) ORDER BY c.name, c.section, s.name`, classID, classID, teacherID, teacherID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleSaveClassSubject(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassID   int64 `json:"class_id"`
		SubjectID int64 `json:"subject_id"`
		TeacherID int64 `json:"teacher_id"`
		Periods   int   `json:"periods_per_week"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.ClassID == 0 || req.SubjectID == 0 || req.Periods < 1 || req.Periods > 20 {
		errJSON(w, 400, "Class, subject and 1–20 periods per week are required")
		return
	}
	if req.TeacherID > 0 {
		var role string
		a.db.QueryRow(`SELECT role FROM users WHERE id=?`, req.TeacherID).Scan(&role)
		if !isTeaching(role) {
			errJSON(w, 400, "The selected person is not a teaching staff member")
			return
		}
	}
	_, err := a.db.Exec(`INSERT INTO class_subjects(class_id,subject_id,teacher_id,periods_per_week) VALUES(?,?,?,?)
		ON CONFLICT(class_id,subject_id) DO UPDATE SET teacher_id=excluded.teacher_id, periods_per_week=excluded.periods_per_week`,
		req.ClassID, req.SubjectID, nullID(req.TeacherID), req.Periods)
	if err != nil {
		serverError(w, err)
		return
	}
	// Keep already-generated periods in step with the new teacher.
	a.db.Exec(`UPDATE timetable SET teacher_id=? WHERE class_id=? AND subject_id=?`, nullID(req.TeacherID), req.ClassID, req.SubjectID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleDeleteClassSubject(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var classID, subjectID int64
	a.db.QueryRow(`SELECT class_id, subject_id FROM class_subjects WHERE id=?`, id).Scan(&classID, &subjectID)
	a.db.Exec(`DELETE FROM timetable WHERE class_id=? AND subject_id=?`, classID, subjectID)
	if _, err := a.db.Exec(`DELETE FROM class_subjects WHERE id=?`, id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleTimetable returns the timetable for a class or a teacher (default: the signed-in teacher).
func (a *App) handleTimetable(w http.ResponseWriter, r *http.Request, u *User) {
	classID, teacherID := queryInt(r, "class_id"), queryInt(r, "teacher_id")
	if classID == 0 && teacherID == 0 {
		teacherID = u.ID
	}
	var where string
	var arg int64
	if classID > 0 {
		where, arg = "t.class_id=?", classID
	} else {
		where, arg = "t.teacher_id=?", teacherID
	}
	entries, err := a.queryMaps(`SELECT t.id,t.class_id,t.day,t.period,t.subject_id,t.teacher_id,s.name AS subject_name,
		u.name AS teacher_name, c.name||' - '||c.section AS class_name
		FROM timetable t JOIN subjects s ON s.id=t.subject_id JOIN classes c ON c.id=t.class_id LEFT JOIN users u ON u.id=t.teacher_id
		WHERE `+where+` ORDER BY t.day, t.period`, arg)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"config": a.timetableConfig(), "entries": entries})
}

func (a *App) handleGenerateTimetable(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassIDs []int64 `json:"class_ids"`
	}
	readJSON(r, &req)
	res, err := a.generateTimetable(req.ClassIDs)
	if err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	a.refreshCover()
	writeJSON(w, 200, res)
}

// handleSetSlot changes one period of a class timetable by hand, refusing teacher clashes.
func (a *App) handleSetSlot(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		ClassID   int64 `json:"class_id"`
		Day       int   `json:"day"`
		Period    int   `json:"period"`
		SubjectID int64 `json:"subject_id"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	cfg := a.timetableConfig()
	if req.Day < 1 || req.Day > cfg.Days || req.Period < 1 || req.Period > cfg.Periods {
		errJSON(w, 400, "Invalid day or period")
		return
	}
	if req.SubjectID == 0 {
		a.db.Exec(`DELETE FROM timetable WHERE class_id=? AND day=? AND period=?`, req.ClassID, req.Day, req.Period)
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	var teacherID sql.NullInt64
	if err := a.db.QueryRow(`SELECT teacher_id FROM class_subjects WHERE class_id=? AND subject_id=?`, req.ClassID, req.SubjectID).Scan(&teacherID); err != nil {
		errJSON(w, 400, "This subject is not allocated to the class")
		return
	}
	if teacherID.Valid {
		var clash string
		a.db.QueryRow(`SELECT c.name||' - '||c.section FROM timetable t JOIN classes c ON c.id=t.class_id
			WHERE t.teacher_id=? AND t.day=? AND t.period=? AND t.class_id<>?`, teacherID.Int64, req.Day, req.Period, req.ClassID).Scan(&clash)
		if clash != "" {
			errJSON(w, 409, "The teacher already has "+clash+" in this period")
			return
		}
	}
	_, err := a.db.Exec(`INSERT INTO timetable(class_id,day,period,subject_id,teacher_id) VALUES(?,?,?,?,?)
		ON CONFLICT(class_id,day,period) DO UPDATE SET subject_id=excluded.subject_id, teacher_id=excluded.teacher_id`,
		req.ClassID, req.Day, req.Period, req.SubjectID, teacherID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleStaffList returns active staff for pickers (teaching=1 limits to teaching roles).
func (a *App) handleStaffList(w http.ResponseWriter, r *http.Request, u *User) {
	set := staffRolesSQL
	if r.URL.Query().Get("teaching") == "1" {
		set = teachingRolesSQL
	}
	rows, err := a.queryMaps(`SELECT id,name,role,email,phone,
		(SELECT COUNT(*) FROM timetable t WHERE t.teacher_id=users.id) AS periods
		FROM users WHERE active=1 AND role IN ` + set + ` ORDER BY name`)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, row := range rows {
		row["role_label"] = roleLabel(row["role"].(string))
	}
	writeJSON(w, 200, rows)
}
