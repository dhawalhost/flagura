package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func runAudit(dir string, failOnStale bool) {
	_, body, err := makeRequest(http.MethodGet, "/api/v1/flags", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var data struct {
		Flags []domain.FeatureFlag `json:"flags"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags response: %v\n", err)
		os.Exit(1)
	}

	extensions := map[string]bool{
		".go": true, ".ts": true, ".js": true, ".tsx": true, ".jsx": true,
		".py": true, ".rs": true, ".java": true, ".kt": true, ".swift": true,
	}

	type FlagAuditItem struct {
		Flag        domain.FeatureFlag
		Health      domain.FlagHealthReport
		Occurrences int
		Files       []string
	}

	var items []FlagAuditItem
	for _, f := range data.Flags {
		items = append(items, FlagAuditItem{
			Flag:   f,
			Health: domain.AnalyzeFlagHealth(f),
		})
	}

	if dir != "" {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				if info != nil && (info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}

			ext := filepath.Ext(path)
			if !extensions[ext] {
				return nil
			}

			cleanPath := filepath.Clean(path)
			// Skip symlinks to prevent TOCTOU traversal (CWE-367)
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			// #nosec G304 G122 -- path is bounded by directory walk and cleaned
			content, err := os.ReadFile(cleanPath)
			if err != nil {
				return nil
			}
			contentStr := string(content)

			for i := range items {
				k := items[i].Flag.Key
				if strings.Contains(contentStr, `"`+k+`"`) || strings.Contains(contentStr, `'`+k+`'`) {
					items[i].Occurrences++
					items[i].Files = append(items[i].Files, path)
				}
			}

			return nil
		})
	}

	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(items)
		return
	}

	staleCount := 0
	fmt.Println("\n🧹 Flagura Codebase Audit & Technical Debt Report")
	fmt.Println("─────────────────────────────────────────────────────────────────")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "STATUS\tFLAG KEY\tHEALTH STATE\tCODE REFS\tRECOMMENDATION")
	fmt.Fprintln(w, "------\t--------\t------------\t---------\t--------------")

	for _, item := range items {
		statusIcon := "🟢 Active"
		if item.Health.IsStale {
			staleCount++
			if item.Health.Status == domain.HealthStatusStale {
				statusIcon = "🟡 Stale (100%)"
			} else {
				statusIcon = "🔴 Dead Code"
			}
		}
		refCountStr := fmt.Sprintf("%d file(s)", len(item.Files))
		if dir == "" || len(item.Files) == 0 {
			refCountStr = "0 refs"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", statusIcon, item.Flag.Key, item.Health.Status, refCountStr, item.Health.SuggestedAction)
	}
	_ = w.Flush()

	if staleCount == 0 {
		fmt.Println("\n✅ No technical debt detected! All feature flags are actively routing traffic.")
	} else {
		fmt.Printf("\n⚠️  Found %d flag(s) ready for code cleanup.\n", staleCount)
	}
	fmt.Println()

	if failOnStale && staleCount > 0 {
		fmt.Printf("❌ Failed CI: Found %d stale feature flag(s) that should be removed from code.\n", staleCount)
		os.Exit(1)
	}
}
