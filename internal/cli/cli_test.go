package cli_test

import (
	"bytes"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Sinnaminty/tmig/internal/cli"
	"github.com/Sinnaminty/tmig/internal/task"
)

func run(db string, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	err := cli.Run(append([]string{"--db", db}, args...), &out, &errOut)
	return out.String(), errOut.String(), err
}

func mustRun(t *testing.T, db string, args ...string) string {
	t.Helper()
	out, errOut, err := run(db, args...)
	if err != nil {
		t.Fatalf("tmig %q: %v\nstderr: %s", args, err, errOut)
	}
	if errOut != "" {
		t.Fatalf("tmig %q unexpectedly wrote stderr: %s", args, errOut)
	}
	return out
}

func snapshot(t *testing.T, db string) []task.Task {
	t.Helper()
	s, err := task.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tasks, err := s.List(task.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return tasks
}

func TestTaskLifecycle(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "tasks.db")
	if got := mustRun(t, db, "list"); got != "No tasks found.\n" {
		t.Fatalf("empty list = %q", got)
	}
	if got := mustRun(t, db, "add", "First task"); got != "Added task 1.\n" {
		t.Fatalf("add output = %q", got)
	}
	first := snapshot(t, db)[0]
	if first.Title != "First task" || first.Status != "pending" || first.Priority != "medium" || first.Due != "" {
		t.Fatalf("unexpected defaults: %+v", first)
	}
	mustRun(t, db, "update", "1", "--title", "Renamed", "--due", "2026-09-21", "--priority", "high")
	first.Title, first.Due, first.Priority = "Renamed", "2026-09-21", "high"
	if got := snapshot(t, db)[0]; got != first {
		t.Fatalf("updated task = %+v; want %+v", got, first)
	}
	mustRun(t, db, "complete", "1")
	mustRun(t, db, "complete", "1")
	if got := mustRun(t, db, "list", "--status", "pending"); got != "No tasks found.\n" {
		t.Fatalf("completed task still pending: %s", got)
	}
	listing := mustRun(t, db, "list", "--status", "complete", "--priority", "high")
	if !strings.Contains(listing, "TITLE") || !strings.Contains(listing, "Renamed") || !strings.Contains(listing, "2026-09-21") {
		t.Fatalf("missing task data in list: %s", listing)
	}
	mustRun(t, db, "update", "--due", "", "--status", "pending", "1")
	first.Due = ""
	if got := snapshot(t, db)[0]; got != first {
		t.Fatalf("reopened task = %+v; want %+v", got, first)
	}
	mustRun(t, db, "delete", "1")
	if got := mustRun(t, db, "list"); got != "No tasks found.\n" {
		t.Fatalf("deleted task remains: %s", got)
	}
	if _, _, err := run(db, "delete", "1"); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("deleting missing task = %v; want ErrNotFound", err)
	}
}

func TestAddArgumentForms(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, title, due, priority string
		args                       []string
	}{
		{"title first", "Task one", "2026-09-21", "high", []string{"add", "Task one", "--due", "2026-09-21", "--priority", "high"}},
		{"title last", "Task two", "2026-09-21", "low", []string{"add", "--due", "2026-09-21", "--priority", "low", "Task two"}},
		{"equals syntax", "Task three", "2026-09-21", "high", []string{"add", "--due=2026-09-21", "--priority=high", "Task three"}},
		{"dash title", "- my task", "", "medium", []string{"add", "--", "- my task"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := filepath.Join(t.TempDir(), "tasks.db")
			mustRun(t, db, tt.args...)
			got := snapshot(t, db)
			if len(got) != 1 || got[0].Title != tt.title || got[0].Due != tt.due || got[0].Priority != tt.priority {
				t.Fatalf("unexpected parsed task: %+v", got)
			}
		})
	}
}

func TestInvalidCommandsPreserveTasks(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "tasks.db")
	mustRun(t, db, "add", "Keep me", "--priority", "high")
	before := snapshot(t, db)
	for _, tt := range []struct {
		name, message string
		args          []string
	}{
		{"unknown command", "unknown command", []string{"unknown"}},
		{"extra TUI argument", "positional", []string{"tui", "extra"}},
		{"unknown global flag", "flag", []string{"--unknown"}},
		{"unknown command flag", "flag", []string{"add", "Task", "--unknown"}},
		{"missing flag value", "needs an argument", []string{"list", "--sort"}},
		{"missing title", "positional", []string{"add"}},
		{"extra title", "positional", []string{"add", "one", "two"}},
		{"extra list argument", "positional", []string{"list", "extra"}},
		{"missing ID", "positional", []string{"complete"}},
		{"nonnumeric ID", "positive integer", []string{"delete", "abc"}},
		{"zero ID", "positive integer", []string{"delete", "0"}},
		{"negative ID", "positive integer", []string{"delete", "--", "-1"}},
		{"overflow ID", "positive integer", []string{"delete", "9223372036854775808"}},
		{"missing task", "task not found", []string{"complete", "9999"}},
		{"no changes", "at least one", []string{"update", "1"}},
		{"empty title", "title", []string{"add", ""}},
		{"invalid date", "due date", []string{"add", "Task", "--due", "2026-02-30"}},
		{"invalid priority", "priority", []string{"add", "Task", "--priority", "urgent"}},
		{"invalid status", "status", []string{"list", "--status", "unknown"}},
		{"invalid filter priority", "priority", []string{"list", "--priority", "urgent"}},
		{"invalid sort", "sort", []string{"list", "--sort", "unknown"}},
		{"partial update failure", "due date", []string{"update", "1", "--title", "Do not save", "--due", "2026-02-30"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := run(db, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("error = %v; want %q", err, tt.message)
			}
			if out != "" {
				t.Errorf("failed command reported output: %q", out)
			}
			if got := snapshot(t, db); !reflect.DeepEqual(got, before) {
				t.Fatalf("failed command changed tasks: %+v; want %+v", got, before)
			}
		})
	}
}

