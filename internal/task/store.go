// Package task provides SQLite-backed task operations shared by user interfaces.
package task

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("task not found")

type Task struct {
	ID        int64
	Title     string
	Due       string
	Priority  string
	Status    string
	CreatedAt string
}

type Filter struct {
	Status   string
	Priority string
	Sort     string
}

// Changes uses pointers to distinguish an omitted field from an empty due date.
type Changes struct {
	Title    *string
	Due      *string
	Priority *string
	Status   *string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// A file URI prevents characters such as '?' in a path becoming DSN options.
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout = 5000;
		CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL CHECK (length(trim(title)) > 0),
			due TEXT NOT NULL DEFAULT '',
			priority TEXT NOT NULL CHECK (priority IN ('low', 'medium', 'high')),
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'complete')),
			created_at TEXT NOT NULL
		)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func validateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title must not be empty")
	}
	if strings.ContainsFunc(title, unicode.IsControl) {
		return errors.New("title must be a single line without control characters")
	}
	return nil
}

func validateDue(due string) error {
	if due != "" {
		if _, err := time.Parse(time.DateOnly, due); err != nil {
			return errors.New("due date must be a real date in YYYY-MM-DD format")
		}
	}
	return nil
}

func validatePriority(priority string) error {
	switch priority {
	case "low", "medium", "high":
		return nil
	default:
		return errors.New("priority must be low, medium, or high")
	}
}

func validateStatus(status string) error {
	if status != "pending" && status != "complete" {
		return errors.New("status must be pending or complete")
	}
	return nil
}

func (s *Store) Add(title, due, priority string) (int64, error) {
	title = strings.TrimSpace(title)
	if err := validateTitle(title); err != nil {
		return 0, err
	}
	if err := validateDue(due); err != nil {
		return 0, err
	}
	if err := validatePriority(priority); err != nil {
		return 0, err
	}
	result, err := s.db.Exec(`INSERT INTO tasks (title, due, priority, created_at)
		VALUES (?, ?, ?, ?)`, title, due, priority, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) List(filter Filter) ([]Task, error) {
	query := `SELECT id, title, due, priority, status, created_at FROM tasks WHERE 1 = 1`
	var args []any
	if filter.Status != "" && filter.Status != "all" {
		if err := validateStatus(filter.Status); err != nil {
			return nil, err
		}
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Priority != "" {
		if err := validatePriority(filter.Priority); err != nil {
			return nil, err
		}
		query += " AND priority = ?"
		args = append(args, filter.Priority)
	}
	// Only known SQL fragments can be used for ordering; values stay parameterized.
	switch filter.Sort {
	case "", "id":
		query += " ORDER BY id"
	case "due":
		query += " ORDER BY due = '', due, id"
	case "priority":
		query += " ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, id"
	default:
		return nil, errors.New("sort must be id, due, or priority")
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Due, &t.Priority, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) Update(id int64, changes Changes) error {
	var fields []string
	var args []any
	if changes.Title != nil {
		title := strings.TrimSpace(*changes.Title)
		if err := validateTitle(title); err != nil {
			return err
		}
		fields = append(fields, "title = ?")
		args = append(args, title)
	}
	if changes.Due != nil {
		if err := validateDue(*changes.Due); err != nil {
			return err
		}
		fields = append(fields, "due = ?")
		args = append(args, *changes.Due)
	}
	if changes.Priority != nil {
		if err := validatePriority(*changes.Priority); err != nil {
			return err
		}
		fields = append(fields, "priority = ?")
		args = append(args, *changes.Priority)
	}
	if changes.Status != nil {
		if err := validateStatus(*changes.Status); err != nil {
			return err
		}
		fields = append(fields, "status = ?")
		args = append(args, *changes.Status)
	}
	if len(fields) == 0 {
		return errors.New("provide at least one field to update")
	}
	args = append(args, id)
	result, err := s.db.Exec("UPDATE tasks SET "+strings.Join(fields, ", ")+" WHERE id = ?", args...)
	return changed(result, err)
}

func (s *Store) Delete(id int64) error {
	result, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return changed(result, err)
}

func changed(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
