package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/ini.v1"
)

type browserKind int

const (
	browserFirefox browserKind = iota
	browserChrome
)

type browser struct {
	name         string
	kind         browserKind
	processNames []string
	configDir    string
}

func detectBrowsers() []browser {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	candidates := []browser{
		{"Firefox", browserFirefox, []string{"firefox", "firefox-esr"}, filepath.Join(home, ".mozilla", "firefox")},
		{"Chrome", browserChrome, []string{"chrome", "google-chrome"}, filepath.Join(home, ".config", "google-chrome")},
		{"Chromium", browserChrome, []string{"chromium", "chromium-browser"}, filepath.Join(home, ".config", "chromium")},
	}

	var found []browser
	for _, b := range candidates {
		if info, err := os.Stat(b.configDir); err == nil && info.IsDir() {
			found = append(found, b)
		}
	}
	return found
}

func isRunning(processNames []string) bool {
	for _, name := range processNames {
		if exec.Command("pgrep", "-x", name).Run() == nil {
			return true
		}
	}
	return false
}

func selectProfile(title string, options []huh.Option[string]) (string, error) {
	var selected string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(&selected),
		),
	).Run(); err != nil {
		return "", fmt.Errorf("profile selection cancelled")
	}
	return selected, nil
}

func findFirefoxDB(configDir string) (string, error) {
	iniPath := filepath.Join(configDir, "profiles.ini")
	cfg, err := ini.Load(iniPath)
	if err != nil {
		return "", fmt.Errorf("reading profiles.ini: %w", err)
	}

	type profile struct {
		name string
		path string
	}

	var profiles []profile
	for _, section := range cfg.Sections() {
		if section.Name() == "DEFAULT" || !section.HasKey("Path") {
			continue
		}
		path := section.Key("Path").String()
		name := section.Key("Name").String()
		if name == "" {
			name = path
		}
		placesPath := filepath.Join(configDir, path, "places.sqlite")
		if _, err := os.Stat(placesPath); err == nil {
			profiles = append(profiles, profile{name, path})
		}
	}

	if len(profiles) == 0 {
		return "", fmt.Errorf("no profiles found in profiles.ini")
	}

	if len(profiles) == 1 {
		return filepath.Join(configDir, profiles[0].path, "places.sqlite"), nil
	}

	options := make([]huh.Option[string], len(profiles))
	for i, p := range profiles {
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", p.name, p.path), p.path)
	}

	selectedPath, err := selectProfile("Select Firefox profile", options)
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, selectedPath, "places.sqlite"), nil
}

func findChromeDB(configDir string, browserName string) (string, error) {
	type profile struct {
		name string
		dir  string
	}

	var profiles []profile

	localStatePath := filepath.Join(configDir, "Local State")
	if data, err := os.ReadFile(localStatePath); err == nil {
		var state struct {
			Profile struct {
				InfoCache map[string]struct {
					Name string `json:"name"`
				} `json:"info_cache"`
			} `json:"profile"`
		}
		if err := json.Unmarshal(data, &state); err == nil {
			for dir, info := range state.Profile.InfoCache {
				name := info.Name
				if name == "" {
					name = dir
				}
				histPath := filepath.Join(configDir, dir, "History")
				if _, err := os.Stat(histPath); err == nil {
					profiles = append(profiles, profile{name, dir})
				}
			}
		}
	}

	if len(profiles) == 0 {
		matches, _ := filepath.Glob(filepath.Join(configDir, "*", "History"))
		for _, m := range matches {
			dir := filepath.Base(filepath.Dir(m))
			profiles = append(profiles, profile{dir, dir})
		}
	}

	if len(profiles) == 0 {
		return "", fmt.Errorf("no %s profiles found", browserName)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].name < profiles[j].name
	})

	if len(profiles) == 1 {
		return filepath.Join(configDir, profiles[0].dir, "History"), nil
	}

	options := make([]huh.Option[string], len(profiles))
	for i, p := range profiles {
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", p.name, p.dir), p.dir)
	}

	selectedDir, err := selectProfile(fmt.Sprintf("Select %s profile", browserName), options)
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, selectedDir, "History"), nil
}

func findDB(b browser) (string, error) {
	if b.kind == browserFirefox {
		return findFirefoxDB(b.configDir)
	}
	return findChromeDB(b.configDir, b.name)
}

