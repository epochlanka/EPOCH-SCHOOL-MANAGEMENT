package main

// Storage: which PostgreSQL database is in use, the small config file for backup
// settings (kept outside the database so it survives a restore), and backups made
// with pg_dump.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const backupPrefix = "epoch-backup-"

// storeConfig is saved as JSON in the config file (EPOCH_CONFIG, default <data dir>/config.json).
type storeConfig struct {
	DBPath      string `json:"db_path,omitempty"` // SQLite file used by earlier versions (only read to import it)
	BackupDir   string `json:"backup_dir,omitempty"`
	BackupDaily bool   `json:"backup_daily"`
	BackupKeep  int    `json:"backup_keep"`
	LastBackup  string `json:"last_backup,omitempty"`
}

type storage struct {
	file   string
	mu     sync.Mutex
	cfg    storeConfig
	notice string // result of the SQLite import, shown in Settings
}

// appDataDir is the per-machine folder for the config file and backups, outside the project folder.
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

func (s *storage) setNotice(msg string) {
	s.mu.Lock()
	s.notice = msg
	s.mu.Unlock()
}

func (s *storage) backupDir() string {
	if d := s.get().BackupDir; d != "" {
		return d
	}
	return filepath.Join(appDataDir(), "backups")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// describeDB names the database for the Settings page without revealing the password.
func describeDB(dbURL string) string {
	c, err := pgx.ParseConfig(dbURL)
	if err != nil {
		return "PostgreSQL"
	}
	where := fmt.Sprintf("%s:%d", c.Host, c.Port)
	if strings.HasPrefix(c.Host, "/") {
		where = "this computer"
	}
	return fmt.Sprintf("PostgreSQL database %q on %s (user %s)", c.Database, where, c.User)
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

type backupFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Created string `json:"created"`
}

func listBackups(dir string) []backupFile {
	out := []backupFile{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), backupPrefix) || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, backupFile{e.Name(), info.Size(), info.ModTime().Format(tsLayout)})
		}
	}
	slices.SortFunc(out, func(a, b backupFile) int { return strings.Compare(b.Name, a.Name) })
	return out
}

// pgDump writes a plain-SQL dump of the database to dest using pg_dump. The password
// is passed in the environment rather than on the command line.
func pgDump(ctx context.Context, dbURL, dest string) error {
	bin, err := exec.LookPath("pg_dump")
	if err != nil {
		return errors.New("pg_dump was not found; install the PostgreSQL client tools (postgresql-client) on the server")
	}
	cmd := exec.CommandContext(ctx, bin, "--no-owner", "--no-privileges", "--file", dest)
	cmd.Env = os.Environ()
	if u, err := url.Parse(dbURL); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		if pw, ok := u.User.Password(); ok {
			cmd.Env = append(cmd.Env, "PGPASSWORD="+pw)
			u.User = url.User(u.User.Username())
		}
		dbURL = u.String()
	}
	cmd.Args = append(cmd.Args, "--dbname", dbURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_dump: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// backupNow saves a dump to the backup folder and removes the oldest beyond the keep limit.
func (a *App) backupNow() (string, error) {
	dir := a.store.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamp := backupPrefix + time.Now().Format("2006-01-02-150405")
	dest := filepath.Join(dir, stamp+".sql")
	for i := 2; fileExists(dest); i++ {
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d.sql", stamp, i))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	tmp := dest + ".partial"
	if err := pgDump(ctx, a.cfg.DatabaseURL, tmp); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	if err := a.store.update(func(c *storeConfig) { c.LastBackup = now() }); err != nil {
		log.Printf("config: %v", err)
	}
	if keep := a.store.get().BackupKeep; keep > 0 {
		backups := listBackups(dir)
		for i := keep; i < len(backups); i++ {
			os.Remove(filepath.Join(dir, backups[i].Name))
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
	size := a.scalar(`SELECT pg_database_size(current_database())`)
	_, dumpErr := exec.LookPath("pg_dump")
	dir := a.store.backupDir()
	a.store.mu.Lock()
	notice := a.store.notice
	a.store.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"db_desc": describeDB(a.cfg.DatabaseURL), "db_size": size, "config_file": a.store.file, "pg_dump": dumpErr == nil,
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
		if abs == filepath.Join(appDataDir(), "backups") {
			abs = "" // same as the default
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

// handleDownloadBackup sends a fresh dump of the database to the browser.
func (a *App) handleDownloadBackup(w http.ResponseWriter, r *http.Request, u *User) {
	tmpDir, err := os.MkdirTemp("", "epoch-backup")
	if err != nil {
		serverError(w, err)
		return
	}
	defer os.RemoveAll(tmpDir)
	name := backupPrefix + time.Now().Format("2006-01-02-150405") + ".sql"
	path := filepath.Join(tmpDir, name)
	if err := pgDump(r.Context(), a.cfg.DatabaseURL, path); err != nil {
		errJSON(w, 500, "Backup failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/sql")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}
