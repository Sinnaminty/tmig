// Package tui provides the interactive terminal interface for tmig.
package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/Sinnaminty/tmig/internal/task"
)

type ui struct {
	app     *tview.Application
	pages   *tview.Pages
	table   *tview.Table
	header  *tview.TextView
	view    *tview.TextView
	detail  *tview.TextView
	message *tview.TextView
	store   *task.Store
	tasks   []task.Task
	filter  task.Filter
	wide    bool
}

// Run uses the caller's store; the caller remains responsible for closing it.
func Run(store *task.Store) error {
	u, err := newUI(store)
	if err != nil {
		return err
	}
	if err := u.app.Run(); err != nil {
		return fmt.Errorf("start TUI (requires an interactive terminal): %w", err)
	}
	return nil
}

func newUI(store *task.Store) (*ui, error) {
	u := &ui{
		app: tview.NewApplication(), pages: tview.NewPages(),
		table:  tview.NewTable().SetSelectable(true, false).SetFixed(1, 0),
		header: tview.NewTextView(), view: tview.NewTextView(), detail: tview.NewTextView(), message: tview.NewTextView(),
		store: store, filter: task.Filter{Status: "all", Sort: "id"},
	}
	stylePanel(u.table.Box, " TASKS ")
	u.table.SetBorderPadding(1, 0, 1, 1)
	u.table.SetSelectedStyle(tcell.StyleDefault.Background(color(selection)).Foreground(color(foreground)))
	u.table.SetSelectionChangedFunc(func(row, column int) { u.showDetail() })
	u.table.SetInputCapture(u.handleKey)
	u.header.SetDynamicColors(true).SetBackgroundColor(color(background))
	u.view.SetDynamicColors(true).SetBackgroundColor(color(background))
	u.detail.SetWrap(true).SetDynamicColors(true).SetTextColor(color(foreground))
	stylePanel(u.detail.Box, " SELECTED TASK ")
	u.detail.SetBackgroundColor(color(panel))
	u.detail.SetBorderPadding(0, 0, 1, 1)
	u.message.SetBackgroundColor(color(background))
	footer := tview.NewTextView().SetDynamicColors(true).SetText(
		"[" + accent + "]↑↓[-] Move   [" + accent + "]a[-] Add   [" + accent + "]Enter[-] Edit   [" + accent + "]Space[-] Complete / reopen\n" +
			"[" + accent + "]f[-] Filter / sort   [" + accent + "]d[-] Delete   [" + accent + "]r[-] Refresh   [" + accent + "]q[-] Quit")
	footer.SetTextColor(color(muted)).SetBackgroundColor(color(background))
	body := tview.NewFlex()
	body.SetBackgroundColor(color(background))
	gap := tview.NewBox().SetBackgroundColor(color(background))
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(u.header, 4, 0, false).
		AddItem(u.view, 2, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(u.message, 1, 0, false).
		AddItem(footer, 2, 0, false)
	root.SetBackgroundColor(color(background))
	u.pages.AddPage("main", root, true, true)
	u.pages.SetBackgroundColor(color(background))
	u.app.SetRoot(u.pages, true).SetFocus(u.table).EnablePaste(true)
	// Keep forms usable and prevent hidden edits in terminals too small to draw them.
	tooSmall := false
	layoutWidth := 0
	u.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		width, height := screen.Size()
		screen.Fill(' ', tcell.StyleDefault.Background(color(background)))
		tooSmall = width < 76 || height < 24
		if tooSmall {
			tview.Print(screen, "Resize to at least 76 x 24. Ctrl-C to quit.", 0, 0, width, tview.AlignLeft, color(foreground))
			return true
		}
		// A bounded, centered workspace avoids stretching a short list across an
		// entire monitor. Narrow terminals stack the details below the list.
		xpad, ypad := max(0, (width-152)/2)+1, max(0, (height-38)/2)+1
		root.SetBorderPadding(ypad, ypad, xpad, xpad)
		if layoutWidth != width {
			layoutWidth = width
			u.wide = width >= 116
			body.Clear()
			if u.wide {
				body.SetDirection(tview.FlexColumn).AddItem(u.table, 0, 1, true).
					AddItem(gap, 1, 0, false).AddItem(u.detail, 32, 0, false)
			} else {
				body.SetDirection(tview.FlexRow).AddItem(u.table, 0, 1, true).
					AddItem(gap, 1, 0, false).AddItem(u.detail, 5, 0, false)
			}
			u.showDetail()
		}
		tableWidth := width - 2*xpad
		if u.wide {
			tableWidth -= 33
		}
		for row := 0; row < u.table.GetRowCount(); row++ {
			u.table.GetCell(row, 1).SetMaxWidth(max(10, tableWidth-44))
		}
		return false
	})
	u.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			u.app.Stop()
			return nil
		}
		if tooSmall {
			return nil
		}
		return event
	})
	if err := u.refresh(0); err != nil {
		return nil, err
	}
	return u, nil
}

func (u *ui) selected() (task.Task, bool) {
	row, _ := u.table.GetSelection()
	if row < 1 || row > len(u.tasks) {
		return task.Task{}, false
	}
	return u.tasks[row-1], true
}

