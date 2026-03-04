# history-cleaner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/mahendrakalkura/history-cleaner)](https://goreportcard.com/report/github.com/mahendrakalkura/history-cleaner)
[![Platform](https://img.shields.io/badge/Platform-Linux-lightgrey?logo=linux&logoColor=white)](https://www.linux.org/)

A command-line tool to selectively delete browsing history from Firefox, Chrome, or Chromium by domain.

## Demo

![Demo](demo.gif)

## How it works

1. Choose a browser (Firefox, Chrome, or Chromium — only installed ones are shown)
2. If the browser is running, exit with an error
3. Choose a profile (skipped if only one exists)
4. Enter number of days to scan for domains
5. Select domains to delete from the list (with visit counts)
6. Confirm and delete — removes ALL history for selected domains, not just the scanned range
7. Shows deleted and remaining visit counts

## Walkthrough

### Step 1: Browser Selection

The tool auto-detects installed browsers by checking for their config directories. Only browsers found on your system are listed. If only one browser is installed, it is selected automatically.

- Firefox — `~/.mozilla/firefox/`
- Chrome — `~/.config/google-chrome/`
- Chromium — `~/.config/chromium/`

### Step 2: Running Check

The selected browser **must** be closed. The tool checks for running processes (e.g., `firefox`, `firefox-esr`, `chrome`, `google-chrome`, `chromium`, `chromium-browser`) and exits with an error if any are found.

### Step 3: Profile Selection

If the browser has multiple profiles, you are prompted to pick one. If only one profile exists, it is selected automatically.

- **Firefox** — reads `profiles.ini` and lists profiles by name and path
- **Chrome/Chromium** — reads the `Local State` JSON file to list profiles by display name, falling back to directory scanning if the file is missing

### Step 4: Date Range

You are asked how many days of history to scan (default: 1). This controls which domains appear in the next step. Enter a larger number to see domains from further back.

### Step 5: Domain Selection

A multi-select list shows all domains visited within the date range, sorted alphabetically. Each domain displays the number of visits found in the scanned range.

Use space to toggle domains and enter to confirm your selection.

> **Important:** The date range only controls which domains are _shown_. Deletion in the next step removes **all** history for the selected domains across all time, not just the scanned range.

### Step 6: Confirmation and Deletion

The selected domains are listed and you are asked to confirm. On confirmation, all visits and orphaned URL entries for the selected domains are deleted in a single database transaction. The tool then reports the number of deleted visits and how many visits remain in the database.

## Requirements

- Linux with `pgrep` available
- Firefox, Chrome, or Chromium installed
- Go 1.26+ and CGO enabled (for building)

## Build

```sh
make build
```

## Usage

```sh
./main
```

Close the target browser before running.

## Development

```sh
make build      # Format, fetch deps, tidy, compile
make lint       # Run golangci-lint
make test       # Run tests with coverage report
make run        # Build and run
make benchmarks # Run benchmarks
```

### Example Output

**`make test`:**
```
=== RUN   TestExtractHost
--- PASS: TestExtractHost (0.00s)
=== RUN   TestEscapeLike
--- PASS: TestEscapeLike (0.00s)
...
PASS
coverage: 26.1% of statements

Coverage:
26.1%
```

**`make benchmarks`:**
```
BenchmarkExtractHost-12    2031642    594.1 ns/op    432 B/op    3 allocs/op
BenchmarkEscapeLike-12    10979582    110.5 ns/op     32 B/op    2 allocs/op
```

## Dependencies

- [charmbracelet/huh](https://github.com/charmbracelet/huh) — interactive terminal forms
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite driver
- [gopkg.in/ini.v1](https://gopkg.in/ini.v1) — INI file parser