const chromeEpochOffsetUsec = 11644473600 * 1_000_000

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func extractHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func queryHosts(db *sql.DB, kind browserKind, cutoff time.Time) (map[string]int, error) {
	var query string
	var param int64

	switch kind {
	case browserFirefox:
		query = `
			SELECT DISTINCT p.url
			FROM moz_places p
			INNER JOIN moz_historyvisits v ON v.place_id = p.id
			WHERE v.visit_date >= ?`
		param = cutoff.UnixMicro()
	default:
		query = `
			SELECT DISTINCT u.url
			FROM urls u
			INNER JOIN visits v ON v.url = u.id
			WHERE v.visit_time >= ?`
		param = cutoff.UnixMicro() + chromeEpochOffsetUsec
	}

	rows, err := db.Query(query, param)
	if err != nil {
		return nil, fmt.Errorf("querying history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	hostSet := map[string]int{}
	var scanErrors []error
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			scanErrors = append(scanErrors, err)
			continue
		}
		host := extractHost(u)
		if host != "" {
			hostSet[host]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating history rows: %w", err)
	}
	if len(scanErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d row(s) could not be scanned:\n", len(scanErrors))
		for _, err := range scanErrors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
	}
	return hostSet, nil
}

func deleteHosts(db *sql.DB, kind browserKind, domains []string) (deleted int, remaining int, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	totalDeleted := 0
	for _, domain := range domains {
		escapedDomain := escapeLike(domain)
		exactPattern := "%://" + escapedDomain
		pathPattern := "%://" + escapedDomain + "/%"

		var res sql.Result
		switch kind {
		case browserFirefox:
			res, err = tx.Exec(`
				DELETE FROM moz_historyvisits
				WHERE place_id IN (
					SELECT id FROM moz_places WHERE url LIKE ? ESCAPE '\' OR url LIKE ? ESCAPE '\'
				)`, exactPattern, pathPattern)
		default:
			res, err = tx.Exec(`
				DELETE FROM visits
				WHERE url IN (
					SELECT id FROM urls WHERE url LIKE ? ESCAPE '\' OR url LIKE ? ESCAPE '\'
				)`, exactPattern, pathPattern)
		}
		if err != nil {
			return totalDeleted, 0, fmt.Errorf("deleting visits for %s: %w", domain, err)
		}

		n, _ := res.RowsAffected()
		totalDeleted += int(n)

		switch kind {
		case browserFirefox:
			_, err = tx.Exec(`
				DELETE FROM moz_places
				WHERE (url LIKE ? ESCAPE '\' OR url LIKE ? ESCAPE '\')
				AND id NOT IN (SELECT place_id FROM moz_historyvisits)
				AND foreign_count = 0`, exactPattern, pathPattern)
		default:
			_, err = tx.Exec(`
				DELETE FROM urls
				WHERE (url LIKE ? ESCAPE '\' OR url LIKE ? ESCAPE '\')
				AND id NOT IN (SELECT url FROM visits)`, exactPattern, pathPattern)
		}
		if err != nil {
			return totalDeleted, 0, fmt.Errorf("cleaning orphans for %s: %w", domain, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return totalDeleted, 0, fmt.Errorf("committing transaction: %w", err)
	}

	var countQuery string
	switch kind {
	case browserFirefox:
		countQuery = `SELECT COUNT(*) FROM moz_historyvisits`
	default:
		countQuery = `SELECT COUNT(*) FROM visits`
	}
	if err := db.QueryRow(countQuery).Scan(&remaining); err != nil {
		return totalDeleted, 0, fmt.Errorf("counting remaining visits: %w", err)
	}

	return totalDeleted, remaining, nil
}

func main() {
	browsers := detectBrowsers()
	if len(browsers) == 0 {
		fmt.Fprintln(os.Stderr, "No supported browsers found.")
		os.Exit(1)
	}

	var chosen browser
	if len(browsers) == 1 {
		chosen = browsers[0]
	} else {
		options := make([]huh.Option[int], len(browsers))
		for i, b := range browsers {
			options[i] = huh.NewOption(b.name, i)
		}

		var idx int
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[int]().
					Title("Select browser").
					Options(options...).
					Value(&idx),
			),
		).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			os.Exit(0)
		}
		chosen = browsers[idx]
	}

	if isRunning(chosen.processNames) {
		fmt.Fprintf(os.Stderr, "%s is running! Close it first, then re-run this tool.\n", chosen.name)
		os.Exit(1)
	}

	dbPath, err := findDB(chosen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding database: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Using: %s\n", dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")

	var daysStr string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("How many days of history to scan?").
				Description("Domains found in this range will be shown for selection.\nDeletion will remove ALL history for selected domains.").
				Placeholder("1").
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					n, err := strconv.Atoi(s)
					if err != nil || n < 1 {
						return fmt.Errorf("enter a positive number")
					}
					return nil
				}).
				Value(&daysStr),
		),
	).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		os.Exit(0)
	}

	days := 1
	if daysStr != "" {
		days, _ = strconv.Atoi(daysStr)
	}

	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	hostCounts, err := queryHosts(db, chosen.kind, cutoff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(hostCounts) == 0 {
		fmt.Printf("No history entries found in the last %d day(s).\n", days)
		return
	}

	type hostCount struct {
		host  string
		count int
	}
	hosts := make([]hostCount, 0, len(hostCounts))
	for h, c := range hostCounts {
		hosts = append(hosts, hostCount{h, c})
	}
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].host < hosts[j].host
	})

	domainOptions := make([]huh.Option[string], len(hosts))
	for i, h := range hosts {
		domainOptions[i] = huh.NewOption(fmt.Sprintf("%-40s (%d visits)", h.host, h.count), h.host)
	}

	var selected []string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("Select domains to delete (found in last %d day(s), deletion is across ALL time)", days)).
				Options(domainOptions...).
				Value(&selected).
				Height(25),
		),
	).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		os.Exit(0)
	}

	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return
	}

	fmt.Printf("\nWill delete ALL history for %d domain(s):\n", len(selected))
	for _, s := range selected {
		fmt.Printf("  - %s\n", s)
	}

	var confirm bool
	if err := huh.NewConfirm().
		Title("Proceed with deletion?").
		Value(&confirm).
		Run(); err != nil || !confirm {
		fmt.Println("Aborted.")
		return
	}

	deleted, remaining, err := deleteHosts(db, chosen.kind, selected)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nDone. Deleted %d visit(s). %d visit(s) remaining.\n", deleted, remaining)
}