func (u *ui) showDetail() {
	if selected, ok := u.selected(); ok {
		status, statusColor := "Pending", amber
		if selected.Status == "complete" {
			status, statusColor = "Complete", green
		}
		due := selected.Due
		if due == "" {
			due = "No deadline"
		}
		title := tview.Escape(selected.Title)
		if u.wide {
			u.detail.SetText(fmt.Sprintf(
				"\n[%s]TASK #%d[-]\n\n[%s::b]%s[-::-]\n\n[%s]STATUS[-]\n[%s]%s[-]\n\n[%s]PRIORITY[-]\n[#%06x]%s[-]\n\n[%s]DUE DATE[-]\n%s\n\n[%s]Enter to edit\nSpace to complete / reopen[-]",
				muted, selected.ID, foreground, title, muted, statusColor, status,
				muted, priorityColor(selected.Priority).Hex(), strings.ToUpper(selected.Priority), muted, due, muted))
		} else {
			u.detail.SetText(fmt.Sprintf("[%s::b]#%d  %s[-::-]\n[%s]%s[-]  ·  %s priority  ·  %s", foreground, selected.ID, title, statusColor, status, selected.Priority, due))
		}
	} else {
		u.detail.SetText("[" + foreground + "]No matching tasks[-]\n[" + muted + "]Press a to add a task or f to change filters.[-]")
	}
}

func (u *ui) refresh(preferredID int64) error {
	tasks, err := u.store.List(u.filter)
	if err != nil {
		return err
	}
	row, _ := u.table.GetSelection()
	u.tasks = tasks
	u.table.Clear()
	for column, title := range []string{"#", "TASK", "STATUS", "PRIORITY", "DUE"} {
		u.table.SetCell(0, column, tview.NewTableCell(" "+title+" ").SetSelectable(false).SetTextColor(color(muted)).SetAttributes(tcell.AttrBold))
	}
	pending, complete := 0, 0
	for index, item := range tasks {
		due := item.Due
		if due == "" {
			due = "-"
		}
		status, statusColor := "○ Pending", color(amber)
		if item.Status == "complete" {
			complete++
			status, statusColor = "✓ Done", color(green)
		} else {
			pending++
		}
		values := []string{strconv.FormatInt(item.ID, 10), item.Title, status, strings.ToUpper(item.Priority), due}
		colors := []tcell.Color{color(muted), color(foreground), statusColor, priorityColor(item.Priority), color(muted)}
		for column, value := range values {
			cell := tview.NewTableCell(" " + tview.Escape(value) + " ").SetTextColor(colors[column]).
				SetSelectedStyle(tcell.StyleDefault.Background(color(selection)).Foreground(colors[column]))
			if column == 1 {
				cell.SetExpansion(1).SetAttributes(tcell.AttrBold)
			}
			u.table.SetCell(index+1, column, cell)
		}
		if item.ID == preferredID {
			row = index + 1
		}
	}
	if len(tasks) == 0 {
		u.table.Select(0, 0)
	} else {
		u.table.Select(max(1, min(row, len(tasks))), 0)
	}
	priority := u.filter.Priority
	if priority == "" {
		priority = "all"
	}
	u.header.SetText(fmt.Sprintf("[%s:%s:b] TMIG [-:-:-]  [%s::b]Task Manager In Go[-::-]\n\n[%s]%d TASKS IN VIEW[-]    [%s]○ %d pending[-]    [%s]✓ %d complete[-]", background, accent, foreground, muted, len(tasks), amber, pending, green, complete))
	u.view.SetText(fmt.Sprintf("[%s]VIEW[-]  [%s]%s[-]   [%s]PRIORITY[-]  [%s]%s[-]   [%s]SORT[-]  [%s]%s[-]", muted, foreground, u.filter.Status, muted, foreground, priority, muted, foreground, u.filter.Sort))
	u.showDetail()
	return nil
}

func (u *ui) report(err error, success string, selectedID int64) {
	if err == nil {
		err = u.refresh(selectedID)
	}
	if err != nil {
		u.message.SetTextColor(color(red)).SetText(" " + err.Error())
	} else {
		u.message.SetTextColor(color(green)).SetText(" " + success)
	}
}

func (u *ui) handleKey(event *tcell.EventKey) *tcell.EventKey {
	selected, ok := u.selected()
	if event.Key() == tcell.KeyEnter {
		if ok {
			u.edit(&selected)
		}
		return nil
	}
	if event.Key() != tcell.KeyRune {
		return event
	}
	switch event.Rune() {
	case 'q':
		u.app.Stop()
	case 'a':
		u.edit(nil)
	case 'e':
		if ok {
			u.edit(&selected)
		}
	case ' ':
		if ok {
			status := "complete"
			if selected.Status == "complete" {
				status = "pending"
			}
			u.report(u.store.Update(selected.ID, task.Changes{Status: &status}), fmt.Sprintf("Task %d marked %s.", selected.ID, status), selected.ID)
		}
	case 'd':
		if ok {
			u.confirmDelete(selected)
		}
	case 'f':
		u.filters()
	case 'r':
		u.report(nil, "Refreshed.", selected.ID)
	default:
		return event
	}
	return nil
}

