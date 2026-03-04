package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/path", "example.com"},
		{"https://example.com:8080/path", "example.com"},
		{"http://sub.example.com", "sub.example.com"},
		{"https://example.com", "example.com"},
		{"invalid-url", ""},
		{"", ""},
		{"ftp://files.example.com", "files.example.com"},
		{"https://EXAMPLE.COM/path", "example.com"},
		{"https://192.168.1.1/path", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractHost(tt.input)
			if result != tt.expected {
				t.Errorf("extractHost(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"test%domain", "test\\%domain"},
		{"test_domain", "test\\_domain"},
		{"%_%", "\\%\\_\\%"},
		{"normal.host.name", "normal.host.name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeLike(tt.input)
			if result != tt.expected {
				t.Errorf("escapeLike(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectBrowsers(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME: %v", err)
		}
	}()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".mozilla", "firefox"), 0755); err != nil {
		t.Fatalf("Failed to create firefox dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".config", "google-chrome"), 0755); err != nil {
		t.Fatalf("Failed to create chrome dir: %v", err)
	}

	browsers := detectBrowsers()

	foundFirefox := false
	foundChrome := false
	for _, b := range browsers {
		if b.name == "Firefox" {
			foundFirefox = true
		}
		if b.name == "Chrome" {
			foundChrome = true
		}
	}

	if !foundFirefox {
		t.Error("Expected to find Firefox browser")
	}
	if !foundChrome {
		t.Error("Expected to find Chrome browser")
	}
}

func TestQueryHosts(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}
	}()

	_, err = db.Exec(`
		CREATE TABLE moz_places (
			id INTEGER PRIMARY KEY,
			url TEXT
		);
		CREATE TABLE moz_historyvisits (
			id INTEGER PRIMARY KEY,
			place_id INTEGER,
			visit_date INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	now := time.Now()
	_, err = db.Exec(`
		INSERT INTO moz_places (id, url) VALUES
		(1, 'https://example.com/page1'),
		(2, 'https://example.com/page2'),
		(3, 'https://test.com/page1'),
		(4, 'invalid-url');

		INSERT INTO moz_historyvisits (id, place_id, visit_date) VALUES
		(1, 1, ?),
		(2, 2, ?),
		(3, 3, ?),
		(4, 1, ?);
	`, now.UnixMicro(), now.UnixMicro(), now.UnixMicro(), now.UnixMicro()-1000000)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	cutoff := now.Add(-time.Hour)
	hosts, err := queryHosts(db, browserFirefox, cutoff)
	if err != nil {
		t.Fatalf("queryHosts failed: %v", err)
	}

	if hosts["example.com"] != 2 {
		t.Errorf("Expected example.com to have 2 visits, got %d", hosts["example.com"])
	}
	if hosts["test.com"] != 1 {
		t.Errorf("Expected test.com to have 1 visit, got %d", hosts["test.com"])
	}
}

func TestDeleteHosts(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close DB: %v", err)
		}
	}()

	_, err = db.Exec(`
		CREATE TABLE moz_places (
			id INTEGER PRIMARY KEY,
			url TEXT,
			foreign_count INTEGER DEFAULT 0
		);
		CREATE TABLE moz_historyvisits (
			id INTEGER PRIMARY KEY,
			place_id INTEGER,
			visit_date INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	now := time.Now()
	_, err = db.Exec(`
		INSERT INTO moz_places (id, url, foreign_count) VALUES
		(1, 'https://example.com/page1', 0),
		(2, 'https://example.com/page2', 0),
		(3, 'https://test.com/page1', 0);

		INSERT INTO moz_historyvisits (id, place_id, visit_date) VALUES
		(1, 1, ?),
		(2, 2, ?),
		(3, 3, ?);
	`, now.UnixMicro(), now.UnixMicro(), now.UnixMicro())
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	deleted, remaining, err := deleteHosts(db, browserFirefox, []string{"example.com"})
	if err != nil {
		t.Fatalf("deleteHosts failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected 2 deleted visits, got %d", deleted)
	}
	if remaining != 1 {
		t.Errorf("Expected 1 remaining visit, got %d", remaining)
	}
}

func BenchmarkExtractHost(b *testing.B) {
	urls := []string{
		"https://example.com/path",
		"http://sub.domain.example.com:8080/path/to/resource",
		"ftp://files.example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, url := range urls {
			extractHost(url)
		}
	}
}

func BenchmarkEscapeLike(b *testing.B) {
	domains := []string{
		"example.com",
		"test%domain",
		"test_domain",
		"normal.host.name",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, domain := range domains {
			escapeLike(domain)
		}
	}
}

func ExampleextractHost() {
	fmt.Println(extractHost("https://example.com/path"))
	fmt.Println(extractHost("https://sub.example.com:8080/"))
	fmt.Println(extractHost("invalid"))
	// Output:
	// example.com
	// sub.example.com
	//
}
