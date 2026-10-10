package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Progress report: a plain-language view of how a child is doing — attendance trend,
// each subject against the class average and their own last result, upcoming
// homework, teacher remarks, strengths, areas needing support and next steps — in
// the reader's language. With AI enabled the summary is written by Claude (cached
// once per child, language and day); otherwise it is assembled from rules.

type subjectProgress struct {
	Subject  string   `json:"subject"`
	Exam     string   `json:"exam"`
	Date     string   `json:"date"`
	Latest   float64  `json:"latest"`   // % in the most recent published exam
	Previous *float64 `json:"previous"` // % in the exam before that, if any
	ClassAvg float64  `json:"class_avg"`
	Grade    string   `json:"grade"`
}

func pct(v float64) float64 { return math.Round(v*10) / 10 }

func (a *App) handleProgress(w http.ResponseWriter, r *http.Request, u *User) {
	sid := pathID(r, "id")
	if !a.canViewStudent(u, sid) {
		errJSON(w, 403, "You can only view your own child's progress")
		return
	}
	student, err := a.queryOne(`SELECT s.id, s.name, s.admission_no, s.class_id, c.name||' - '||c.section AS class_name, t.name AS class_teacher
		FROM students s LEFT JOIN classes c ON c.id=s.class_id LEFT JOIN users t ON t.id=c.teacher_id WHERE s.id=?`, sid)
	if err != nil {
		errJSON(w, 404, "Student not found")
		return
	}
	lang := a.userLanguage(u.ID)
	t := today()
	ago := func(d int) string { return time.Now().AddDate(0, 0, -d).Format(dateLayout) }

	// Attendance: last 30 days against the 30 before.
	attPct := func(from, to string) (float64, float64) {
		total := a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND date BETWEEN ? AND ?`, sid, from, to)
		if total == 0 {
			return -1, 0
		}
		in := a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND date BETWEEN ? AND ? AND status!='absent'`, sid, from, to)
		return pct(in * 100 / total), total
	}
	att30, days30 := attPct(ago(30), t)
	attPrev, _ := attPct(ago(60), ago(31))
	attendance := map[string]any{
		"percent": att30, "previous": attPrev, "days": days30,
		"absent": a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND date BETWEEN ? AND ? AND status='absent'`, sid, ago(30), t),
		"late":   a.scalar(`SELECT COUNT(*) FROM attendance WHERE student_id=? AND date BETWEEN ? AND ? AND status='late'`, sid, ago(30), t),
	}

	// Results per subject: latest and previous published exam.
	rows, err := a.db.Query(`SELECT e.subject, e.title, e.exam_date, e.max_marks, x.marks, x.remarks,
		(SELECT AVG(y.marks) FROM results y WHERE y.exam_id=e.id)
		FROM results x JOIN exams e ON e.id=x.exam_id WHERE x.student_id=? AND e.published=1 ORDER BY e.exam_date DESC`, sid)
	if err != nil {
		serverError(w, err)
		return
	}
	bySubject := map[string]*subjectProgress{}
	var order []string
	remarks := []map[string]string{}
	for rows.Next() {
		var subject, title, date, remark string
		var maxMarks, marks, avg float64
		if rows.Scan(&subject, &title, &date, &maxMarks, &marks, &remark, &avg) != nil || maxMarks <= 0 {
			continue
		}
		p := pct(marks * 100 / maxMarks)
		if sp, ok := bySubject[subject]; ok {
			if sp.Previous == nil {
				sp.Previous = &p
			}
		} else {
			bySubject[subject] = &subjectProgress{Subject: subject, Exam: title, Date: date, Latest: p, ClassAvg: pct(avg * 100 / maxMarks), Grade: grade(p)}
			order = append(order, subject)
		}
		if strings.TrimSpace(remark) != "" && len(remarks) < 6 {
			remarks = append(remarks, map[string]string{"subject": subject, "exam": title, "remark": remark})
		}
	}
	rows.Close()
	subjects := []subjectProgress{}
	var strengths, focus []string
	for _, name := range order {
		sp := bySubject[name]
		subjects = append(subjects, *sp)
		switch {
		case sp.Latest >= 75 || sp.Latest-sp.ClassAvg >= 10:
			strengths = append(strengths, name)
		case sp.Latest < 50 || sp.Latest-sp.ClassAvg <= -10:
			focus = append(focus, name)
		}
	}
	sort.Strings(strengths)
	sort.Strings(focus)

	homework, _ := a.queryMaps(`SELECT subject, title, due_date FROM homework WHERE class_id=? AND due_date>=? ORDER BY due_date LIMIT 10`, student["class_id"], t)

	// Rule-based summary and next steps (always available, in the reader's language).
	var summary []string
	if att30 >= 0 {
		trend := ""
		if attPrev >= 0 && math.Abs(att30-attPrev) >= 3 {
			if att30 > attPrev {
				trend = L(lang, "prog.att.up", fmt.Sprint(attPrev))
			} else {
				trend = L(lang, "prog.att.down", fmt.Sprint(attPrev))
			}
		}
		summary = append(summary, L(lang, "prog.att", fmt.Sprint(att30), trend))
	}
	if len(subjects) == 0 {
		summary = append(summary, L(lang, "prog.noresults"))
	}
	if len(strengths) > 0 {
		summary = append(summary, L(lang, "prog.strong", strings.Join(strengths, ", ")))
	}
	if len(focus) > 0 {
		summary = append(summary, L(lang, "prog.focus", strings.Join(focus, ", ")))
	}
	if len(homework) > 0 {
		summary = append(summary, L(lang, "prog.hw", fmt.Sprint(len(homework))))
	}
	steps := []string{}
	if att30 >= 0 && att30 < 90 {
		steps = append(steps, L(lang, "step.att"))
	}
	for _, f := range focus {
		if len(steps) < 3 {
			steps = append(steps, L(lang, "step.focus", f))
		}
	}
	if len(homework) > 0 && len(steps) < 3 {
		steps = append(steps, L(lang, "step.hw", fmt.Sprint(len(homework))))
	}
	if len(focus) >= 2 || (att30 >= 0 && att30 < 80) {
		steps = append(steps, L(lang, "step.meet"))
	}
	if len(strengths) > 0 && len(steps) < 4 {
		steps = append(steps, L(lang, "step.praise", strings.Join(strengths, ", ")))
	}
	if len(steps) == 0 {
		steps = append(steps, L(lang, "step.keep"))
	}

	out := map[string]any{
		"student": student, "language": lang, "attendance": attendance, "subjects": subjects,
		"strengths": strengths, "focus": focus, "homework": homework, "remarks": remarks,
		"summary": strings.Join(summary, " "), "summary_source": "auto", "steps": steps, "generated": t,
		"ai_available": a.ai.enabled,
	}
	if out["strengths"] == nil {
		out["strengths"] = []string{}
	}
	if out["focus"] == nil {
		out["focus"] = []string{}
	}

	// AI summary on request, cached per child, language and day.
	if r.URL.Query().Get("ai") == "1" && a.ai.enabled {
		var cached string
		a.db.QueryRow(`SELECT text FROM progress_summaries WHERE student_id=? AND language=? AND date=?`, sid, lang, t).Scan(&cached)
		if cached == "" {
			facts, _ := json.Marshal(map[string]any{"student": student["name"], "class": student["class_name"], "attendance_last_30_days": attendance,
				"subjects": subjects, "teacher_remarks": remarks, "upcoming_homework": homework})
			system := "You write short progress summaries of a school child for their parents in Sri Lanka. " + a.schoolContext() + `
Write in ` + lang + ` (native script), warm, honest and plain — no jargon, no percentages overload. 120–170 words.
Cover how attendance is going, strengths, areas that need support, and finish with 2–3 concrete things the family can do at home.
Use only the facts given; do not invent marks, events or diagnoses. Subject names may stay as given. Return only the summary text.`
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
			text, err := a.ai.complete(ctx, system, "Facts:\n"+string(facts), "low", nil)
			cancel()
			if err == nil && text != "" {
				cached = text
				a.db.Exec(`INSERT OR REPLACE INTO progress_summaries(student_id,language,date,text) VALUES(?,?,?,?)`, sid, lang, t, text)
			} else if err != nil {
				out["ai_error"] = err.Error()
			}
		}
		if cached != "" {
			out["summary"], out["summary_source"] = cached, "ai"
		}
	}
	writeJSON(w, 200, out)
}
