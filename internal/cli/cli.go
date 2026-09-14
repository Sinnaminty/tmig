// Package cli parses commands and presents task operations in the terminal.
package cli

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Sinnaminty/tmig/internal/task"
)

const help = `tmig — Task Manager In Go

Usage: tmig [--db PATH] COMMAND [OPTIONS]

Commands:
  add [--due YYYY-MM-DD] [--priority low|medium|high] "TITLE"
  list [--status all|pending|complete] [--priority low|medium|high] [--sort id|due|priority]
  update [--title TEXT] [--due YYYY-MM-DD] [--priority LEVEL] [--status STATUS] ID
  complete ID
  delete ID
  export [--output tasks.csv] [--status STATUS] [--priority LEVEL] [--sort FIELD]

Use --help after a command for its options. Titles and IDs may also come
before all command options. Use update --due "" ID to remove a due date.
Global --db must precede the command; TMIG_DB also overrides the default path.
`

func Run(args []string, out, errOut io.Writer) error {
	global := flag.NewFlagSet("tmig", flag.ContinueOnError)
	global.SetOutput(errOut)
	dbPath := global.String("db", os.Getenv("TMIG_DB"), "SQLite database path")
	global.Usage = func() { fmt.Fprint(errOut, help) }
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	args = global.Args()
	if len(args) == 0 || args[0] == "help" {
		_, err := fmt.Fprint(out, help)
		return err
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	var title, due, priority, status, sort, output string
	switch command {
	case "add":
		fs.StringVar(&due, "due", "", "due date (YYYY-MM-DD)")
		fs.StringVar(&priority, "priority", "medium", "low, medium, or high")
	case "update":
		fs.StringVar(&title, "title", "", "new title")
		fs.StringVar(&due, "due", "", "due date (YYYY-MM-DD); empty clears it")
		fs.StringVar(&priority, "priority", "", "low, medium, or high")
		fs.StringVar(&status, "status", "", "pending or complete")
	case "list", "export":
		fs.StringVar(&status, "status", "all", "all, pending, or complete")
		fs.StringVar(&priority, "priority", "", "filter by low, medium, or high")
		fs.StringVar(&sort, "sort", "id", "id (ascending), due (earliest first), or priority (high first)")
		if command == "export" {
			fs.StringVar(&output, "output", "tasks.csv", "new CSV file path (must not already exist)")
		}
	case "complete", "delete":
	default:
		return fmt.Errorf("unknown command %q; run tmig --help", command)
	}
	fs.Usage = func() {
		fmt.Fprint(errOut, help)
		fmt.Fprintf(errOut, "\nOptions for %s:\n", command)
		fs.PrintDefaults()
	}
	commandArgs := args[1:]
	// flag stops at the first positional argument. Also accept the natural
	// 'add TITLE --due DATE' and 'update ID --title TITLE' command forms.
	if command == "add" || command == "update" || command == "complete" || command == "delete" {
		if len(commandArgs) > 0 && !strings.HasPrefix(commandArgs[0], "-") {
			commandArgs = append(append([]string{}, commandArgs[1:]...), commandArgs[0])
		}
	}
	if err := fs.Parse(commandArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	wantArgs := 0
	if command == "add" || command == "update" || command == "complete" || command == "delete" {
		wantArgs = 1
	}
	if fs.NArg() != wantArgs {
		return fmt.Errorf("%s expects %d positional argument(s); run tmig %s --help", command, wantArgs, command)
	}
	var id int64
	if command == "update" || command == "complete" || command == "delete" {
		var err error
		id, err = strconv.ParseInt(fs.Arg(0), 10, 64)
		if err != nil || id <= 0 {
			return errors.New("task ID must be a positive integer")
		}
	}
	if *dbPath == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("locate storage directory (or set --db): %w", err)
		}
		dir := filepath.Join(config, "tmig")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create storage directory: %w", err)
		}
		*dbPath = filepath.Join(dir, "tasks.db")
	}
	store, err := task.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	switch command {
	case "add":
		id, err := store.Add(fs.Arg(0), due, priority)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Added task %d.\n", id)
		return err
	case "list", "export":
		tasks, err := store.List(task.Filter{Status: status, Priority: priority, Sort: sort})
		if err != nil {
			return err
		}
		if command == "export" {
			return exportCSV(output, tasks, out)
		}
		return printTasks(out, tasks)
	case "update":
		var changes task.Changes
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "title":
				changes.Title = &title
			case "due":
				changes.Due = &due
			case "priority":
				changes.Priority = &priority
			case "status":
				changes.Status = &status
			}
		})
		err = store.Update(id, changes)
	case "complete":
		status = "complete"
		err = store.Update(id, task.Changes{Status: &status})
	case "delete":
		err = store.Delete(id)
	}
	if err != nil {
		return fmt.Errorf("%s task %d: %w", command, id, err)
	}
	messages := map[string]string{"update": "Updated", "complete": "Completed", "delete": "Deleted"}
	_, err = fmt.Fprintf(out, "%s task %d.\n", messages[command], id)
	return err
}

func printTasks(out io.Writer, tasks []task.Task) error {
	if len(tasks) == 0 {
		_, err := fmt.Fprintln(out, "No tasks found.")
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tSTATUS\tPRIORITY\tDUE\tTITLE"); err != nil {
		return err
	}
	for _, t := range tasks {
		due := t.Due
		if due == "" {
			due = "-"
		}
		if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.ID, t.Status, t.Priority, due, t.Title); err != nil {
			return err
		}
	}
	return w.Flush()
}

func exportCSV(path string, tasks []task.Task, out io.Writer) error {
	// Exclusive creation also protects the database if its path is passed here.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create export: %w", err)
	}
	w := csv.NewWriter(f)
	writeErr := w.Write([]string{"id", "title", "due", "priority", "status", "created_at"})
	for _, t := range tasks {
		if writeErr != nil {
			break
		}
		writeErr = w.Write([]string{strconv.FormatInt(t.ID, 10), t.Title, t.Due, t.Priority, t.Status, t.CreatedAt})
	}
	w.Flush()
	flushErr := w.Error()
	closeErr := f.Close()
	if err := errors.Join(writeErr, flushErr, closeErr); err != nil {
		return fmt.Errorf("write export (file may be incomplete): %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Exported %d task(s) to %s\n", len(tasks), abs)
	return err
}
