# tmig — Task Manager In Go

A small Go command-line task manager backed by SQLite. Add tasks, set priorities
and due dates, track completion, filter and sort your list, and export it to CSV.

## Getting started

Requires Go 1.25 or newer. The SQLite driver is written in Go, so no separate
SQLite server or C compiler is needed.

```sh
git clone https://github.com/Sinnaminty/tmig.git
cd tmig
go build -o bin/tmig .
./bin/tmig --help
```

The examples below assume `bin` is on your `PATH`. You can also use
`./bin/tmig` or `go run .` in place of `tmig`.

## Commands

```sh
tmig add "Submit application" --due 2026-09-21 --priority high
tmig add "Read Go documentation"
tmig list
tmig list --status pending --priority high --sort due
tmig update 1 --title "Review and submit application" --due 2026-09-22
tmig update 1 --due ""
tmig complete 1
tmig update 1 --status pending
tmig export --output application-tasks.csv --status pending --sort priority
tmig delete 2
```

Run `tmig COMMAND --help` for the available options. Put a title or ID either
before all command options or after all of them. Quote titles containing spaces.
For a title starting with a dash, use `tmig add -- "- my task"`.

- Tasks start with `pending` status and `medium` priority.
- Priorities are `low`, `medium`, and `high`; statuses are `pending` and `complete`.
- Dates use `YYYY-MM-DD`. Past dates are allowed. A due date is a calendar date,
  with no time or time zone. Creation timestamps use UTC.
- `update` changes only the supplied fields; `--due ""` removes a due date.
- `list` shows all tasks by default. Status and priority filters combine.
- Sorting supports `id` (ascending), `due` (earliest first, undated tasks last),
  and `priority` (high first). Ties are ordered by ID.
- Completing an already complete task is safe. Reopen it with
  `update ID --status pending`.
- Deletion is permanent and takes effect immediately. IDs are not reused.
- Exports include a header and the same filters and sorting as `list`. They
  preserve title text and correctly quote commas and double quotes. Empty results
  produce a header-only file. Existing files are never overwritten.
- CSV is a data interchange format: when opening an export in spreadsheet
  software, import the title column as text to preserve literal values.
- Invalid commands or failed operations return a nonzero exit code.

## Storage

By default, the database is `tmig/tasks.db` inside the operating system's user
configuration directory (for example, `~/.config/tmig/tasks.db` on Linux).
It is shared across working directories and created on first use.

Use a separate database for a demo or manual testing:

```sh
tmig --db ./demo.db add "Demo task"
tmig --db ./demo.db list
```

Global `--db` goes **before** the command. Alternatively, set `TMIG_DB`.
Precedence is `--db`, then `TMIG_DB`, then the default path. The parent directory
of a custom database path must already exist. Database files and compiled binaries
are excluded from Git.

## Approach

This is a local, single-user application. Go's standard `flag` package handles
arguments, `database/sql` and [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)
provide persistence, and `encoding/csv` handles export escaping.

The code is split into three small pieces:

- `main.go`: prints errors and sets the process exit status.
- `internal/cli`: parses commands and formats terminal and CSV output.
- `internal/task`: stores tasks, validates task fields, and performs SQL queries.

SQL values are parameterized, and sort expressions come from a fixed set.
Updates run in a single statement, so invalid input cannot leave a partially
updated task. Database constraints reinforce the allowed status and priority
values. An SQLite busy timeout allows brief competing writes to finish.

The task package has no terminal dependencies, allowing the planned TUI to reuse
the same operations. There is no ORM, web server, or separate database service.

## Development milestones

1. **Required CLI basics (implemented):** CRUD, completion, due dates, priorities,
   filtering, sorting, SQLite persistence, and CSV export.
2. **Bonus automated tests (implemented):** test task operations with real SQLite,
   command parsing, CSV exports, and process exit behavior.
3. **Bonus validation:** review and expand edge-case handling. Basic field
   validation is already implemented and covered by automated tests.
4. **Bonus TUI:** add an interactive terminal interface with task navigation,
   creation, editing, completion, deletion, filtering, and clear validation errors.

The TUI is not implemented yet.

## Automated tests

```sh
go test ./...
go test ./... -cover
go test -race -shuffle=on ./...
go vet ./...
```

The suite uses Go's standard `testing` package with no additional dependencies.
SQLite databases and exports are created in temporary directories and cleaned up
automatically. Tests do not use your normal task database.

- `internal/task/store_test.go` checks persistence after reopening, validation,
  filters, sorting and tie order, partial updates, completion/reopening, deletion,
  and errors that must leave existing data intact.
- `internal/cli/cli_test.go` checks command syntax, defaults, database selection,
  help, error reporting, CSV contents and escaping, and overwrite protection.
- `main_test.go` runs the application entry point in subprocesses to check exit
  codes, stdout/stderr, and persistence between invocations.

The default storage location test uses Linux's `XDG_CONFIG_HOME` and is skipped
on other operating systems. The other tests use explicit temporary database
paths or `TMIG_DB`. The optional race detector requires a supported platform and
a C compiler, even though the normal application build does not.

## Manual testing checklist

Build with `go build -o bin/tmig .` and run `go vet ./...`. Use a new temporary
directory and set `TMIG_DB` to a database inside it so testing does not touch your
normal task list. Each command starts a new process, which also checks persistence.

- [ ] Run `--help` and `add --help`; inspect the usage text.
- [ ] List an empty database; expect `No tasks found.`
- [ ] Add three tasks with different priorities, two with due dates and one without.
- [ ] List again; confirm titles, IDs, dates, and default pending status.
- [ ] Sort by due date; confirm earliest first and the undated task last.
- [ ] Sort by priority; confirm high, medium, low.
- [ ] Update just a title; confirm its other fields are preserved.
- [ ] Clear a due date with `update ID --due ""`.
- [ ] Complete a task twice, filter by complete, then reopen it.
- [ ] Combine pending status and high priority filters; inspect the result.
- [ ] Export a title containing a comma and double quotes; inspect the CSV.
- [ ] Export a filter with no results; expect just the column header.
- [ ] Export twice to the same path; expect an error and the original file intact.
- [ ] Try an empty title, impossible date, invalid priority/status/sort, invalid ID,
      missing task ID, unknown flag, and an update without changes. Expect errors.
- [ ] Submit an update with both a new title and invalid date; confirm neither changes.
- [ ] Delete a task; confirm it is absent and deleting it again reports an error.
- [ ] Add another task; confirm a deleted ID is not reused.
- [ ] Repeat with a second `--db` path; confirm the task lists stay separate.
