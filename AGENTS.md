# AGENTS.md

## Project Overview

**history-cleaner** is a single-binary Go CLI tool that selectively deletes browsing history by domain from Firefox, Chrome, or Chromium.

## Architecture

Single-file application (`main.go`, ~480 lines). No packages, no interfaces.

### Flow (6 screens)

1. **Browser select** — detects installed browsers, user picks one
2. **Running check** — exits if the chosen browser is open (checks multiple process names per browser)
3. **Profile select** — picks profile (Firefox: `profiles.ini`, Chrome/Chromium: `Local State` JSON or glob fallback)
4. **Date range input** — user enters number of days to scan (default: 1)
5. **Domain select** — multi-select of domains found in the date range, with visit counts
6. **Delete + report** — deletes ALL history for selected domains across all time in a single transaction, shows deleted/remaining counts

### Key functions

- `detectBrowsers()` — checks for `~/.mozilla/firefox/`, `~/.config/google-chrome/`, `~/.config/chromium/`
- `isRunning(processNames)` — `pgrep -x` check against multiple process name variants
- `selectProfile(title, options)` — shared UI helper for profile selection prompts
- `findFirefoxDB(configDir)` — parses `profiles.ini` for profile selection
- `findChromeDB(configDir, name)` — parses `Local State` JSON, falls back to globbing `*/History`
- `extractHost(rawURL)` — returns hostname from URL
- `queryHosts(db, kind, cutoff)` — queries recent history, returns domain→count map
- `deleteHosts(db, kind, domains)` — deletes visits + orphaned URLs in a transaction, returns deleted/remaining counts

### Browser differences

| | Firefox | Chrome/Chromium |
|---|---|---|
| DB file | `places.sqlite` | `History` |
| URL table | `moz_places` | `urls` |
| Visit table | `moz_historyvisits` | `visits` |
| Timestamp | Unix microseconds | Microseconds since 1601-01-01 |
| Profiles | `profiles.ini` | `Local State` JSON |
| Orphan cleanup | `foreign_count = 0` check | Simple `NOT IN` check |
| Process names | `firefox`, `firefox-esr` | `chrome`/`google-chrome`, `chromium`/`chromium-browser` |

## Tech Stack

- **Go 1.25** with CGO (required for sqlite3)
- **charmbracelet/huh** for terminal UI
- **mattn/go-sqlite3** for SQLite access

## Build

```sh
make build    # format, fetch deps, tidy, compile to ./main
make lint     # golangci-lint
```

## Conventions

- All code in a single `main.go`
- No comments in the code
- No tests currently exist
- Error handling: print to stderr and `os.Exit(1)`
- Use `huh` forms for all interactive prompts
- Linux-only (uses `pgrep`, reads browser config from home dir)
- Chromium reuses `browserChrome` kind (identical DB format)
- Deletions are wrapped in a transaction with rollback on error
- WAL checkpoint is run after opening the database
- **Always run `make build && make lint` after every code change** — must have 0 lint issues

## Gotchas

- Browser **must** be closed before running
- Date range input controls which domains are _shown_, but deletion removes ALL history for selected domains
- `extractHost` returns the full hostname, not just the registrable domain
- Chrome timestamps use Windows epoch (1601-01-01), offset by `chromeEpochOffsetUsec`
- Domain matching uses two LIKE patterns (`%://domain` and `%://domain/%`) to avoid substring false positives
