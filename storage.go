package main

// Storage: where the SQLite database lives, the small config file that remembers
// that location (it can't live inside the database itself), and database backups.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	legacyDBPath = "data/epoch.db" // where older versions kept the database (inside the project folder)
	backupPrefix = "epoch-backup-"
)

// storeConfig is saved as JSON in the config file (EPOCH_CONFIG, default <data dir>/config.json).
type storeConfig struct {
	DBPath      string `json:"db_path,omitempty"`
	BackupDir   string `json:"backup_dir,omitempty"`
	BackupDaily bool   `json:"backup_daily"`
	BackupKeep  int    `json:"backup_keep"`
	LastBackup  string `json:"last_backup,omitempty"`
}

// dbChange is a request to switch to another database file, applied by restarting the server.
type dbChange struct {
	path string
	copy bool // copy the current data to path; otherwise open the database already at path
}

// storage is shared by the App and the restart loop in main.
type storage struct {
	file   string
	mu     sync.Mutex
	cfg    storeConfig
	notice string // result of the last database change, shown in Settings
}

// appDataDir is the per-machine folder for the database and config, outside the project folder.
func appDataDir() string {
	if d := env("EPOCH_DATA_DIR", ""); d != "" {
		return d
	}
	if runtime.GOOS == "linux" {
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "epoch-school")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", "epoch-school")
		}
	} else if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "EpochSchool")
	}
	return "data"
}

func loadStorage(file string) (*storage, error) {
	s := &storage{file: file, cfg: storeConfig{BackupKeep: 14}}
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return s, nil
}

