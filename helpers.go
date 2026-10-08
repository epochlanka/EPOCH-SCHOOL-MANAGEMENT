package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	tsLayout   = "2006-01-02 15:04:05"
	dateLayout = "2006-01-02"
)

func now() string   { return time.Now().Format(tsLayout) }
func today() string { return time.Now().Format(dateLayout) }

// prettyDate turns 2026-10-22 into "22 Oct 2026".
func prettyDate(d string) string {
	t, err := time.Parse(dateLayout, d)
	if err != nil {
		return d
	}
	return t.Format("02 Jan 2006")
}

func validDate(d string) bool {
	_, err := time.Parse(dateLayout, d)
	return err == nil
}

// money formats an amount as "Rs. 12,000.00".
func money(v float64) string {
	whole := int64(math.Floor(v))
	cents := int64(math.Round((v - float64(whole)) * 100))
	s := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return fmt.Sprintf("Rs. %s.%02d", b.String(), cents)
}

func grade(pct float64) string {
	switch {
	case pct >= 75:
		return "A"
	case pct >= 65:
		return "B"
	case pct >= 50:
		return "C"
	case pct >= 35:
		return "S"
	default:
		return "W"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	errJSON(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
}

func readJSON(r *http.Request, v any) error {
	body := http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is empty")
		}
		return fmt.Errorf("invalid request: %v", err)
	}
	return nil
}

func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}

func queryInt(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return id
}

// queryMaps runs a query and returns each row as a column->value map, ready for JSON.
func (a *App) queryMaps(query string, args ...any) ([]map[string]any, error) {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *App) queryOne(query string, args ...any) (map[string]any, error) {
	rows, err := a.queryMaps(query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return rows[0], nil
}

func (a *App) queryIDs(query string, args ...any) ([]int64, error) {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid && id.Int64 > 0 {
			ids = append(ids, id.Int64)
		}
	}
	return ids, rows.Err()
}

func (a *App) scalar(query string, args ...any) float64 {
	var v sql.NullFloat64
	if err := a.db.QueryRow(query, args...).Scan(&v); err != nil {
		return 0
	}
	return v.Float64
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func int64Args(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
