package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "epoch_session"

type User struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	WhatsApp string `json:"whatsapp"`
	Role     string `json:"role"`
	Language string `json:"language"`
}

type handlerFunc func(w http.ResponseWriter, r *http.Request, u *User)

func hashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type loginLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{hits: map[string][]time.Time{}} }

// blocked reports whether a key (IP+email) has 10 or more failed logins in the last 15 minutes.
// Only failures count, so many people signing in from one school network are not locked out.
func (l *loginLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) == 0 {
		delete(l.hits, key)
		return false
	}
	l.hits[key] = recent
	return len(recent) >= 10
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.hits[key], time.Now())
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	limitKey := clientIP(r) + "|" + strings.ToLower(strings.TrimSpace(req.Email))
	if a.limiter.blocked(limitKey) {
		errJSON(w, 429, "Too many failed login attempts. Please wait 15 minutes and try again.")
		return
	}
	var u User
	var hash string
	var active int
	err := a.db.QueryRow(`SELECT id,name,email,phone,whatsapp,role,language,password_hash,active FROM users WHERE email=?`,
		strings.TrimSpace(req.Email)).Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.WhatsApp, &u.Role, &u.Language, &hash, &active)
	if err != nil || active == 0 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		a.limiter.fail(limitKey)
		errJSON(w, 401, "Invalid email or password")
		return
	}
	token := newToken()
	exp := time.Now().Add(7 * 24 * time.Hour)
	if _, err := a.db.Exec(`INSERT INTO sessions(token,user_id,expires_at) VALUES(?,?,?)`, token, u.ID, exp.Format(tsLayout)); err != nil {
		serverError(w, err)
		return
	}
	a.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now())
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookie,
	})
	writeJSON(w, 200, u)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, u *User) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.db.Exec(`DELETE FROM sessions WHERE token=?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) currentUser(r *http.Request) (*User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, errors.New("no session")
	}
	var u User
	err = a.db.QueryRow(`SELECT u.id,u.name,u.email,u.phone,u.whatsapp,u.role,u.language
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token=? AND s.expires_at > ? AND u.active=1`, c.Value, now()).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.WhatsApp, &u.Role, &u.Language)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// auth wraps a handler with session authentication, a permission check and a JSON
// content-type requirement on state-changing requests (CSRF defence).
func (a *App) auth(perm string, fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := a.currentUser(r)
		if err != nil {
			errJSON(w, 401, "Please sign in")
			return
		}
		if !hasPerm(u.Role, perm) {
			errJSON(w, 403, "You do not have permission to do this")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			errJSON(w, 415, "Content-Type must be application/json")
			return
		}
		fn(w, r, u)
	})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request, u *User) {
	children := []map[string]any{}
	var err error
	switch u.Role {
	case "parent":
		children, err = a.queryMaps(`SELECT s.id,s.name,s.admission_no,s.gender,c.name||' - '||c.section AS class_name
			FROM students s LEFT JOIN classes c ON c.id=s.class_id WHERE s.parent_id=? ORDER BY s.name`, u.ID)
	case "student":
		children, err = a.queryMaps(`SELECT s.id,s.name,s.admission_no,s.gender,c.name||' - '||c.section AS class_name
			FROM students s LEFT JOIN classes c ON c.id=s.class_id WHERE s.user_id=?`, u.ID)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	unread := a.scalar(`SELECT COUNT(*) FROM notifications WHERE user_id=? AND is_read=0`, u.ID)
	unreadMsg := a.scalar(`SELECT COUNT(*) FROM messages WHERE recipient_id=? AND is_read=0`, u.ID)
	writeJSON(w, 200, map[string]any{
		"user": u, "children": children,
		"unread_notifications": unread, "unread_messages": unreadMsg,
		"school": map[string]string{
			"name": a.setting("school_name"), "phone": a.setting("school_phone"),
			"email": a.setting("school_email"), "address": a.setting("school_address"),
		},
		"ai_enabled": a.ai.enabled,
		"role_label": roleLabel(u.Role), "perms": roles[u.Role].Perms, "staff": isStaff(u.Role), "teaching": isTeaching(u.Role),
		"roles":         rolesForClient(),
		"pending_leave": a.pendingLeaveCount(u),
	})
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Phone    string `json:"phone"`
		WhatsApp string `json:"whatsapp"`
		Language string `json:"language"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if req.Language == "" {
		req.Language = "English"
	}
	if _, err := a.db.Exec(`UPDATE users SET phone=?, whatsapp=?, language=? WHERE id=?`,
		strings.TrimSpace(req.Phone), strings.TrimSpace(req.WhatsApp), req.Language, u.ID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if len(req.New) < 8 {
		errJSON(w, 400, "New password must be at least 8 characters")
		return
	}
	var hash string
	a.db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, u.ID).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Current)) != nil {
		errJSON(w, 400, "Current password is incorrect")
		return
	}
	newHash, err := hashPassword(req.New)
	if err != nil {
		serverError(w, err)
		return
	}
	a.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, newHash, u.ID)
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.db.Exec(`DELETE FROM sessions WHERE user_id=? AND token<>?`, u.ID, c.Value)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// canViewStudent reports whether u may see data for the given student.
func (a *App) canViewStudent(u *User, studentID int64) bool {
	switch u.Role {
	case "parent":
		return a.scalar(`SELECT COUNT(*) FROM students WHERE id=? AND parent_id=?`, studentID, u.ID) > 0
	case "student":
		return a.scalar(`SELECT COUNT(*) FROM students WHERE id=? AND user_id=?`, studentID, u.ID) > 0
	}
	if !hasPerm(u.Role, PStudentsView) {
		return false
	}
	var classID int64
	a.db.QueryRow(`SELECT COALESCE(class_id,0) FROM students WHERE id=?`, studentID).Scan(&classID)
	return a.inClassScope(u, PStudentsView, classID)
}
