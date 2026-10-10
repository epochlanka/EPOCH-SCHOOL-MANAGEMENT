package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

var leaveTypes = map[string]string{
	"casual": "Casual leave", "medical": "Medical leave", "duty": "Duty leave",
	"half_day": "Half day", "maternity": "Maternity leave", "other": "Other",
}

// ---------- Staff attendance ----------

// staffOnLeave returns staff with approved leave covering date.
func (a *App) staffOnLeave(date string) map[int64]bool {
	ids, _ := a.queryIDs(`SELECT user_id FROM leave_requests WHERE status='approved' AND ? BETWEEN from_date AND to_date`, date)
	out := map[int64]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// unavailableTeachers returns teachers who cannot teach on date: on approved leave or marked absent/leave.
func (a *App) unavailableTeachers(date string) map[int64]bool {
	out := a.staffOnLeave(date)
	ids, _ := a.queryIDs(`SELECT user_id FROM staff_attendance WHERE date=? AND status IN ('absent','leave')`, date)
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func (a *App) handleStaffAttendance(w http.ResponseWriter, r *http.Request, u *User) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		date = today()
	}
	rows, err := a.queryMaps(`SELECT u.id AS user_id, u.name, u.role, u.phone,
		COALESCE(sa.status,'') AS status, COALESCE(sa.check_in,'') AS check_in, COALESCE(sa.check_out,'') AS check_out,
		(SELECT lr.leave_type FROM leave_requests lr WHERE lr.user_id=u.id AND lr.status='approved' AND ? BETWEEN lr.from_date AND lr.to_date LIMIT 1) AS leave_type
		FROM users u LEFT JOIN staff_attendance sa ON sa.user_id=u.id AND sa.date=?
		WHERE u.active=1 AND u.role IN `+staffRolesSQL+` ORDER BY u.name`, date, date)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, row := range rows {
		row["role_label"] = roleLabel(row["role"].(string))
		row["teaching"] = isTeaching(row["role"].(string))
	}
	writeJSON(w, 200, map[string]any{"date": date, "staff": rows})
}