func TestHelpDoesNotOpenDatabase(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "missing", "tasks.db")
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"add", "--help"}, {"list", "--help"}, {"update", "--help"}, {"complete", "--help"}, {"delete", "--help"}, {"export", "--help"}, {"tui", "--help"}} {
		out, errOut, err := run(db, args...)
		if err != nil || !strings.Contains(out+errOut, "Usage:") {
			t.Errorf("help %q: output %q, error %v", args, out+errOut, err)
		}
	}
	if _, err := os.Stat(db); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("help touched database: %v", err)
	}
}

func TestDatabaseSelection(t *testing.T) {
	// Environment changes must stay out of parallel tests.
	dir := t.TempDir()
	envDB, flagDB := filepath.Join(dir, "env.db"), filepath.Join(dir, "flag.db")
	t.Setenv("TMIG_DB", envDB)
	var out, errOut bytes.Buffer
	if err := cli.Run([]string{"add", "Environment task"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	mustRun(t, flagDB, "add", "Flag task")
	for path, title := range map[string]string{envDB: "Environment task", flagDB: "Flag task"} {
		if got := snapshot(t, path); len(got) != 1 || got[0].Title != title {
			t.Errorf("database %s contains %+v; want only %q", path, got, title)
		}
	}
}

func TestDefaultDatabaseLocation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_CONFIG_HOME controls the default location on Linux")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("TMIG_DB", "")
	var out, errOut bytes.Buffer
	if err := cli.Run([]string{"add", "Default location"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "tmig", "tasks.db")
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("default database not created: %v", err)
	}
	if got := snapshot(t, db); len(got) != 1 || got[0].Title != "Default location" {
		t.Fatalf("default database contains %+v", got)
	}
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestCSVExport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "tasks.db")
	mustRun(t, db, "add", `Review "CSV", carefully`)
	mustRun(t, db, "add", "Low task", "--priority", "low")
	mustRun(t, db, "add", "Café review", "--priority", "high", "--due", "2026-09-21")
	mustRun(t, db, "complete", "2")
	tasks := snapshot(t, db)
	header := []string{"id", "title", "due", "priority", "status", "created_at"}
	medium := []string{"1", `Review "CSV", carefully`, "", "medium", "pending", tasks[0].CreatedAt}
	low := []string{"2", "Low task", "", "low", "complete", tasks[1].CreatedAt}
	high := []string{"3", "Café review", "2026-09-21", "high", "pending", tasks[2].CreatedAt}
	for _, tt := range []struct {
		name string
		args []string
		want [][]string
	}{
		{"all", nil, [][]string{header, medium, low, high}},
		{"priority", []string{"--sort", "priority"}, [][]string{header, high, medium, low}},
		{"pending by due", []string{"--status", "pending", "--sort", "due"}, [][]string{header, high, medium}},
		{"combined filters", []string{"--status", "pending", "--priority", "high"}, [][]string{header, high}},
		{"empty", []string{"--status", "complete", "--priority", "high"}, [][]string{header}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "export.csv")
			out := mustRun(t, db, append([]string{"export", "--output", path}, tt.args...)...)
			if !strings.Contains(out, path) {
				t.Errorf("output does not confirm path: %q", out)
			}
			if got := readCSV(t, path); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CSV = %q; want %q", got, tt.want)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := run(db, "export", "--output", path); !errors.Is(err, os.ErrExist) {
				t.Fatalf("repeat export error = %v; want file exists", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("repeat export modified original (read error: %v)", err)
			}
		})
	}
	if _, _, err := run(db, "export", "--output", db); !errors.Is(err, os.ErrExist) {
		t.Fatalf("export over database error = %v; want file exists", err)
	}
	if got := snapshot(t, db); !reflect.DeepEqual(got, tasks) {
		t.Fatal("export changed the database")
	}
	if _, _, err := run(db, "export", "--output", filepath.Join(dir, "missing", "tasks.csv")); err == nil {
		t.Fatal("export to missing directory succeeded")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestListReportsOutputFailure(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "tasks.db")
	want := errors.New("output unavailable")
	for _, populated := range []bool{false, true} {
		if populated {
			mustRun(t, db, "add", "Task")
		}
		var errOut bytes.Buffer
		if err := cli.Run([]string{"--db", db, "list"}, failingWriter{want}, &errOut); !errors.Is(err, want) {
			t.Errorf("list (populated=%t) error = %v; want output error", populated, err)
		}
	}
}
