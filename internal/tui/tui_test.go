package tui

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/Sinnaminty/tmig/internal/task"
)

type terminal struct {
	u      *ui
	screen tcell.SimulationScreen
	t      *testing.T
}

func newTerminal(t *testing.T) *terminal {
	t.Helper()
	s, err := task.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, err := newUI(s)
	if err != nil {
		t.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	u.app.SetScreen(screen)
	t.Cleanup(screen.Fini)
	screen.SetSize(100, 30)
	u.app.ForceDraw()
	return &terminal{u: u, screen: screen, t: t}
}

// Dispatch through the same capture and root handlers as tview's event loop.
// Keeping dispatch synchronous makes assertions deterministic without sleeps.
func (term *terminal) key(key tcell.Key, r rune) {
	event := term.u.app.GetInputCapture()(tcell.NewEventKey(key, r, tcell.ModNone))
	if event != nil {
		term.u.pages.InputHandler()(event, func(p tview.Primitive) { term.u.app.SetFocus(p) })
	}
	term.u.app.ForceDraw()
}

func (term *terminal) text(value string) {
	for _, r := range value {
		term.key(tcell.KeyRune, r)
	}
}

func (term *terminal) tabs(count int) {
	for range count {
		term.key(tcell.KeyTab, 0)
	}
}

func (term *terminal) tasks() []task.Task {
	term.t.Helper()
	items, err := term.u.store.List(task.Filter{})
	if err != nil {
		term.t.Fatal(err)
	}
	return items
}

func (term *terminal) rendered() string {
	cells, width, height := term.screen.GetContents()
	var text strings.Builder
	for row := range height {
		for column := range width {
			text.WriteString(string(cells[row*width+column].Runes))
		}
		text.WriteRune('\n')
	}
	return text.String()
}

func (term *terminal) add(title, due string) {
	term.text("a")
	term.text(title)
	term.tabs(1)
	term.text(due)
	term.tabs(2)
	term.key(tcell.KeyEnter, 0)
	if term.u.pages.HasPage("dialog") {
		term.t.Fatalf("add did not close the form:\n%s", term.rendered())
	}
}

func TestKeyboardTaskLifecycle(t *testing.T) {
	t.Parallel()
	term := newTerminal(t)
	if !strings.Contains(term.rendered(), "No matching tasks") {
		t.Fatal("missing empty-state instructions")
	}
	// Shortcut letters and tview markup must remain literal text in an input.
	term.add("a qe [red] task", "2026-09-21")
	items := term.tasks()
	if len(items) != 1 || items[0].Title != "a qe [red] task" || items[0].Due != "2026-09-21" || items[0].Priority != "medium" {
		t.Fatalf("unexpected saved task: %+v", items)
	}
	if !strings.Contains(term.rendered(), "a qe [red] task") {
		t.Fatal("title markup was not rendered literally")
	}
	term.text(" ")
	if got := term.tasks()[0].Status; got != "complete" {
		t.Fatalf("Space set status to %q", got)
	}
	term.text(" ")
	if got := term.tasks()[0].Status; got != "pending" {
		t.Fatalf("second Space set status to %q", got)
	}
	term.key(tcell.KeyEnter, 0)
	term.key(tcell.KeyEnd, 0)
	term.text(" edited")
	term.tabs(1)
	term.key(tcell.KeyEnd, 0)
	for range len("2026-09-21") {
		term.key(tcell.KeyBackspace2, 0)
	}
	term.tabs(1)
	term.key(tcell.KeyEnter, 0)
	term.key(tcell.KeyDown, 0) // medium -> high
	term.key(tcell.KeyEnter, 0)
	term.tabs(1)
	term.key(tcell.KeyEnter, 0)
	got := term.tasks()[0]
	if got.Title != "a qe [red] task edited" || got.Due != "" || got.Priority != "high" || got.Status != "pending" {
		t.Fatalf("edit produced %+v", got)
	}
	term.text("d")
	term.key(tcell.KeyEnter, 0) // Cancel is the default button.
	if len(term.tasks()) != 1 {
		t.Fatal("default delete action did not cancel")
	}
	term.text("d")
	term.key(tcell.KeyEscape, 0)
	if len(term.tasks()) != 1 || term.u.pages.HasPage("dialog") {
		t.Fatal("Escape did not cancel deletion")
	}
	term.text("d")
	term.tabs(1)
	term.key(tcell.KeyEnter, 0)
	if len(term.tasks()) != 0 || !strings.Contains(term.rendered(), "No matching tasks") {
		t.Fatal("confirmed deletion did not refresh the empty list")
	}
	term.text(" ed") // Actions on an empty selection must do nothing.
	if term.u.pages.HasPage("dialog") {
		t.Fatal("empty selection opened an edit/delete dialog")
	}
}

func TestFormValidationAndCancellation(t *testing.T) {
	t.Parallel()
	term := newTerminal(t)
	term.text("a")
	term.text("Keep this title")
	term.tabs(1)
	term.text("2026-02-30")
	term.tabs(2)
	term.key(tcell.KeyEnter, 0)
	if len(term.tasks()) != 0 || !term.u.pages.HasPage("dialog") || !strings.Contains(term.rendered(), "due date") {
		t.Fatalf("invalid form was not retained with an error:\n%s", term.rendered())
	}
	term.key(tcell.KeyBacktab, 0)
	term.key(tcell.KeyBacktab, 0) // Back to due date.
	term.key(tcell.KeyEnd, 0)
	for range len("2026-02-30") {
		term.key(tcell.KeyBackspace2, 0)
	}
	term.text("2026-02-28")
	term.tabs(2)
	term.key(tcell.KeyEnter, 0)
	before := term.tasks()
	if len(before) != 1 || before[0].Title != "Keep this title" || before[0].Due != "2026-02-28" {
		t.Fatalf("corrected form did not preserve input: %+v", before)
	}
	term.text("e")
	term.text("Discard this")
	term.key(tcell.KeyEscape, 0)
	if got := term.tasks(); !reflect.DeepEqual(got, before) || term.u.pages.HasPage("dialog") {
		t.Fatal("Escape saved an edit or left the form open")
	}
	term.text("a")
	term.text("Discard new task")
	term.tabs(4) // Cancel button.
	term.key(tcell.KeyEnter, 0)
	if got := term.tasks(); !reflect.DeepEqual(got, before) {
		t.Fatal("Cancel saved a new task")
	}
}

func TestFiltersSelectionAndRefresh(t *testing.T) {
	t.Parallel()
	term := newTerminal(t)
	term.add("Later", "2026-09-22")
	term.add("Earlier", "2026-09-20")
	term.text("f")
	term.key(tcell.KeyEnter, 0)
	term.key(tcell.KeyDown, 0) // all -> pending
	term.key(tcell.KeyEnter, 0)
	term.tabs(2)
	term.key(tcell.KeyEnter, 0)
	term.key(tcell.KeyDown, 0) // id -> due
	term.key(tcell.KeyEnter, 0)
	term.tabs(1)
	term.key(tcell.KeyEnter, 0)
	if term.u.filter.Status != "pending" || term.u.filter.Sort != "due" || term.u.tasks[0].Title != "Earlier" {
		t.Fatalf("filters not applied: %+v, tasks %+v", term.u.filter, term.u.tasks)
	}
	if selected, _ := term.u.selected(); selected.Title != "Earlier" {
		t.Fatalf("sorting lost the selected task: %+v", selected)
	}
	term.text(" ")
	if len(term.u.tasks) != 1 || term.u.tasks[0].Title != "Later" {
		t.Fatal("completed task remained in pending filter")
	}
	if _, err := term.u.store.Add("External task", "", "high"); err != nil {
		t.Fatal(err)
	}
	term.text("r")
	if len(term.u.tasks) != 2 {
		t.Fatal("refresh did not load external changes")
	}
	term.key(tcell.KeyDown, 0)
	if selected, _ := term.u.selected(); selected.Title != "External task" {
		t.Fatalf("Down did not select next task: %+v", selected)
	}
	term.text("f")
	term.key(tcell.KeyEscape, 0)
	if term.u.filter.Status != "pending" || term.u.filter.Sort != "due" {
		t.Fatal("canceling filters changed them")
	}
}

func TestSmallTerminalBlocksHiddenEdits(t *testing.T) {
	t.Parallel()
	term := newTerminal(t)
	term.screen.SetSize(40, 10)
	term.u.app.ForceDraw()
	term.text("a")
	if term.u.pages.HasPage("dialog") || !strings.Contains(term.rendered(), "Resize to at least") {
		t.Fatal("small terminal did not block hidden actions")
	}
	term.screen.SetSize(76, 24)
	term.u.app.ForceDraw()
	term.add("Resized", "")
	if len(term.tasks()) != 1 {
		t.Fatal("resizing did not restore interaction")
	}
}
