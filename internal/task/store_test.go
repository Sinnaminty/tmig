package task_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Sinnaminty/tmig/internal/task"
)

func openStore(t *testing.T, path string) *task.Store {
	t.Helper()
	s, err := task.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func newStore(t *testing.T) *task.Store {
	t.Helper()
	return openStore(t, filepath.Join(t.TempDir(), "tasks.db"))
}

func addTask(t *testing.T, s *task.Store, title, due, priority string) int64 {
	t.Helper()
	id, err := s.Add(title, due, priority)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func listTasks(t *testing.T, s *task.Store) []task.Task {
	t.Helper()
	tasks, err := s.List(task.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return tasks
}

func ptr(s string) *string { return &s }

func TestAddAndPersistence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tasks.db")
	s := openStore(t, path)
	if tasks := listTasks(t, s); len(tasks) != 0 {
		t.Fatalf("new database has tasks: %+v", tasks)
	}
	before := time.Now().UTC().Truncate(time.Second)
	title := `Review "CSV", café'); DROP TABLE tasks; --`
	id := addTask(t, s, "  "+title+"  ", "2024-02-29", "high")
	if id <= 0 {
		t.Fatalf("ID = %d; want positive", id)
	}
	tasks := listTasks(t, s)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks; want 1", len(tasks))
	}
	got := tasks[0]
	want := task.Task{ID: id, Title: title, Due: "2024-02-29", Priority: "high", Status: "pending", CreatedAt: got.CreatedAt}
	if got != want {
		t.Fatalf("got %+v; want %+v", got, want)
	}
	created, err := time.Parse(time.RFC3339, got.CreatedAt)
	if err != nil || !strings.HasSuffix(got.CreatedAt, "Z") || created.Before(before) || created.After(time.Now().UTC()) {
		t.Fatalf("unexpected creation timestamp %q (parse error: %v)", got.CreatedAt, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openStore(t, path)
	if got := listTasks(t, reopened); !reflect.DeepEqual(got, tasks) {
		t.Fatalf("reopened tasks = %+v; want %+v", got, tasks)
	}
}

func TestAddRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, title, due, priority, message string
	}{
		{"empty title", "", "", "medium", "title"},
		{"whitespace title", " \t\n", "", "medium", "title"},
		{"multiline title", "first\nsecond", "", "medium", "title"},
		{"terminal escape", "bad\x1b[31m", "", "medium", "title"},
		{"impossible date", "task", "2026-02-30", "medium", "due date"},
		{"non leap year", "task", "2025-02-29", "medium", "due date"},
		{"date format", "task", "09/21/2026", "medium", "due date"},
		{"unknown priority", "task", "", "urgent", "priority"},
		{"empty priority", "task", "", "", "priority"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newStore(t)
			id, err := s.Add(tt.title, tt.due, tt.priority)
			if err == nil || !strings.Contains(err.Error(), tt.message) || id != 0 {
				t.Fatalf("Add = (%d, %v); want zero ID and %q error", id, err, tt.message)
			}
			if tasks := listTasks(t, s); len(tasks) != 0 {
				t.Fatalf("invalid add saved tasks: %+v", tasks)
			}
		})
	}
}

