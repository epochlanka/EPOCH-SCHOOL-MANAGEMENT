// Epoch School Communication & Management System.
// A single Go binary that serves the web UI and JSON API, stores data in SQLite,
// delivers notifications over App / SMS / WhatsApp and integrates Claude for AI features.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
}

func main() {
	loadDotEnv(".env")
	cfg := loadConfig()
	if loc, err := time.LoadLocation(cfg.TZ); err == nil {
		time.Local = loc
	} else {
		log.Printf("unknown TZ %q, using system time zone", cfg.TZ)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := openDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	app := &App{db: db, cfg: cfg, limiter: newLoginLimiter()}
	app.notify = newNotifier(app)
	app.ai = newAI(app)
	if err := app.seed(); err != nil {
		log.Fatalf("seed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.notify.run(ctx)
	go app.runScheduler(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("Epoch School System running on http://localhost%s", displayAddr(cfg.Addr))
	log.Printf("SMS provider: %s | WhatsApp provider: %s | AI: %s", app.notify.sms.Name(), app.notify.wa.Name(), app.ai.statusText())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
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
	any := []string{}
	staff := []string{"admin", "teacher"}
	admin := []string{"admin"}
	h := func(pattern string, roles []string, fn handlerFunc) { mux.Handle(pattern, a.auth(roles, fn)) }

	// Auth & profile
	mux.HandleFunc("POST /api/login", a.handleLogin)
	h("POST /api/logout", any, a.handleLogout)
	h("GET /api/me", any, a.handleMe)
	h("PUT /api/me", any, a.handleUpdateMe)
	h("POST /api/me/password", any, a.handleChangePassword)

	// Dashboard & administration
	h("GET /api/dashboard", staff, a.handleDashboard)
	h("GET /api/users", staff, a.handleListUsers)
	h("POST /api/users", admin, a.handleCreateUser)
	h("PUT /api/users/{id}", admin, a.handleUpdateUser)
	h("DELETE /api/users/{id}", admin, a.handleDeleteUser)
	h("GET /api/classes", staff, a.handleListClasses)
	h("POST /api/classes", admin, a.handleSaveClass)
	h("PUT /api/classes/{id}", admin, a.handleSaveClass)
	h("DELETE /api/classes/{id}", admin, a.handleDeleteClass)
	h("GET /api/routes", staff, a.handleListRoutes)
	h("POST /api/routes", admin, a.handleSaveRoute)
	h("PUT /api/routes/{id}", admin, a.handleSaveRoute)
	h("DELETE /api/routes/{id}", admin, a.handleDeleteRoute)
	h("GET /api/students", staff, a.handleListStudents)
	h("POST /api/students", admin, a.handleSaveStudent)
	h("PUT /api/students/{id}", admin, a.handleSaveStudent)
	h("DELETE /api/students/{id}", admin, a.handleDeleteStudent)
	h("GET /api/settings", any, a.handleGetSettings)
	h("PUT /api/settings", admin, a.handlePutSettings)
	h("POST /api/settings/test-message", admin, a.handleTestMessage)

	// Academic
	h("GET /api/attendance", staff, a.handleGetAttendance)
	h("POST /api/attendance", staff, a.handleSaveAttendance)
	h("GET /api/attendance/report", staff, a.handleAttendanceReport)
	h("GET /api/fees", staff, a.handleListFees)
	h("POST /api/fees", admin, a.handleCreateFees)
	h("POST /api/fees/{id}/pay", admin, a.handlePayFee)
	h("POST /api/fees/{id}/remind", admin, a.handleRemindFee)
	h("POST /api/fees/remind-due", admin, a.handleRemindDue)
	h("DELETE /api/fees/{id}", admin, a.handleDeleteFee)
	h("GET /api/homework", staff, a.handleListHomework)
	h("POST /api/homework", staff, a.handleCreateHomework)
	h("DELETE /api/homework/{id}", staff, a.handleDeleteHomework)
	h("GET /api/exams", staff, a.handleListExams)
	h("POST /api/exams", staff, a.handleCreateExam)
	h("DELETE /api/exams/{id}", staff, a.handleDeleteExam)
	h("GET /api/exams/{id}/results", staff, a.handleGetResults)
	h("POST /api/exams/{id}/results", staff, a.handleSaveResults)

	// Communication
	h("GET /api/bus-updates", staff, a.handleListBusUpdates)
	h("POST /api/routes/{id}/updates", staff, a.handleCreateBusUpdate)
	h("GET /api/announcements", staff, a.handleListAnnouncements)
	h("POST /api/announcements", staff, a.handleCreateAnnouncement)
	h("GET /api/notifications", any, a.handleListNotifications)
	h("POST /api/notifications/read-all", any, a.handleReadAllNotifications)
	h("POST /api/notifications/{id}/read", any, a.handleReadNotification)
	h("GET /api/messages", any, a.handleConversations)
	h("GET /api/messages/contacts", any, a.handleContacts)
	h("GET /api/messages/thread/{id}", any, a.handleThread)
	h("POST /api/messages", any, a.handleSendMessage)
	h("GET /api/reports/communications", staff, a.handleCommReport)

	// Parent / student portal
	h("GET /api/portal/{id}", any, a.handlePortal)

	// AI (Claude)
	h("GET /api/ai/status", any, a.handleAIStatus)
	h("POST /api/ai/compose", staff, a.handleAICompose)
	h("POST /api/ai/translate", any, a.handleAITranslate)
	h("POST /api/ai/reply", any, a.handleAIReply)
	h("POST /api/ai/chat", any, a.handleAIChat)

	static, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(static)))
	return logRequests(securityHeaders(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
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