func (s *storage) get() storeConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *storage) update(fn func(c *storeConfig)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	fn(&next)
	b, _ := json.MarshalIndent(next, "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.file); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

// dbPath picks the database file: DB_PATH from the environment wins, then the path
// chosen in Settings, then the default in the data folder.
func (s *storage) dbPath(cfg Config) (path, source string) {
	switch {
	case cfg.DBPath != "":
		path, source = cfg.DBPath, "env"
	case s.get().DBPath != "":
		path, source = s.get().DBPath, "settings"
	default:
		path, source = filepath.Join(appDataDir(), "epoch.db"), "default"
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return path, source
}

func (s *storage) backupDir(dbPath string) string {
	if d := s.get().BackupDir; d != "" {
		return d
	}
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// migrateLegacyDB copies a database left in the project's data/ folder to the new
// default location the first time the system starts without one there.
func migrateLegacyDB(path, source string) {
	if source != "default" || fileExists(path) || !fileExists(legacyDBPath) {
		return
	}
	old, err := sql.Open("sqlite", "file:"+legacyDBPath+"?_pragma=busy_timeout(10000)")
	if err != nil {
		log.Printf("legacy database: %v", err)
		return
	}
	defer old.Close()
	if err := snapshotDB(old, path); err != nil {
		log.Printf("could not move %s to %s: %v", legacyDBPath, path, err)
		return
	}
	log.Printf("Copied existing database %s to %s (the old file can be deleted once you've checked everything is there)", legacyDBPath, path)
}

// snapshotDB writes a consistent copy of a live database to dest (which must not exist).
// It is safe to run while the system is in use, unlike copying the .db file directly.
func snapshotDB(db *sql.DB, dest string) error {
	if fileExists(dest) {
		return fmt.Errorf("%s already exists", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".partial"
	os.Remove(tmp)
	if _, err := db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// checkWritableDir makes sure files can be created in dir.
func checkWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".epoch-write-test-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// checkEpochDB confirms that path is an existing Epoch School database.
func checkEpochDB(path string) error {
	if info, err := os.Stat(path); err != nil {
		return errors.New("file not found")
	} else if info.IsDir() {
		return errors.New("that is a folder, not a database file")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n); err != nil {
		return errors.New("this file is not an Epoch School database")
	}
	if n == 0 {
		return errors.New("this database has no admin account, so nobody could sign in")
	}
	return nil
}

type backupFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Created string `json:"created"`
}

func listBackups(dir string) []backupFile {
	out := []backupFile{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), backupPrefix) || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, backupFile{e.Name(), info.Size(), info.ModTime().Format(tsLayout)})
		}
	}
	slices.SortFunc(out, func(a, b backupFile) int { return strings.Compare(b.Name, a.Name) })
	return out
}

// backupNow saves a snapshot to the backup folder and removes the oldest beyond the keep limit.
func (a *App) backupNow() (string, error) {
	dir := a.store.backupDir(a.dbPath)
	stamp := backupPrefix + time.Now().Format("2006-01-02-150405")
	dest := filepath.Join(dir, stamp+".db")
	for i := 2; fileExists(dest); i++ {
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d.db", stamp, i))
	}
	if err := snapshotDB(a.db, dest); err != nil {
		return "", err
	}
	if err := a.store.update(func(c *storeConfig) { c.LastBackup = now() }); err != nil {
		log.Printf("config: %v", err)
	}
	if keep := a.store.get().BackupKeep; keep > 0 {
		backups := listBackups(dir)
		for i := keep; i < len(backups); i++ {
			if p := filepath.Join(dir, backups[i].Name); p != a.dbPath { // a restored backup may be the live database
				os.Remove(p)
			}
		}
	}
	return dest, nil
}

// runBackups makes the daily automatic backup when it is turned on.
func (a *App) runBackups(ctx context.Context) {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for {
		c := a.store.get()
		if c.BackupDaily && !strings.HasPrefix(c.LastBackup, today()) {
			if dest, err := a.backupNow(); err != nil {
				log.Printf("daily backup: %v", err)
			} else {
				log.Printf("Daily backup saved to %s", dest)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// ---------- Handlers (admin only) ----------

func (a *App) handleGetStorage(w http.ResponseWriter, r *http.Request, u *User) {
	c := a.store.get()
	var size int64
	for _, suffix := range []string{"", "-wal"} {
		if info, err := os.Stat(a.dbPath + suffix); err == nil {
			size += info.Size()
		}
	}
	dir := a.store.backupDir(a.dbPath)
	a.store.mu.Lock()
	notice := a.store.notice
	a.store.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"db_path": a.dbPath, "db_source": a.dbSource, "db_size": size, "config_file": a.store.file,
		"backup_dir": dir, "backup_dir_custom": c.BackupDir != "", "backup_daily": c.BackupDaily, "backup_keep": c.BackupKeep,
		"last_backup": c.LastBackup, "backups": listBackups(dir), "notice": notice,
	})
}

func (a *App) handlePutStorage(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		BackupDir   string `json:"backup_dir"`
		BackupDaily bool   `json:"backup_daily"`
		BackupKeep  int    `json:"backup_keep"`
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	dir := strings.TrimSpace(req.BackupDir)
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if err := checkWritableDir(abs); err != nil {
			errJSON(w, 400, "Can't write to the backup folder: "+err.Error())
			return
		}
		if abs == filepath.Join(filepath.Dir(a.dbPath), "backups") {
			abs = "" // same as the default, so keep following the database
		}
		dir = abs
	}
	err := a.store.update(func(c *storeConfig) {
		c.BackupDir, c.BackupDaily, c.BackupKeep = dir, req.BackupDaily, max(req.BackupKeep, 0)
	})
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleBackupNow(w http.ResponseWriter, r *http.Request, u *User) {
	dest, err := a.backupNow()
	if err != nil {
		errJSON(w, 500, "Backup failed: "+err.Error())
		return
	}
	log.Printf("Backup saved to %s by %s", dest, u.Email)
	writeJSON(w, 200, map[string]string{"path": dest})
}

// handleDownloadBackup streams a fresh snapshot of the database to the browser.
func (a *App) handleDownloadBackup(w http.ResponseWriter, r *http.Request, u *User) {
	tmpDir, err := os.MkdirTemp("", "epoch-backup")
	if err != nil {
		serverError(w, err)
		return
	}
	defer os.RemoveAll(tmpDir)
	name := backupPrefix + time.Now().Format("2006-01-02-150405") + ".db"
	path := filepath.Join(tmpDir, name)
	if err := snapshotDB(a.db, path); err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}

// handleChangeDatabase switches to another database file. The server restarts itself
// (in-process, a few seconds) so nothing is written to the old file after the copy.
func (a *App) handleChangeDatabase(w http.ResponseWriter, r *http.Request, u *User) {
	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"` // "copy" or "open"
	}
	if err := readJSON(r, &req); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if a.dbSource == "env" {
		errJSON(w, 409, "The database path is fixed by DB_PATH in the server environment or .env file. Remove it there to choose the location here.")
		return
	}
	p := strings.TrimSpace(req.Path)
	if p == "" {
		errJSON(w, 400, "Enter a file path for the database")
		return
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	p, err := filepath.Abs(p)
	if err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		p = filepath.Join(p, "epoch.db")
	}
	if p == a.dbPath {
		errJSON(w, 400, "That is already the current database")
		return
	}
	change := &dbChange{path: p, copy: req.Mode != "open"}
	if change.copy {
		if fileExists(p) {
			errJSON(w, 400, p+" already exists. Choose \"Open the database already there\" to use it, or pick another name.")
			return
		}
		if err := checkWritableDir(filepath.Dir(p)); err != nil {
			errJSON(w, 400, "Can't write to that folder: "+err.Error())
			return
		}
	} else if err := checkEpochDB(p); err != nil {
		errJSON(w, 400, "Can't use "+p+": "+err.Error())
		return
	}
	log.Printf("%s requested database change to %s (copy=%v)", u.Email, p, change.copy)
	writeJSON(w, 202, map[string]any{"path": p, "restarting": true})
	go func() {
		time.Sleep(300 * time.Millisecond) // let the response reach the browser
		a.restart(change)
	}()
}

// applyDBChange runs after the server has stopped: it copies the data if asked and
// records the new path. On failure the old database stays in use.
func applyDBChange(st *storage, db *sql.DB, c *dbChange) {
	setNotice := func(msg string) {
		st.mu.Lock()
		st.notice = msg
		st.mu.Unlock()
		log.Print(msg)
	}
	if c.copy {
		if err := snapshotDB(db, c.path); err != nil {
			setNotice("Database was not moved: " + err.Error())
			return
		}
	}
	if err := st.update(func(cfg *storeConfig) { cfg.DBPath = c.path }); err != nil {
		setNotice("Database location was not saved: " + err.Error())
		return
	}
	if c.copy {
		setNotice("Data copied to " + c.path + ". The previous file was left in place as a fallback.")
	} else {
		setNotice("Now using the database at " + c.path)
	}
}