func (a *App) handleSaveStaffAttendance(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Date    string `json:"date"`
		Records []struct {
			UserID int64  `json:"user_id"`
			Status string `json:"status"`
		} `json:"records"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if !validDate(req.Date) || req.Date > today() {
		errJSON(w, 400, "Choose today or a past date")
		return
	}
	changed := false
	for _, rec := range req.Records {
		switch rec.Status {
		case "present", "late", "absent", "leave":
		default:
			continue
		}
		var prev string
		a.db.QueryRow(`SELECT status FROM staff_attendance WHERE user_id=? AND date=?`, rec.UserID, req.Date).Scan(&prev)
		checkIn := ""
		if (rec.Status == "present" || rec.Status == "late") && req.Date == today() {
			checkIn = time.Now().Format("15:04")
		}
		if _, err := a.db.Exec(`INSERT INTO staff_attendance(user_id,date,status,check_in,marked_by) VALUES(?,?,?,?,?)
			ON CONFLICT(user_id,date) DO UPDATE SET status=excluded.status, marked_by=excluded.marked_by,
			check_in=CASE WHEN staff_attendance.check_in='' THEN excluded.check_in ELSE staff_attendance.check_in END`,
			rec.UserID, req.Date, rec.Status, checkIn, u.ID); err != nil {
			serverError(w, err)
			return
		}
		if prev == rec.Status {
			continue
		}
		changed = true
		// A teacher who turns up after all no longer needs cover.
		if rec.Status == "present" || rec.Status == "late" {
			a.db.Exec(`DELETE FROM substitutions WHERE absent_teacher_id=? AND date=?`, rec.UserID, req.Date)
		}
	}
	res := map[string]any{"ok": true}
	if changed && req.Date >= today() {
		assigned, unfilled := a.assignSubstitutions(req.Date)
		res["substitutions_assigned"], res["substitutions_unfilled"] = assigned, unfilled
	}
	writeJSON(w, 200, res)
}

// handleCheckIn lets any staff member record their own arrival or departure.
func (a *App) handleCheckIn(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Action string `json:"action"` // "in" or "out"
	}
	readJSON(r, &req)
	t, clock := today(), time.Now().Format("15:04")
	if req.Action == "out" {
		res, _ := a.db.Exec(`UPDATE staff_attendance SET check_out=? WHERE user_id=? AND date=? AND check_in<>''`, clock, u.ID, t)
		if n, _ := res.RowsAffected(); n == 0 {
			errJSON(w, 400, "Check in first")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "checked_out", "time": clock})
		return
	}
	var existing string
	a.db.QueryRow(`SELECT check_in FROM staff_attendance WHERE user_id=? AND date=?`, u.ID, t).Scan(&existing)
	if existing != "" {
		errJSON(w, 400, "You already checked in at "+existing)
		return
	}
	status := "present"
	if late := a.setting("staff_late_after"); late != "" && clock > late {
		status = "late"
	}
	if _, err := a.db.Exec(`INSERT INTO staff_attendance(user_id,date,status,check_in,marked_by) VALUES(?,?,?,?,?)
		ON CONFLICT(user_id,date) DO UPDATE SET status=excluded.status, check_in=excluded.check_in`, u.ID, t, status, clock, u.ID); err != nil {
		serverError(w, err)
		return
	}
	a.db.Exec(`DELETE FROM substitutions WHERE absent_teacher_id=? AND date=?`, u.ID, t)
	writeJSON(w, 200, map[string]string{"status": status, "time": clock})
}

// ---------- Leave ----------

func (a *App) pendingLeaveCount(u *User) float64 {
	if !hasPerm(u.Role, PLeaveApprove) {
		return 0
	}
	return a.scalar(`SELECT COUNT(*) FROM leave_requests WHERE status='pending' AND user_id<>?`, u.ID)
}

func (a *App) handleListLeave(w http.ResponseWriter, r *http.Request, u *User) {
	status := r.URL.Query().Get("status")
	mine := r.URL.Query().Get("mine") == "1" || !hasPerm(u.Role, PLeaveApprove)
	q := `SELECT l.*, u.name, u.role, rv.name AS reviewer_name,
		CAST(julianday(l.to_date)-julianday(l.from_date)+1 AS INTEGER) AS days
		FROM leave_requests l JOIN users u ON u.id=l.user_id LEFT JOIN users rv ON rv.id=l.reviewed_by
		WHERE (?='' OR l.status=?)`
	args := []any{status, status}
	if mine {
		q += ` AND l.user_id=?`
		args = append(args, u.ID)
	}
	rows, err := a.queryMaps(q+` ORDER BY CASE l.status WHEN 'pending' THEN 0 ELSE 1 END, l.from_date DESC LIMIT 300`, args...)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, row := range rows {
		row["role_label"] = roleLabel(row["role"].(string))
		row["type_label"] = leaveTypes[row["leave_type"].(string)]
	}
	writeJSON(w, 200, rows)
}

func (a *App) handleApplyLeave(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Type   string `json:"leave_type"`
		From   string `json:"from_date"`
		To     string `json:"to_date"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if _, ok := leaveTypes[req.Type]; !ok {
		errJSON(w, 400, "Choose a leave type")
		return
	}
	if req.To == "" {
		req.To = req.From
	}
	if !validDate(req.From) || !validDate(req.To) || req.To < req.From {
		errJSON(w, 400, "Choose valid from and to dates")
		return
	}
	if a.scalar(`SELECT COUNT(*) FROM leave_requests WHERE user_id=? AND status IN ('pending','approved') AND from_date<=? AND to_date>=?`, u.ID, req.To, req.From) > 0 {
		errJSON(w, 409, "You already have a leave request for these dates")
		return
	}
	res, err := a.db.Exec(`INSERT INTO leave_requests(user_id,leave_type,from_date,to_date,reason,created_at) VALUES(?,?,?,?,?,?)`,
		u.ID, req.Type, req.From, req.To, strings.TrimSpace(req.Reason), now())
	if err != nil {
		serverError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	approvers, _ := a.queryIDs(`SELECT id FROM users WHERE active=1 AND id<>? AND role IN ('admin','principal','vice_principal')
		UNION SELECT s.head_id FROM sections s JOIN classes c ON c.section_id=s.id
		WHERE c.teacher_id=? OR c.id IN (SELECT class_id FROM class_subjects WHERE teacher_id=?)`, u.ID, u.ID, u.ID)
	a.notify.NotifyEach("leave", approvers, []string{"app"}, func(lang string) (string, string) {
		return L(lang, "leavereq.title", u.Name),
			strings.TrimSpace(L(lang, "leavereq.body", u.Name, L(lang, "leave."+req.Type), dateRangeL(req.From, req.To, lang), req.Reason))
	})
	writeJSON(w, 201, map[string]any{"id": id})
}

func (a *App) handleLeaveDecision(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var req struct {
		Status string `json:"status"` // approved | rejected
		Note   string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.Status != "approved" && req.Status != "rejected" {
		errJSON(w, 400, "Status must be approved or rejected")
		return
	}
	var userID int64
	var from, to, prev, typ string
	if err := a.db.QueryRow(`SELECT user_id, from_date, to_date, status, leave_type FROM leave_requests WHERE id=?`, id).Scan(&userID, &from, &to, &prev, &typ); err != nil {
		errJSON(w, 404, "Leave request not found")
		return
	}
	if userID == u.ID {
		errJSON(w, 403, "You cannot approve your own leave")
		return
	}
	if prev == "cancelled" {
		errJSON(w, 400, "This request was cancelled by the staff member")
		return
	}
	a.db.Exec(`UPDATE leave_requests SET status=?, review_note=?, reviewed_by=?, reviewed_at=? WHERE id=?`, req.Status, strings.TrimSpace(req.Note), u.ID, now(), id)

	res := map[string]any{"ok": true}
	if req.Status == "approved" {
		assigned, unfilled := a.coverLeave(from, to)
		res["substitutions_assigned"], res["substitutions_unfilled"] = assigned, unfilled
	} else if prev == "approved" {
		a.dropCover(userID, from, to)
	}
	a.notify.NotifyEach("leave", []int64{userID}, a.notify.channelsFor("staff"), func(lang string) (string, string) {
		msg := L(lang, "leave."+req.Status+".body", L(lang, "leave."+typ), dateRangeL(from, to, lang), u.Name)
		if req.Note != "" {
			msg += L(lang, "leave.note", req.Note)
		}
		return L(lang, "leave."+req.Status+".title"), msg
	})
	writeJSON(w, 200, res)
}

func (a *App) handleCancelLeave(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var userID int64
	var from, to, status string
	if err := a.db.QueryRow(`SELECT user_id, from_date, to_date, status FROM leave_requests WHERE id=?`, id).Scan(&userID, &from, &to, &status); err != nil || userID != u.ID {
		errJSON(w, 404, "Leave request not found")
		return
	}
	if status != "pending" && status != "approved" {
		errJSON(w, 400, "Only pending or approved requests can be cancelled")
		return
	}
	if status == "approved" && to < today() {
		errJSON(w, 400, "This leave has already been taken")
		return
	}
	a.db.Exec(`UPDATE leave_requests SET status='cancelled' WHERE id=?`, id)
	if status == "approved" {
		a.dropCover(userID, from, to)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// coverLeave assigns relief teachers for every upcoming school day in a leave period.
func (a *App) coverLeave(from, to string) (assigned, unfilled int) {
	start, _ := time.Parse(dateLayout, max(from, today()))
	end, _ := time.Parse(dateLayout, to)
	for d, n := start, 0; !d.After(end) && n < 60; d, n = d.AddDate(0, 0, 1), n+1 {
		as, un := a.assignSubstitutions(d.Format(dateLayout))
		assigned += as
		unfilled += un
	}
	return
}

// refreshCover recalculates relief duties for today and upcoming leave after the
// timetable changes, since periods may have moved.
func (a *App) refreshCover() {
	t := today()
	a.db.Exec(`DELETE FROM substitutions WHERE date>=?`, t)
	a.assignSubstitutions(t)
	rows, _ := a.queryMaps(`SELECT from_date, to_date FROM leave_requests WHERE status='approved' AND to_date>?`, t)
	for _, r := range rows {
		a.coverLeave(r["from_date"].(string), r["to_date"].(string))
	}
}

// dropCover removes future relief assignments made for a teacher's leave.
func (a *App) dropCover(userID int64, from, to string) {
	a.db.Exec(`DELETE FROM substitutions WHERE absent_teacher_id=? AND date BETWEEN ? AND ? AND date>=?`, userID, from, to, today())
}

// ---------- Substitutions ----------

type subDuty struct {
	period                    int
	time, class, subject, who string
}

// assignSubstitutions finds every period on date whose teacher is unavailable and
// gives it to a free teacher: someone not on leave or absent, with no class of their
// own and no other relief duty in that period. Teachers of the same subject are
// preferred, then those with the fewest periods and relief duties that day. Each
// relief teacher is notified once with all their periods. It returns how many
// periods were covered and how many could not be.
func (a *App) assignSubstitutions(date string) (assigned, unfilled int) {
	cfg := a.timetableConfig()
	day := cfg.schoolDay(date)
	if day == 0 {
		return 0, 0
	}
	away := a.unavailableTeachers(date)
	// Drop assignments whose relief teacher is now unavailable themselves.
	for id := range away {
		a.db.Exec(`DELETE FROM substitutions WHERE date=? AND substitute_id=?`, date, id)
	}
	if len(away) == 0 {
		return 0, 0
	}
	awayIDs := make([]int64, 0, len(away))
	for id := range away {
		awayIDs = append(awayIDs, id)
	}

	type need struct {
		classID, subjectID, teacherID int64
		period                        int
		class, subject, teacher       string
	}
	rows, err := a.db.Query(`SELECT t.class_id, t.subject_id, t.teacher_id, t.period, c.name||' - '||c.section, s.name, u.name
		FROM timetable t JOIN classes c ON c.id=t.class_id JOIN subjects s ON s.id=t.subject_id JOIN users u ON u.id=t.teacher_id
		WHERE t.day=? AND t.teacher_id IN (`+placeholders(len(awayIDs))+`)
		AND NOT EXISTS (SELECT 1 FROM substitutions x WHERE x.date=? AND x.class_id=t.class_id AND x.period=t.period AND x.status='assigned')
		ORDER BY t.period`, append(append([]any{day}, int64Args(awayIDs)...), date)...)
	if err != nil {
		log.Printf("substitutions: %v", err)
		return 0, 0
	}
	var needs []need
	for rows.Next() {
		var n need
		if rows.Scan(&n.classID, &n.subjectID, &n.teacherID, &n.period, &n.class, &n.subject, &n.teacher) == nil {
			needs = append(needs, n)
		}
	}
	rows.Close()
	if len(needs) == 0 {
		return 0, 0
	}

	// Candidate pool: active teaching staff who are in school today.
	candidates, _ := a.queryIDs(`SELECT id FROM users WHERE active=1 AND role IN ` + teachingRolesSQL)
	busy := map[slotKey]bool{}
	periodsToday := map[int64]int{}
	ttRows, _ := a.db.Query(`SELECT teacher_id, period FROM timetable WHERE day=? AND teacher_id IS NOT NULL`, day)
	for ttRows.Next() {
		var t int64
		var p int
		ttRows.Scan(&t, &p)
		busy[slotKey{t, 0, p}] = true
		periodsToday[t]++
	}
	ttRows.Close()
	reliefToday := map[int64]int{}
	subRows, _ := a.db.Query(`SELECT substitute_id, period FROM substitutions WHERE date=? AND status='assigned'`, date)
	for subRows.Next() {
		var t int64
		var p int
		subRows.Scan(&t, &p)
		busy[slotKey{t, 0, p}] = true
		reliefToday[t]++
	}
	subRows.Close()
	teaches := map[[2]int64]bool{} // teacher, subject
	csRows, _ := a.db.Query(`SELECT teacher_id, subject_id FROM class_subjects WHERE teacher_id IS NOT NULL`)
	for csRows.Next() {
		var t, s int64
		csRows.Scan(&t, &s)
		teaches[[2]int64{t, s}] = true
	}
	csRows.Close()

	duties := map[int64][]subDuty{}
	for _, n := range needs {
		best, bestScore := int64(0), 1<<30
		for _, c := range candidates {
			if away[c] || busy[slotKey{c, 0, n.period}] {
				continue
			}
			score := periodsToday[c] + reliefToday[c]*3
			if !teaches[[2]int64{c, n.subjectID}] {
				score += 10
			}
			if score < bestScore {
				best, bestScore = c, score
			}
		}
		status := "assigned"
		if best == 0 {
			status = "unfilled"
			unfilled++
		} else {
			assigned++
			busy[slotKey{best, 0, n.period}] = true
			reliefToday[best]++
			duties[best] = append(duties[best], subDuty{n.period, cfg.periodTime(n.period), n.class, n.subject, n.teacher})
		}
		a.db.Exec(`INSERT INTO substitutions(date,class_id,period,subject_id,absent_teacher_id,substitute_id,status,created_at) VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(date,class_id,period) DO UPDATE SET substitute_id=excluded.substitute_id, status=excluded.status, absent_teacher_id=excluded.absent_teacher_id`,
			date, n.classID, n.period, n.subjectID, n.teacherID, nullID(best), status, now())
	}
	for teacher, list := range duties {
		a.notifyRelief(teacher, date, list)
	}
	if unfilled > 0 {
		managers, _ := a.queryIDs(`SELECT id FROM users WHERE active=1 AND role IN ('admin','principal','vice_principal','section_head')`)
		a.notify.NotifyEach("staff", managers, []string{"app"}, func(lang string) (string, string) {
			return L(lang, "unfilled.title"), L(lang, "unfilled.body", fmt.Sprint(unfilled), dateL(date, lang))
		})
	}
	return
}

func (a *App) notifyRelief(teacher int64, date string, list []subDuty) {
	sort.Slice(list, func(i, j int) bool { return list[i].period < list[j].period })
	a.notify.NotifyEach("substitution", []int64{teacher}, a.notify.channelsFor("staff"), func(lang string) (string, string) {
		var lines []string
		for _, d := range list {
			lines = append(lines, L(lang, "relief.line", fmt.Sprint(d.period), d.time, d.class, d.subject, d.who))
		}
		return L(lang, "relief.title", dateL(date, lang)), strings.Join(lines, "\n")
	})
}

func (a *App) handleListSubstitutions(w http.ResponseWriter, r *http.Request, u *User) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		date = today()
	}
	cfg := a.timetableConfig()
	rows, err := a.queryMaps(`SELECT x.*, c.name||' - '||c.section AS class_name, s.name AS subject_name,
		ab.name AS absent_name, sb.name AS substitute_name
		FROM substitutions x JOIN classes c ON c.id=x.class_id LEFT JOIN subjects s ON s.id=x.subject_id
		LEFT JOIN users ab ON ab.id=x.absent_teacher_id LEFT JOIN users sb ON sb.id=x.substitute_id
		WHERE x.date=? ORDER BY x.period, c.name`, date)
	if err != nil {
		serverError(w, err)
		return
	}
	for _, row := range rows {
		row["time"] = cfg.periodTime(int(row["period"].(int64)))
	}
	away := []map[string]any{}
	for id := range a.unavailableTeachers(date) {
		if m, err := a.queryOne(`SELECT id,name,role FROM users WHERE id=?`, id); err == nil {
			m["role_label"] = roleLabel(m["role"].(string))
			away = append(away, m)
		}
	}
	writeJSON(w, 200, map[string]any{"date": date, "school_day": cfg.schoolDay(date) > 0, "rows": rows, "away": away})
}

func (a *App) handleAutoSubstitutions(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Date string `json:"date"`
	}
	readJSON(r, &req)
	if !validDate(req.Date) {
		req.Date = today()
	}
	// Re-try unfilled periods too.
	a.db.Exec(`DELETE FROM substitutions WHERE date=? AND status='unfilled'`, req.Date)
	assigned, unfilled := a.assignSubstitutions(req.Date)
	writeJSON(w, 200, map[string]int{"assigned": assigned, "unfilled": unfilled})
}

// handleFreeTeachers lists teachers free in a period on a date (for manual changes).
func (a *App) handleFreeTeachers(w http.ResponseWriter, r *http.Request, u *User) {
	date, period := r.URL.Query().Get("date"), int(queryInt(r, "period"))
	cfg := a.timetableConfig()
	day := cfg.schoolDay(date)
	away := a.unavailableTeachers(date)
	rows, err := a.queryMaps(`SELECT u.id, u.name, u.role FROM users u WHERE u.active=1 AND u.role IN `+teachingRolesSQL+`
		AND NOT EXISTS (SELECT 1 FROM timetable t WHERE t.teacher_id=u.id AND t.day=? AND t.period=?)
		AND NOT EXISTS (SELECT 1 FROM substitutions x WHERE x.substitute_id=u.id AND x.date=? AND x.period=?)
		ORDER BY u.name`, day, period, date, period)
	if err != nil {
		serverError(w, err)
		return
	}
	out := []map[string]any{}
	for _, row := range rows {
		if !away[row["id"].(int64)] {
			row["role_label"] = roleLabel(row["role"].(string))
			out = append(out, row)
		}
	}
	writeJSON(w, 200, out)
}

func (a *App) handleSetSubstitute(w http.ResponseWriter, r *http.Request, u *User) {
	id := pathID(r, "id")
	var req struct {
		SubstituteID int64 `json:"substitute_id"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	var date, class, subject, absent string
	var period int
	if err := a.db.QueryRow(`SELECT x.date, x.period, c.name||' - '||c.section, COALESCE(s.name,''), COALESCE(ab.name,'')
		FROM substitutions x JOIN classes c ON c.id=x.class_id LEFT JOIN subjects s ON s.id=x.subject_id LEFT JOIN users ab ON ab.id=x.absent_teacher_id
		WHERE x.id=?`, id).Scan(&date, &period, &class, &subject, &absent); err != nil {
		errJSON(w, 404, "Substitution not found")
		return
	}
	status := "unfilled"
	if req.SubstituteID > 0 {
		status = "assigned"
		if a.scalar(`SELECT COUNT(*) FROM substitutions WHERE date=? AND period=? AND substitute_id=? AND id<>?`, date, period, req.SubstituteID, id) > 0 {
			errJSON(w, 409, "That teacher already has relief duty in this period")
			return
		}
	}
	a.db.Exec(`UPDATE substitutions SET substitute_id=?, status=? WHERE id=?`, nullID(req.SubstituteID), status, id)
	if req.SubstituteID > 0 {
		cfg := a.timetableConfig()
		a.notifyRelief(req.SubstituteID, date, []subDuty{{period, cfg.periodTime(period), class, subject, absent}})
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- My workspace (every staff member) ----------

func (a *App) handleWorkspace(w http.ResponseWriter, r *http.Request, u *User) {
	t := today()
	cfg := a.timetableConfig()
	day := cfg.schoolDay(t)
	att, err := a.queryOne(`SELECT status, check_in, check_out FROM staff_attendance WHERE user_id=? AND date=?`, u.ID, t)
	if err != nil {
		att = map[string]any{"status": "", "check_in": "", "check_out": ""}
	}
	lessons, _ := a.queryMaps(`SELECT t.period, s.name AS subject_name, c.name||' - '||c.section AS class_name,
		(SELECT sb.name FROM substitutions x JOIN users sb ON sb.id=x.substitute_id WHERE x.date=? AND x.class_id=t.class_id AND x.period=t.period) AS covered_by
		FROM timetable t JOIN subjects s ON s.id=t.subject_id JOIN classes c ON c.id=t.class_id
		WHERE t.teacher_id=? AND t.day=? ORDER BY t.period`, t, u.ID, day)
	relief, _ := a.queryMaps(`SELECT x.period, s.name AS subject_name, c.name||' - '||c.section AS class_name, ab.name AS absent_name
		FROM substitutions x JOIN classes c ON c.id=x.class_id LEFT JOIN subjects s ON s.id=x.subject_id LEFT JOIN users ab ON ab.id=x.absent_teacher_id
		WHERE x.substitute_id=? AND x.date=? ORDER BY x.period`, u.ID, t)
	for _, list := range [][]map[string]any{lessons, relief} {
		for _, row := range list {
			row["time"] = cfg.periodTime(int(row["period"].(int64)))
		}
	}
	upcoming, _ := a.queryMaps(`SELECT x.date, x.period, s.name AS subject_name, c.name||' - '||c.section AS class_name
		FROM substitutions x JOIN classes c ON c.id=x.class_id LEFT JOIN subjects s ON s.id=x.subject_id
		WHERE x.substitute_id=? AND x.date>? ORDER BY x.date, x.period LIMIT 20`, u.ID, t)
	myClasses, _ := a.queryMaps(`SELECT c.id, c.name||' - '||c.section AS label FROM classes c WHERE c.teacher_id=?`, u.ID)
	writeJSON(w, 200, map[string]any{
		"today": t, "school_day": day > 0, "day_name": time.Now().Format("Monday"),
		"attendance": att, "lessons": lessons, "relief": relief, "upcoming_relief": upcoming,
		"my_classes": myClasses, "on_leave": a.staffOnLeave(t)[u.ID], "late_after": a.setting("staff_late_after"),
	})
}
