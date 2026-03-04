# history-cleaner

A command-line tool to selectively delete browsing history from Firefox, Chrome, or Chromium by domain.

## How it works

1. Choose a browser (Firefox, Chrome, or Chromium — only installed ones are shown)
2. If the browser is running, exit with an error
3. Choose a profile (skipped if only one exists)
4. Enter number of days to scan for domains
5. Select domains to delete from the list (with visit counts)
6. Confirm and delete — removes ALL history for selected domains, not just the scanned range
7. Shows deleted and remaining visit counts

## Requirements

- Linux with `pgrep` available
- Firefox, Chrome, or Chromium installed
- Go 1.25+ and CGO enabled (for building)

## Build

```sh
make build
```

## Usage

```sh
./main
```

Close the target browser before running.

## Dependencies

- [charmbracelet/huh](https://github.com/charmbracelet/huh) — interactive terminal forms
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite driver
