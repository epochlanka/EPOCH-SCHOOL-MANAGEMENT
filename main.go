// Epoch School Communication & Management System.
// A single Go binary that serves the web UI and JSON API, stores data in SQLite,
// delivers notifications over App / SMS / WhatsApp and integrates Claude for AI features.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"
)

//go:embed web
var webFS embed.FS

type App struct {
	db      *sql.DB
	cfg     Config
	notify  *Notifier
	ai      *AI
	limiter *loginLimiter

	store    *storage
	dbPath   string
	dbSource string          // "env", "settings" or "default"
	restart  func(*dbChange) // stops the server so it can reopen with another database
}

func main() {
	loadDotEnv(".env")
	cfg := loadConfig()
	if loc, err := time.LoadLocation(cfg.TZ); err == nil {
		time.Local = loc
	} else {
		log.Printf("unknown TZ %q, using system time zone", cfg.TZ)
	}
	store, err := loadStorage(cfg.ConfigFile)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// serve returns a change when the admin picks another database in Settings;
	// the server then starts again on the new file.
	for {
		change, err := serve(ctx, cfg, store)
		if err != nil {
			log.Fatal(err)
		}
		if change == nil {
			return
		}
		log.Printf("Restarting...")
	}
}

func serve(ctx context.Context, cfg Config, store *storage) (*dbChange, error) {
	dbPath, source := store.dbPath(cfg)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	migrateLegacyDB(dbPath, source)
	db, err := openDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("database %s: %w", dbPath, err)
	}
	defer db.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	changes := make(chan *dbChange, 1)
	app := &App{db: db, cfg: cfg, limiter: newLoginLimiter(), store: store, dbPath: dbPath, dbSource: source}
	app.restart = func(c *dbChange) {
		select {
		case changes <- c:
			cancel()
		default: // a change is already in progress
		}
	}
	app.notify = newNotifier(app)
	app.ai = newAI(app)
	if err := app.seed(); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}

	var workers sync.WaitGroup
	workers.Go(func() { app.notify.run(runCtx) })
	workers.Go(func() { app.runScheduler(runCtx) })
	workers.Go(func() { app.runBackups(runCtx) })

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-runCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("Epoch School System running on http://localhost%s", displayAddr(cfg.Addr))
	log.Printf("Database: %s (%s)", dbPath, source)
	log.Printf("SMS provider: %s | WhatsApp provider: %s | AI: %s", app.notify.sms.Name(), app.notify.wa.Name(), app.ai.statusText())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return nil, err
	}
	<-stopped
	workers.Wait()

	select {
	case c := <-changes:
		applyDBChange(store, db, c)
		return c, nil
	default:
		return nil, nil
	}
}