func (u *ui) closeDialog() {
	u.pages.RemovePage("dialog")
	u.app.SetFocus(u.table)
}

func (u *ui) showForm(title string, form *tview.Form, message *tview.TextView) {
	stylePanel(form.Box, title)
	styleForm(form)
	message.SetBackgroundColor(color(panel))
	message.SetTextColor(color(muted))
	form.SetCancelFunc(u.closeDialog)
	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).AddItem(message, 3, 0, false)
	centered := tview.NewFlex().AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).AddItem(content, 14, 0, true).
			AddItem(nil, 0, 1, false), 72, 0, true).
		AddItem(nil, 0, 1, false)
	u.pages.AddPage("dialog", centered, true, true)
	u.app.SetFocus(form)
}

func (u *ui) edit(existing *task.Task) {
	item := task.Task{Priority: "medium", Status: "pending"}
	caption := " Add task "
	if existing != nil {
		item = *existing
		caption = fmt.Sprintf(" Edit task %d ", item.ID)
	}
	priorities := []string{"low", "medium", "high"}
	form := tview.NewForm().
		AddInputField("Title", item.Title, 48, nil, nil).
		AddInputField("Due (YYYY-MM-DD)", item.Due, 12, nil, nil).
		AddDropDown("Priority", priorities, slices.Index(priorities, item.Priority), nil)
	message := tview.NewTextView().SetText(" Blank due date means no deadline. Tab: next field. Esc: cancel.")
	form.AddButton("Save", func() {
		title := form.GetFormItem(0).(*tview.InputField).GetText()
		due := form.GetFormItem(1).(*tview.InputField).GetText()
		_, priority := form.GetFormItem(2).(*tview.DropDown).GetCurrentOption()
		id := item.ID
		var err error
		if existing == nil {
			id, err = u.store.Add(title, due, priority)
		} else {
			err = u.store.Update(id, task.Changes{Title: &title, Due: &due, Priority: &priority})
		}
		if err != nil {
			message.SetTextColor(color(red)).SetText(" " + err.Error())
			return
		}
		u.closeDialog()
		u.report(nil, fmt.Sprintf("Saved task %d.", id), id)
		if !slices.ContainsFunc(u.tasks, func(item task.Task) bool { return item.ID == id }) {
			u.message.SetTextColor(color(amber)).SetText(fmt.Sprintf(" Saved task %d. Hidden by the current filters.", id))
		}
	}).AddButton("Cancel", u.closeDialog)
	u.showForm(caption, form, message)
}

func (u *ui) confirmDelete(item task.Task) {
	modal := tview.NewModal().
		SetBackgroundColor(color(panel)).SetTextColor(color(foreground)).
		SetButtonBackgroundColor(color(field)).SetButtonTextColor(color(foreground)).
		SetButtonActivatedStyle(tcell.StyleDefault.Background(color(red)).Foreground(color(background)).Bold(true)).
		SetText(fmt.Sprintf("Delete task %d?\n%s\n\nThis cannot be undone.", item.ID, tview.Escape(item.Title))).
		AddButtons([]string{"Cancel", "Delete"}).
		SetDoneFunc(func(index int, label string) {
			u.closeDialog()
			if label == "Delete" {
				u.report(u.store.Delete(item.ID), fmt.Sprintf("Deleted task %d.", item.ID), 0)
			}
		})
	u.pages.AddPage("dialog", modal, true, true)
	u.app.SetFocus(modal)
}

func (u *ui) filters() {
	statuses := []string{"all", "pending", "complete"}
	priorities := []string{"all", "low", "medium", "high"}
	sorts := []string{"id", "due", "priority"}
	priority := u.filter.Priority
	if priority == "" {
		priority = "all"
	}
	form := tview.NewForm().
		AddDropDown("Status", statuses, slices.Index(statuses, u.filter.Status), nil).
		AddDropDown("Priority", priorities, slices.Index(priorities, priority), nil).
		AddDropDown("Sort", sorts, slices.Index(sorts, u.filter.Sort), nil)
	message := tview.NewTextView().SetText(" Due: earliest first, undated last. Priority: high first.\n Filters apply to this TUI session only. Esc: cancel.")
	form.AddButton("Apply", func() {
		_, status := form.GetFormItem(0).(*tview.DropDown).GetCurrentOption()
		_, priority := form.GetFormItem(1).(*tview.DropDown).GetCurrentOption()
		_, sort := form.GetFormItem(2).(*tview.DropDown).GetCurrentOption()
		if priority == "all" {
			priority = ""
		}
		selected, _ := u.selected()
		previous := u.filter
		u.filter = task.Filter{Status: status, Priority: priority, Sort: sort}
		if err := u.refresh(selected.ID); err != nil {
			u.filter = previous
			message.SetTextColor(color(red)).SetText(" " + err.Error())
			return
		}
		u.closeDialog()
		u.message.SetText("")
	}).AddButton("Cancel", u.closeDialog)
	u.showForm(" Filters and sorting ", form, message)
}
