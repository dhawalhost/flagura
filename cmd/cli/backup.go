package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func runBackup(subCmd string, args []string, file string, compress bool) {
	switch subCmd {
	case "create", "export", "dump":
		if file == "" {
			timestamp := time.Now().UTC().Format("20060102-150405")
			if compress {
				file = fmt.Sprintf("flagura-backup-%s.json.gz", timestamp)
			} else {
				file = fmt.Sprintf("flagura-backup-%s.json", timestamp)
			}
		}

		path := "/api/v1/backup/export"
		if compress {
			path += "?compress=true"
		}

		url := fmt.Sprintf("%s%s", normalizeEndpoint(endpoint), path)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
			os.Exit(1)
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		if projectID != "" {
			req.Header.Set("X-Project-ID", projectID)
		}

		resp, err := getHttpClient().Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backup request failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(os.Stderr, "Backup failed (%d): %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		cleanFile := filepath.Clean(file)
		out, err := os.OpenFile(cleanFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open output file: %v\n", err)
			os.Exit(1)
		}
		defer out.Close()

		n, err := io.Copy(out, resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write backup data: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Database backup created successfully (%d bytes written to %s)\n", n, file)

	case "restore", "import", "load":
		if file == "" {
			fmt.Fprintf(os.Stderr, "Usage: flagura backup restore --file=<path>\n")
			os.Exit(1)
		}

		cleanFile := filepath.Clean(file)
		f, err := os.Open(cleanFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open backup file %s: %v\n", file, err)
			os.Exit(1)
		}
		defer f.Close()

		url := fmt.Sprintf("%s/api/v1/backup/import", normalizeEndpoint(endpoint))
		req, err := http.NewRequest(http.MethodPost, url, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create restore request: %v\n", err)
			os.Exit(1)
		}
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		if projectID != "" {
			req.Header.Set("X-Project-ID", projectID)
		}

		resp, err := getHttpClient().Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Restore request failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Restore failed (%d): %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		fmt.Printf("✓ Database snapshot restored successfully:\n%s\n", string(body))

	default:
		fmt.Fprintf(os.Stderr, "Usage: flagura backup [create|restore] --file=<path> [--compress]\n")
		os.Exit(1)
	}
}