func TestListFiltersAndSorting(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := addTask(t, s, "Undated low", "", "low")
	b := addTask(t, s, "Later high", "2026-09-22", "high")
	c := addTask(t, s, "Earlier medium", "2026-09-20", "medium")
	d := addTask(t, s, "Earlier high", "2026-09-20", "high")
	e := addTask(t, s, "Undated medium", "", "medium")
	if err := s.Update(d, task.Changes{Status: ptr("complete")}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		filter task.Filter
		ids    []int64
	}{
		{"default", task.Filter{}, []int64{a, b, c, d, e}},
		{"all by ID", task.Filter{Status: "all", Sort: "id"}, []int64{a, b, c, d, e}},
		{"due with ties and undated", task.Filter{Sort: "due"}, []int64{c, d, b, a, e}},
		{"priority with ties", task.Filter{Sort: "priority"}, []int64{b, d, c, e, a}},
		{"pending", task.Filter{Status: "pending"}, []int64{a, b, c, e}},
		{"complete", task.Filter{Status: "complete"}, []int64{d}},
		{"high", task.Filter{Priority: "high"}, []int64{b, d}},
		{"combined", task.Filter{Status: "pending", Priority: "medium", Sort: "due"}, []int64{c, e}},
		{"no matches", task.Filter{Status: "complete", Priority: "low"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := s.List(tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			var ids []int64
			for _, item := range tasks {
				ids = append(ids, item.ID)
			}
			if !slices.Equal(ids, tt.ids) {
				t.Errorf("IDs = %v; want %v", ids, tt.ids)
			}
		})
	}
	for _, filter := range []task.Filter{
		{Status: "unknown"}, {Priority: "urgent"}, {Sort: "id; DROP TABLE tasks"},
	} {
		if _, err := s.List(filter); err == nil {
			t.Errorf("List(%+v) succeeded; want error", filter)
		}
	}
	if tasks := listTasks(t, s); len(tasks) != 5 {
		t.Fatalf("filter errors changed tasks: %+v", tasks)
	}
}

func TestUpdatePreservesOmittedFields(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := addTask(t, s, "Original", "2026-09-21", "high")
	want := listTasks(t, s)[0]
	for _, tt := range []struct {
		name    string
		changes task.Changes
		apply   func(*task.Task)
	}{
		{"title only", task.Changes{Title: ptr("  Renamed  ")}, func(v *task.Task) { v.Title = "Renamed" }},
		{"clear due", task.Changes{Due: ptr("")}, func(v *task.Task) { v.Due = "" }},
		{"complete", task.Changes{Status: ptr("complete")}, func(v *task.Task) { v.Status = "complete" }},
		{"complete again", task.Changes{Status: ptr("complete")}, func(v *task.Task) {}},
		{"reopen", task.Changes{Status: ptr("pending")}, func(v *task.Task) { v.Status = "pending" }},
		{"multiple fields", task.Changes{Due: ptr("2026-10-01"), Priority: ptr("low")}, func(v *task.Task) {
			v.Due, v.Priority = "2026-10-01", "low"
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.Update(id, tt.changes); err != nil {
				t.Fatal(err)
			}
			tt.apply(&want)
			if got := listTasks(t, s)[0]; got != want {
				t.Fatalf("got %+v; want %+v", got, want)
			}
		})
	}
}

func TestInvalidUpdatesAreAtomic(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := addTask(t, s, "Keep me", "2026-09-21", "high")
	before := listTasks(t, s)
	for _, tt := range []struct {
		name    string
		changes task.Changes
	}{
		{"no fields", task.Changes{}},
		{"empty title", task.Changes{Title: ptr("")}},
		{"invalid due after title", task.Changes{Title: ptr("Do not save"), Due: ptr("2026-02-30")}},
		{"invalid priority after title", task.Changes{Title: ptr("Do not save"), Priority: ptr("urgent")}},
		{"invalid status after valid fields", task.Changes{Title: ptr("Do not save"), Due: ptr("2026-10-01"), Priority: ptr("low"), Status: ptr("unknown")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.Update(id, tt.changes); err == nil {
				t.Fatal("invalid update succeeded")
			}
			if got := listTasks(t, s); !reflect.DeepEqual(got, before) {
				t.Fatalf("failed update changed tasks: %+v; want %+v", got, before)
			}
		})
	}
}

func TestDeleteAndMissingTasks(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	keep := addTask(t, s, "Keep", "", "medium")
	removed := addTask(t, s, "Remove", "", "low")
	if err := s.Delete(removed); err != nil {
		t.Fatal(err)
	}
	if got := listTasks(t, s); len(got) != 1 || got[0].ID != keep {
		t.Fatalf("delete affected wrong tasks: %+v", got)
	}
	for _, id := range []int64{removed, 9999, 0, -1} {
		if err := s.Delete(id); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("Delete(%d) = %v; want ErrNotFound", id, err)
		}
		if err := s.Update(id, task.Changes{Title: ptr("Missing")}); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("Update(%d) = %v; want ErrNotFound", id, err)
		}
	}
	if next := addTask(t, s, "New", "", "medium"); next <= removed {
		t.Errorf("new ID %d reused a deleted ID (last was %d)", next, removed)
	}
}

func TestOpenInvalidPath(t *testing.T) {
	t.Parallel()
	if s, err := task.Open(filepath.Join(t.TempDir(), "missing", "tasks.db")); err == nil {
		s.Close()
		t.Fatal("opening a database in a missing directory succeeded")
	}
}
