package main

import (
	"testing"
)

func TestIsSQLite(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"sqlite://data.db", true},
		{"sqlite3://data.db", true},
		{"file:local.db", true},
		{"flagura.db", true},
		{"flagura.sqlite", true},
		{"flagura.sqlite3", true},
		{"postgres://user:pass@localhost:5432/flagura", false},
		{"postgresql://user:pass@localhost:5432/flagura", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isSQLite(tt.url)
			if got != tt.want {
				t.Errorf("isSQLite(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
