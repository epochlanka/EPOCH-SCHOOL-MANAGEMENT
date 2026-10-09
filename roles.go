package main

import (
	"slices"
	"strings"
)

// Permissions. Each role is granted a set of these; handlers ask for one.
const (
	PDashboard     = "dashboard"      // school-wide dashboard
	PUsers         = "users"          // create/edit user accounts
	PSettings      = "settings"       // integrations, database, automation settings
	PSetup         = "setup"          // sections, classes, subjects, bus routes
	PStudentsView  = "students.view"  // browse students
	PStudentsEdit  = "students.edit"  // add/edit students and parents
	PAttendance    = "attendance"     // mark student attendance (scoped to own classes)
	PFeesView      = "fees.view"      // see fees
	PFeesEdit      = "fees.edit"      // issue fees, record payments, reminders
	PAcademic      = "academic"       // homework, exams, results (scoped to own classes)
	PAnnounce      = "announce"       // send messages to own classes
	PAnnounceAll   = "announce.all"   // send to the whole school, all parents, all staff
	PReports       = "reports"        // communication delivery reports
	PTransport     = "transport"      // bus updates
	PStaffView     = "staff.view"     // see teacher attendance
	PStaffMark     = "staff.mark"     // mark teacher attendance
	PLeaveApprove  = "leave.approve"  // approve / reject leave requests
	PTimetableEdit = "timetable.edit" // subject allocation and timetable generation
	PSubstitutions = "substitutions"  // run / change relief-teacher assignments
	PAdmissions    = "admissions"     // enquiries and enrolment
	PLibrary       = "library"        // books and loans
	PAICompose     = "ai.compose"     // AI message writer
	PAIData        = "ai.data"        // AI assistant with school-wide data
)

var allPerms = []string{PDashboard, PUsers, PSettings, PSetup, PStudentsView, PStudentsEdit, PAttendance,
	PFeesView, PFeesEdit, PAcademic, PAnnounce, PAnnounceAll, PReports, PTransport, PStaffView, PStaffMark,
	PLeaveApprove, PTimetableEdit, PSubstitutions, PAdmissions, PLibrary, PAICompose, PAIData}

type roleInfo struct {
	Label    string
	Teaching bool // can be given subjects, a timetable and relief periods
	Perms    []string
}

var roleOrder = []string{"admin", "director", "principal", "vice_principal", "section_head", "class_teacher",
	"subject_teacher", "accountant", "admissions", "front_office", "librarian", "parent", "student"}

var roles = map[string]roleInfo{
	"admin":    {"System Admin", false, allPerms},
	"director": {"Director", false, []string{PDashboard, PStudentsView, PFeesView, PReports, PStaffView, PLeaveApprove, PAnnounce, PAnnounceAll, PAdmissions, PAICompose, PAIData}},
	"principal":      {"Principal", true, without(allPerms, PSettings)},
	"vice_principal": {"Vice Principal", true, without(allPerms, PSettings, PUsers, PFeesEdit)},
	"section_head": {"Head of Section", true, []string{PDashboard, PStudentsView, PAttendance, PAcademic, PAnnounce, PReports, PTransport,
		PStaffView, PStaffMark, PLeaveApprove, PSubstitutions, PAICompose, PAIData}},
	"class_teacher":   {"Class Teacher", true, []string{PStudentsView, PAttendance, PAcademic, PAnnounce, PAICompose, PAIData}},
	"subject_teacher": {"Subject Teacher", true, []string{PStudentsView, PAcademic, PAnnounce, PAICompose}},
	"accountant":      {"Accountant", false, []string{PDashboard, PStudentsView, PFeesView, PFeesEdit, PReports, PAnnounce, PAnnounceAll, PAICompose}},
	"admissions":      {"Admissions Officer", false, []string{PAdmissions, PStudentsView, PStudentsEdit, PAICompose}},
	"front_office":    {"Front Office", false, []string{PStudentsView, PStudentsEdit, PAnnounce, PAnnounceAll, PTransport, PStaffView, PStaffMark, PAdmissions, PAICompose}},
	"librarian":       {"Librarian", false, []string{PLibrary, PStudentsView, PAICompose}},
	"parent":          {"Parent", false, nil},
	"student":         {"Student", false, nil},
}

func without(list []string, drop ...string) []string {
	var out []string
	for _, p := range list {
		if !slices.Contains(drop, p) {
			out = append(out, p)
		}
	}
	return out
}

func validRole(role string) bool { _, ok := roles[role]; return ok }
func isStaff(role string) bool   { return validRole(role) && role != "parent" && role != "student" }
func isTeaching(role string) bool {
	return roles[role].Teaching
}
func roleLabel(role string) string {
	if r, ok := roles[role]; ok {
		return r.Label
	}
	return role
}

// hasPerm checks a permission. "" means any signed-in user; "staff" means any staff role.
func hasPerm(role, perm string) bool {
	switch perm {
	case "":
		return true
	case "staff":
		return isStaff(role)
	}
	return slices.Contains(roles[role].Perms, perm)
}

// SQL fragments for role groups.
var (
	staffRolesSQL    = "('" + strings.Join(filterRoles(isStaff), "','") + "')"
	teachingRolesSQL = "('" + strings.Join(filterRoles(isTeaching), "','") + "')"
)

func filterRoles(keep func(string) bool) []string {
	var out []string
	for _, r := range roleOrder {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// classScope returns the classes a user may act on for a permission. all=true means
// every class. Heads of section see their section; class teachers their own class
// (plus classes they teach, except for attendance); subject teachers the classes they teach.
func (a *App) classScope(u *User, perm string) (all bool, ids []int64) {
	switch u.Role {
	case "section_head":
		ids, _ = a.queryIDs(`SELECT c.id FROM classes c JOIN sections s ON s.id=c.section_id WHERE s.head_id=?
			UNION SELECT id FROM classes WHERE teacher_id=? UNION SELECT class_id FROM class_subjects WHERE teacher_id=?`, u.ID, u.ID, u.ID)
	case "class_teacher":
		if perm == PAttendance {
			ids, _ = a.queryIDs(`SELECT id FROM classes WHERE teacher_id=?`, u.ID)
		} else {
			ids, _ = a.queryIDs(`SELECT id FROM classes WHERE teacher_id=? UNION SELECT class_id FROM class_subjects WHERE teacher_id=?`, u.ID, u.ID)
		}
	case "subject_teacher":
		ids, _ = a.queryIDs(`SELECT class_id FROM class_subjects WHERE teacher_id=? UNION SELECT id FROM classes WHERE teacher_id=?`, u.ID, u.ID)
	default:
		return true, nil
	}
	if ids == nil {
		ids = []int64{}
	}
	return false, ids
}

func (a *App) inClassScope(u *User, perm string, classID int64) bool {
	all, ids := a.classScope(u, perm)
	return all || slices.Contains(ids, classID)
}

// scopeSQL returns " AND <col> IN (...)" for a limited scope, or "" for all classes.
func (a *App) scopeSQL(u *User, perm, col string) (string, []any) {
	all, ids := a.classScope(u, perm)
	if all {
		return "", nil
	}
	return " AND " + col + " IN (" + placeholders(len(ids)) + ")", int64Args(ids)
}

func rolesForClient() []map[string]any {
	out := []map[string]any{}
	for _, r := range roleOrder {
		out = append(out, map[string]any{"id": r, "label": roles[r].Label, "teaching": roles[r].Teaching, "staff": isStaff(r)})
	}
	return out
}