func displayAddr(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return addr
	}
	return "/" + addr
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	h := func(pattern, perm string, fn handlerFunc) { mux.Handle(pattern, a.auth(perm, fn)) }
	const anyone, staff = "", "staff"

	// Auth & profile
	mux.HandleFunc("POST /api/login", a.handleLogin)
	h("POST /api/logout", anyone, a.handleLogout)
	h("GET /api/me", anyone, a.handleMe)
	h("PUT /api/me", anyone, a.handleUpdateMe)
	h("POST /api/me/password", anyone, a.handleChangePassword)

	// Dashboard & administration
	h("GET /api/dashboard", PDashboard, a.handleDashboard)
	h("GET /api/users", staff, a.handleListUsers)
	h("POST /api/users", PUsers, a.handleCreateUser)
	h("PUT /api/users/{id}", PUsers, a.handleUpdateUser)
	h("DELETE /api/users/{id}", PUsers, a.handleDeleteUser)
	h("GET /api/staff", staff, a.handleStaffList)
	h("GET /api/sections", staff, a.handleListSections)
	h("POST /api/sections", PSetup, a.handleSaveSection)
	h("PUT /api/sections/{id}", PSetup, a.handleSaveSection)
	h("DELETE /api/sections/{id}", PSetup, a.handleDeleteSection)
	h("GET /api/classes", staff, a.handleListClasses)
	h("POST /api/classes", PSetup, a.handleSaveClass)
	h("PUT /api/classes/{id}", PSetup, a.handleSaveClass)
	h("DELETE /api/classes/{id}", PSetup, a.handleDeleteClass)
	h("GET /api/routes", staff, a.handleListRoutes)
	h("POST /api/routes", PSetup, a.handleSaveRoute)
	h("PUT /api/routes/{id}", PSetup, a.handleSaveRoute)
	h("DELETE /api/routes/{id}", PSetup, a.handleDeleteRoute)
	h("GET /api/students", PStudentsView, a.handleListStudents)
	h("POST /api/students", PStudentsEdit, a.handleSaveStudent)
	h("PUT /api/students/{id}", PStudentsEdit, a.handleSaveStudent)
	h("DELETE /api/students/{id}", PStudentsEdit, a.handleDeleteStudent)
	h("GET /api/settings", anyone, a.handleGetSettings)
	h("PUT /api/settings", PSettings, a.handlePutSettings)
	h("POST /api/settings/test-message", PSettings, a.handleTestMessage)
	h("GET /api/system/storage", PSettings, a.handleGetStorage)
	h("PUT /api/system/storage", PSettings, a.handlePutStorage)
	h("POST /api/system/backup", PSettings, a.handleBackupNow)
	h("GET /api/system/backup/download", PSettings, a.handleDownloadBackup)
	h("POST /api/system/database", PSettings, a.handleChangeDatabase)

	// Subjects, timetable, staff attendance, leave, substitutions
	h("GET /api/subjects", staff, a.handleListSubjects)
	h("POST /api/subjects", PSetup, a.handleSaveSubject)
	h("PUT /api/subjects/{id}", PSetup, a.handleSaveSubject)
	h("DELETE /api/subjects/{id}", PSetup, a.handleDeleteSubject)
	h("GET /api/class-subjects", staff, a.handleListClassSubjects)
	h("POST /api/class-subjects", PTimetableEdit, a.handleSaveClassSubject)
	h("DELETE /api/class-subjects/{id}", PTimetableEdit, a.handleDeleteClassSubject)
	h("GET /api/timetable", anyone, a.handleTimetable)
	h("POST /api/timetable/generate", PTimetableEdit, a.handleGenerateTimetable)
	h("PUT /api/timetable/slot", PTimetableEdit, a.handleSetSlot)
	h("GET /api/workspace", staff, a.handleWorkspace)
	h("POST /api/staff-attendance/check-in", staff, a.handleCheckIn)
	h("GET /api/staff-attendance", PStaffView, a.handleStaffAttendance)
	h("POST /api/staff-attendance", PStaffMark, a.handleSaveStaffAttendance)
	h("GET /api/leave", staff, a.handleListLeave)
	h("POST /api/leave", staff, a.handleApplyLeave)
	h("POST /api/leave/{id}/decision", PLeaveApprove, a.handleLeaveDecision)
	h("POST /api/leave/{id}/cancel", staff, a.handleCancelLeave)
	h("GET /api/substitutions", staff, a.handleListSubstitutions)
	h("POST /api/substitutions/auto", PSubstitutions, a.handleAutoSubstitutions)
	h("GET /api/substitutions/free-teachers", PSubstitutions, a.handleFreeTeachers)
	h("PUT /api/substitutions/{id}", PSubstitutions, a.handleSetSubstitute)

	// Admissions & library
	h("GET /api/admissions", PAdmissions, a.handleListAdmissions)
	h("POST /api/admissions", PAdmissions, a.handleSaveAdmission)
	h("PUT /api/admissions/{id}", PAdmissions, a.handleSaveAdmission)
	h("POST /api/admissions/{id}/enrol", PAdmissions, a.handleEnrol)
	h("DELETE /api/admissions/{id}", PAdmissions, a.handleDeleteAdmission)
	h("GET /api/books", PLibrary, a.handleListBooks)
	h("POST /api/books", PLibrary, a.handleSaveBook)
	h("PUT /api/books/{id}", PLibrary, a.handleSaveBook)
	h("DELETE /api/books/{id}", PLibrary, a.handleDeleteBook)
	h("GET /api/loans", PLibrary, a.handleListLoans)
	h("POST /api/loans", PLibrary, a.handleIssueBook)
	h("POST /api/loans/{id}/return", PLibrary, a.handleReturnBook)
	h("POST /api/loans/remind-overdue", PLibrary, a.handleRemindOverdueBooks)

	// Academic
	h("GET /api/attendance", PAttendance, a.handleGetAttendance)
	h("POST /api/attendance", PAttendance, a.handleSaveAttendance)
	h("GET /api/attendance/report", PStudentsView, a.handleAttendanceReport)
	h("GET /api/fees", PFeesView, a.handleListFees)
	h("POST /api/fees", PFeesEdit, a.handleCreateFees)
	h("POST /api/fees/{id}/pay", PFeesEdit, a.handlePayFee)
	h("POST /api/fees/{id}/remind", PFeesEdit, a.handleRemindFee)
	h("POST /api/fees/remind-due", PFeesEdit, a.handleRemindDue)
	h("DELETE /api/fees/{id}", PFeesEdit, a.handleDeleteFee)
	h("GET /api/homework", PAcademic, a.handleListHomework)
	h("POST /api/homework", PAcademic, a.handleCreateHomework)
	h("DELETE /api/homework/{id}", PAcademic, a.handleDeleteHomework)
	h("GET /api/exams", PAcademic, a.handleListExams)
	h("POST /api/exams", PAcademic, a.handleCreateExam)
	h("DELETE /api/exams/{id}", PAcademic, a.handleDeleteExam)
	h("GET /api/exams/{id}/results", PAcademic, a.handleGetResults)
	h("POST /api/exams/{id}/results", PAcademic, a.handleSaveResults)

	// Communication
	h("GET /api/bus-updates", staff, a.handleListBusUpdates)
	h("POST /api/routes/{id}/updates", PTransport, a.handleCreateBusUpdate)
	h("GET /api/announcements", PAnnounce, a.handleListAnnouncements)
	h("POST /api/announcements", PAnnounce, a.handleCreateAnnouncement)
	h("GET /api/notifications", anyone, a.handleListNotifications)
	h("POST /api/notifications/read-all", anyone, a.handleReadAllNotifications)
	h("POST /api/notifications/{id}/read", anyone, a.handleReadNotification)
	h("GET /api/messages", anyone, a.handleConversations)
	h("GET /api/messages/contacts", anyone, a.handleContacts)
	h("GET /api/messages/thread/{id}", anyone, a.handleThread)
	h("POST /api/messages", anyone, a.handleSendMessage)
	h("GET /api/reports/communications", PReports, a.handleCommReport)

	// Parent / student portal
	h("GET /api/portal/{id}", anyone, a.handlePortal)

	// AI (Claude)
	h("GET /api/ai/status", anyone, a.handleAIStatus)
	h("POST /api/ai/compose", PAICompose, a.handleAICompose)
	h("POST /api/ai/translate", anyone, a.handleAITranslate)
	h("POST /api/ai/reply", anyone, a.handleAIReply)
	h("POST /api/ai/chat", anyone, a.handleAIChat)

	// The UI is embedded in the binary; set WEB_DIR=web during development to serve it from disk.
	if dir := os.Getenv("WEB_DIR"); dir != "" {
		mux.Handle("/", http.FileServer(http.Dir(dir)))
	} else {
		static, _ := fs.Sub(webFS, "web")
		mux.Handle("/", http.FileServer(http.FS(static)))
	}
	return logRequests(securityHeaders(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if len(r.URL.Path) > 4 && r.URL.Path[:5] == "/api/" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}
