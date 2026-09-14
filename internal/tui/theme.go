package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Explicit colors keep the interface consistent across terminal palettes.
const (
	background = "#10151f"
	panel      = "#171f2d"
	field      = "#233047"
	selection  = "#2b405b"
	border     = "#35465e"
	foreground = "#e6edf7"
	muted      = "#9aaac0"
	accent     = "#82cfff"
	green      = "#a6e3c0"
	amber      = "#f4cc8a"
	red        = "#f3a6b5"
)

func color(value string) tcell.Color { return tcell.GetColor(value) }

func stylePanel(box *tview.Box, title string) {
	box.SetBackgroundColor(color(panel)).SetBorder(true).
		SetBorderColor(color(border)).SetTitleColor(color(muted)).
		SetTitleAlign(tview.AlignLeft).SetTitle(title)
}

func styleForm(form *tview.Form) {
	form.SetBackgroundColor(color(panel))
	form.SetLabelColor(color(muted)).
		SetFieldBackgroundColor(color(field)).SetFieldTextColor(color(foreground)).
		SetButtonBackgroundColor(color(field)).SetButtonTextColor(color(foreground)).
		SetButtonActivatedStyle(tcell.StyleDefault.Background(color(accent)).Foreground(color(background)).Bold(true))
	for index := 0; index < form.GetFormItemCount(); index++ {
		switch item := form.GetFormItem(index).(type) {
		case *tview.InputField:
			item.SetBackgroundColor(color(panel))
		case *tview.TextArea:
			item.SetBackgroundColor(color(panel))
		case *tview.DropDown:
			item.SetBackgroundColor(color(panel))
		}
	}
}

func priorityColor(priority string) tcell.Color {
	switch priority {
	case "high":
		return color(red)
	case "medium":
		return color(amber)
	default:
		return color(muted)
	}
}
