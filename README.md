# tmig: Task Manager In Go

Project submission for [ChannelBound, LLC - Junior Developer & Data Analyst](https://www.indeed.com/viewjob?jk=7b37d65ed238eaa3&from=shareddesktop_copy).
[demo](https://github.com/user-attachments/assets/93a00c11-876a-4992-8939-d6b46f673536)

Implements **Option E: CLI Task Manager with SQLite Backend**, including all three
bonus features: automated tests, data validation, and a tui written with tview.

## Features

- Add, list, update, and delete tasks; mark complete or reopen.
- Set due dates, priorities, and optional multiline descriptions.
- Filter by status and priority; sort by ID, due date, or priority.
- Export filtered results to CSV with headers and proper field escaping.
- Manage tasks interactively with a `tview` terminal interface.

## Quick start

Requires **Go 1.25+**. SQLite is embedded; no database server or C compiler is needed.

```sh
git clone https://github.com/Sinnaminty/tmig.git
cd tmig
go build -o bin/tmig .
./bin/tmig --help
```

Try the following with a fresh demo database:

```sh
./bin/tmig --db demo.db add "Review imported records" --due 2026-09-21 --priority high
./bin/tmig --db demo.db update 1 --description "Check missing values and duplicate entries."
./bin/tmig --db demo.db list --status pending --sort due
./bin/tmig --db demo.db complete 1
./bin/tmig --db demo.db export --output demo-tasks.csv
./bin/tmig --db demo.db tui
./bin/tmig --db demo.db delete 1
```

In the TUI: `a` adds, `e` edits, Space completes/reopens, `d` deletes with
confirmation, `f` filters/sorts, and `q` quits. Tab moves between form fields;
Escape cancels. Use a terminal at least 76 × 24.

Run `./bin/tmig COMMAND --help` for options. Dates use `YYYY-MM-DD`; priorities
are `low`, `medium`, or `high`. Clear a due date with `update ID --due ""`.

By default, tasks are stored in `tmig/tasks.db` under the OS user configuration
directory. Override with `TMIG_DB` or `--db PATH` before the command.
CSV exports never overwrite existing files.

## Approach

The focus is straightforward Go code, reliable data handling, and easy verification:

- **CLI:** Go’s `flag` package parses arguments; `encoding/csv` writes exports.
- **Storage:** `database/sql` with `modernc.org/sqlite` handles persistence.
  SQL values are parameterized, and sorting uses a fixed set of expressions.
- **Validation:** Shared rules reject empty titles, invalid dates, and unsupported
  priorities/statuses. Updates validate every supplied field before writing,
  preserving existing data when input is invalid. Creation timestamps use UTC.
- **Structure:** `internal/task` owns storage and validation, `internal/cli`
  handles commands and output, and `internal/tui` handles interaction.
  Both interfaces share the same task operations.

> ##### powered by caffeine and lambda functions
