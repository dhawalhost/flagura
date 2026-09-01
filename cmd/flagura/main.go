package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

var (
	version   = "v1.4.0"
	endpoint  string
	apiKey    string
	env       string
	projectID string
	jsonOut   bool
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	// Read environment variables
	defaultEndpoint := os.Getenv("FLAGURA_ENDPOINT")
	if defaultEndpoint == "" {
		defaultEndpoint = "http://localhost:3000"
	}
	defaultApiKey := os.Getenv("FLAGURA_API_KEY")
	defaultProject := os.Getenv("FLAGURA_PROJECT_ID")

	cmd := os.Args[1]

	// Global Flags
	fs := flag.NewFlagSet("flagura", flag.ExitOnError)
	fs.StringVar(&endpoint, "endpoint", defaultEndpoint, "Flagura control plane URL")
	fs.StringVar(&apiKey, "api-key", defaultApiKey, "API key for authentication")
	fs.StringVar(&env, "env", "production", "Target environment (production, staging, development)")
	fs.StringVar(&projectID, "project", defaultProject, "Project ID scope (env: FLAGURA_PROJECT_ID)")
	fs.BoolVar(&jsonOut, "json", false, "Output results as formatted JSON")

	switch cmd {
	case "version", "-v", "--version":
		fmt.Printf("Flagura CLI %s\n", version)
	case "list", "ls":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		runList()
	case "get":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura get <flag-key>\n")
			os.Exit(1)
		}
		runGet(fs.Arg(0))
	case "toggle":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura toggle <flag-key> [--env=production]\n")
			os.Exit(1)
		}
		runToggle(fs.Arg(0))
	case "rollout":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura rollout <flag-key> <percentage> [--env=production]\n")
			os.Exit(1)
		}
		pct, err := strconv.ParseFloat(strings.TrimSuffix(fs.Arg(1), "%"), 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid rollout percentage: %v\n", fs.Arg(1))
			os.Exit(1)
		}
		runRollout(fs.Arg(0), pct)
	case "evaluate", "eval":
		userFlag := fs.String("user", "usr_cli_01", "User ID for evaluation context")
		emailFlag := fs.String("email", "dev@flagura.dev", "Email for evaluation context")
		traceFlag := fs.Bool("trace", false, "Include step-by-step visual execution trace")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura evaluate <flag-key> [--user=<id>] [--email=<email>] [--trace]\n")
			os.Exit(1)
		}
		runEvaluate(fs.Arg(0), *userFlag, *emailFlag, *traceFlag)
	case "promote":
		fromFlag := fs.String("from", "staging", "Source environment")
		toFlag := fs.String("to", "production", "Destination environment")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura promote <flag-key> [--from=staging] [--to=production]\n")
			os.Exit(1)
		}
		runPromote(fs.Arg(0), *fromFlag, *toFlag)
	case "experiment", "exp":
		metricFlag := fs.String("metric", "conversion", "Metric name to analyze")
		controlFlag := fs.String("control", "control", "Control/baseline variant key")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura experiment <flag-key> [--metric=conversion] [--control=control]\n")
			os.Exit(1)
		}
		runExperiment(fs.Arg(0), *metricFlag, *controlFlag)
	case "canary":
		stagesFlag := fs.String("stages", "5%:5m,25%:30m,50%:1h,100%:0s", "Progressive stages (<pct>:<duration>,...)")
		rollbackFlag := fs.Bool("rollback", false, "Trigger immediate health guardrail rollback to 0%")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura canary <flag-key> [--stages=<stages>] [--rollback]\n")
			os.Exit(1)
		}
		runCanary(fs.Arg(0), *stagesFlag, *rollbackFlag)
	case "change-request", "cr":
		statusFlag := fs.String("status", "", "Filter status: PENDING, APPROVED, REJECTED, APPLIED")
		commentsFlag := fs.String("comments", "", "Review comments")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		subCmd := "list"
		if fs.NArg() > 0 {
			subCmd = fs.Arg(0)
		}
		runChangeRequest(subCmd, fs.Args(), *statusFlag, *commentsFlag)
	case "api-key", "apikey", "key":
		nameFlag := fs.String("name", "CLI Service Key", "Descriptive name for the API Key")
		roleFlag := fs.String("role", "developer", "Assigned role: developer or admin")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		subCmd := "list"
		if fs.NArg() > 0 {
			subCmd = fs.Arg(0)
		}
		runAPIKey(subCmd, fs.Args(), *nameFlag, *roleFlag)
	case "audit", "scan", "clean-up", "cleanup":
		dirFlag := fs.String("dir", ".", "Root directory of codebase to scan")
		failOnStale := fs.Bool("fail-on-stale", false, "Exit with error code 1 if stale flags are detected")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		runAudit(*dirFlag, *failOnStale)
	case "health":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		runHealth()
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'flagura help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`⚡ Flagura Developer CLI (%s)
High-performance feature flag management and terminal evaluation engine.

Usage:
  flagura <command> [arguments] [flags]

Commands:
  list, ls                  List all feature flags across environments
  get <key>                 View detailed configuration for a specific flag
  toggle <key>              Instantly toggle the master kill-switch for a flag
  rollout <key> <pct>       Adjust gradual rollout percentage (0-100%%)
  evaluate <key>            Execute real-time flag evaluation in terminal
  promote <key>             Promote configuration from Staging to Production
  experiment <key>          Calculate A/B statistical significance and lift
  canary <key>              Manage automated progressive canary ramp & guardrails
  change-request [list|approve|reject|apply]
                            Enforce 4-Eyes principle change approval governance
  api-key [list|create|revoke]
                            Generate, list, and revoke programmatic API service tokens
  audit, scan, clean-up     Scan codebase files for technical debt & stale flag checks
  health                    Check connection to the Flagura control plane
  version                   Print CLI version
  help                      Show this help message

Flags:
  --endpoint <url>          Flagura control plane URL (default: $FLAGURA_ENDPOINT or http://localhost:3000)
  --api-key <key>           API key for authentication (default: $FLAGURA_API_KEY)
  --env <name>              Target environment: production, staging, development (default: production)
  --name <name>             Name for newly generated API Key (default: CLI Service Key)
  --role <role>             Role for API Key: developer or admin (default: developer)
  --dir <path>              Codebase directory to scan during audit (default: .)
  --fail-on-stale           Exit with code 1 if stale/dead flags are found (useful in CI/CD)
  --json                    Output results as raw formatted JSON

Examples:
  flagura list
  flagura toggle ai-smart-search --env=production
  flagura rollout ai-smart-search 50%% --env=production
  flagura evaluate ai-smart-search --user=usr_alex_42 --trace
  flagura promote ai-smart-search --from=staging --to=production
  flagura api-key create --name="Production Kubernetes SDK" --role=admin
  flagura api-key list
  flagura audit --dir=. --fail-on-stale
`, version)
}

func getHttpClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second}
}

func makeRequest(method, path string, body interface{}) (*http.Response, []byte, error) {
	url := fmt.Sprintf("%s%s", strings.TrimRight(endpoint, "/"), path)
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	return resp, respBytes, err
}

func runList() {
	_, body, err := makeRequest(http.MethodGet, "/api/v1/flags", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Flagura: %v\n", err)
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	var data struct {
		Flags []domain.FeatureFlag `json:"flags"`
		Count int                  `json:"count"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n⚡ Flagura Feature Flags (%d flags)\n", len(data.Flags))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "KEY\tTYPE\tSTATUS (PROD)\tROLLOUT\tSTRATEGY\tHEALTH")
	fmt.Fprintln(w, "---\t----\t-------------\t-------\t--------\t------")

	for _, f := range data.Flags {
		prodEnv := f.Environments[domain.EnvProduction]
		status := "🟢 LIVE"
		if !prodEnv.Enabled {
			status = "🔴 OFF"
		}
		rollout := fmt.Sprintf("%.0f%%", prodEnv.Percentage)
		health := "✅ Active"
		hReport := domain.AnalyzeFlagHealth(f)
		if hReport.Status == domain.HealthStatusStale {
			health = "🧹 Ready for Cleanup"
		} else if hReport.Status == domain.HealthStatusDead {
			health = "⚠️ Dead Flag"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			f.Key, f.Type, status, rollout, string(prodEnv.Strategy), health)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering output table: %v\n", err)
	}
	fmt.Println()
}

func runGet(key string) {
	_, body, err := makeRequest(http.MethodGet, "/api/v1/flags", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var data struct {
		Flags []domain.FeatureFlag `json:"flags"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	var target *domain.FeatureFlag
	for _, f := range data.Flags {
		if f.Key == key || f.ID == key {
			target = &f
			break
		}
	}

	if target == nil {
		fmt.Fprintf(os.Stderr, "Flag %q not found\n", key)
		os.Exit(1)
	}

	if jsonOut {
		out, _ := json.MarshalIndent(target, "", "  ")
		fmt.Println(string(out))
		return
	}

	fmt.Printf("\n⚡ Flag: %s\n", target.Key)
	fmt.Printf("Name:        %s\n", target.Name)
	fmt.Printf("Description: %s\n", target.Description)
	fmt.Printf("Type:        %s\n", target.Type)
	fmt.Printf("Tags:        %s\n", strings.Join(target.Tags, ", "))

	for envKey, cfg := range target.Environments {
		fmt.Printf("\n── Environment: %s ──\n", envKey)
		fmt.Printf("  Status:     %t (Strategy: %s)\n", cfg.Enabled, cfg.Strategy)
		fmt.Printf("  Rollout:    %.1f%%\n", cfg.Percentage)
		fmt.Printf("  Rules:      %d active\n", len(cfg.Rules))
		for i, r := range cfg.Rules {
			fmt.Printf("    [%d] %s: %s %s %v (Action: %s)\n", i+1, r.Name, r.Attribute, r.Operator, r.Values, r.Action)
		}
	}
	fmt.Println()
}

func runToggle(key string) {
	resp, body, err := makeRequest(http.MethodPatch, fmt.Sprintf("/api/v1/flags/%s/toggle?env=%s", key, env), nil)
	if err != nil || resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Failed to toggle flag: %s\n", string(body))
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	fmt.Printf("✓ Successfully toggled flag %q in %s environment.\n", key, env)
}

func runRollout(key string, pct float64) {
	payload := map[string]interface{}{
		"environment": env,
		"percentage":  pct,
	}
	resp, body, err := makeRequest(http.MethodPatch, fmt.Sprintf("/api/v1/flags/%s/rollout", key), payload)
	if err != nil || resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Failed to update rollout: %s\n", string(body))
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	fmt.Printf("✓ Set rollout percentage of %q to %.1f%% in %s environment.\n", key, pct, env)
}

func runEvaluate(key string, userID string, email string, trace bool) {
	path := "/api/v1/evaluate"
	if trace {
		path += "?trace=true"
	}

	payload := map[string]interface{}{
		"flags": []string{key},
		"context": map[string]interface{}{
			"user_id":     userID,
			"email":       email,
			"environment": env,
		},
		"trace": trace,
	}

	_, body, err := makeRequest(http.MethodPost, path, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Evaluation failed: %v\n", err)
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	var data struct {
		Results map[string]domain.EvaluationResult `json:"results"`
		Traces  map[string]struct {
			Steps []struct {
				StepIndex int    `json:"step_index"`
				Name      string `json:"name"`
				Passed    bool   `json:"passed"`
				Detail    string `json:"detail"`
			} `json:"steps"`
			FinalReason domain.EvaluationReason `json:"final_reason"`
			FinalVariant string                 `json:"final_variant"`
			FinalEnabled bool                   `json:"final_enabled"`
			Bucket      float64                `json:"bucket"`
			ElapsedNs   int64                  `json:"elapsed_ns"`
		} `json:"traces"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding evaluation response: %v\n", err)
		os.Exit(1)
	}

	res, ok := data.Results[key]
	if !ok {
		fmt.Fprintf(os.Stderr, "Flag %q missing from evaluation response\n", key)
		os.Exit(1)
	}

	status := "🔴 OFF (Disabled)"
	if res.Enabled {
		status = "🟢 ON (Enabled)"
	}

	fmt.Printf("\n⚡ Evaluation Result: %s\n", key)
	fmt.Printf("Status:    %s\n", status)
	fmt.Printf("Variant:   %s\n", res.Variant)
	fmt.Printf("Reason:    %s\n", res.Reason)
	fmt.Printf("Latency:   %d ns (%.3f µs)\n", res.EvaluationLatencyNs, res.EvaluationLatencyUs)

	if trace {
		if t, ok := data.Traces[key]; ok && len(t.Steps) > 0 {
			fmt.Printf("\n── Visual Execution Trace (%d steps) ──\n", len(t.Steps))
			for _, step := range t.Steps {
				icon := "✓"
				if !step.Passed {
					icon = "✗"
				}
				fmt.Printf("  [%s] Step %d: %s\n", icon, step.StepIndex, step.Name)
				fmt.Printf("      ↳ %s\n", step.Detail)
			}
		}
	}
	fmt.Println()
}

func runPromote(key string, from string, to string) {
	path := fmt.Sprintf("/api/v1/flags/%s/promote?from=%s&to=%s", key, from, to)
	resp, body, err := makeRequest(http.MethodPost, path, nil)
	if err != nil || resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Promotion failed: %s\n", string(body))
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	fmt.Printf("✓ Successfully promoted flag %q from %s to %s.\n", key, from, to)
}

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

func runHealth() {
	t0 := time.Now()
	resp, _, err := makeRequest(http.MethodGet, "/healthz", nil)
	latency := time.Since(t0)

	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Printf("🔴 Flagura Control Plane unreachable at %s (%v)\n", endpoint, err)
		os.Exit(1)
	}

	fmt.Printf("🟢 Flagura Control Plane is healthy and responsive at %s (RTT: %v)\n", endpoint, latency)
}

func runExperiment(key, metric, control string) {
	path := fmt.Sprintf("/api/v1/experiments/%s?metric=%s&control=%s&env=%s", key, metric, control, env)
	resp, body, err := makeRequest(http.MethodGet, path, nil)
	if err != nil || resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Failed to retrieve experiment report: %s\n", string(body))
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	var report domain.ExperimentReport
	if err := json.Unmarshal(body, &report); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing experiment report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n📊 Flagura A/B Experiment Analysis: %s\n", report.FlagKey)
	fmt.Printf("Metric:          %s (Type: %s)\n", report.MetricName, report.EventType)
	fmt.Printf("Control Variant: %s\n", report.ControlVariant)
	fmt.Printf("Total Events:    %d\n", report.TotalEvents)
	if report.WinnerVariant != "" {
		fmt.Printf("🏆 Winner:       %s (Statistically Significant)\n", report.WinnerVariant)
	}

	fmt.Println("\n── Variant Performance ──")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VARIANT\tEXPOSURES\tCONVERSIONS\tRATE\t95% CONFIDENCE INTERVAL")
	fmt.Fprintln(w, "-------\t---------\t-----------\t----\t-----------------------")

	for v, s := range report.VariantStats {
		ci := fmt.Sprintf("[%.2f%%, %.2f%%]", s.CI95Lower*100, s.CI95Upper*100)
		fmt.Fprintf(w, "%s\t%d\t%d\t%.2f%%\t%s\n", v, s.Exposures, s.Conversions, s.ConversionRate*100, ci)
	}
	_ = w.Flush()

	if len(report.Comparisons) > 0 {
		fmt.Println("\n── Statistical Significance vs. Control ──")
		wComp := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(wComp, "TREATMENT\tREL LIFT\tZ-SCORE\tP-VALUE\tCONFIDENCE\tSTATUS")
		fmt.Fprintln(wComp, "---------\t--------\t-------\t-------\t----------\t------")

		for v, c := range report.Comparisons {
			lift := fmt.Sprintf("%+.2f%%", c.RelativeLiftPct)
			conf := fmt.Sprintf("%.1f%%", c.ConfidencePct)
			statusStr := string(c.Status)
			if c.Status == domain.ExpStatusWinning {
				statusStr = "🟢 WINNING"
			} else if c.Status == domain.ExpStatusLosing {
				statusStr = "🔴 LOSING"
			} else if c.Status == domain.ExpStatusInsufficientData {
				statusStr = "⏳ NEED SAMPLES"
			}

			fmt.Fprintf(wComp, "%s\t%s\t%.2f\t%.4f\t%s\t%s\n", v, lift, c.ZScore, c.PValue, conf, statusStr)
		}
		_ = wComp.Flush()
	}
	fmt.Println()
}

func runCanary(key, stagesStr string, isRollback bool) {
	if isRollback {
		path := fmt.Sprintf("/api/v1/flags/%s/canary/rollback", key)
		payload := map[string]string{"reason": "CLI Manual Health Rollback Triggered"}
		resp, body, err := makeRequest(http.MethodPost, path, payload)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Rollback failed: %s\n", string(body))
			os.Exit(1)
		}
		fmt.Printf("🚨 Health Rollback Executed: Flag '%s' has been reverted to 0%% rollout.\n", key)
		return
	}

	// Parse stages format: "5%:5m,25%:30m,50%:1h,100%:0s"
	parts := strings.Split(stagesStr, ",")
	var stages []domain.CanaryStage
	for i, part := range parts {
		sub := strings.Split(strings.TrimSpace(part), ":")
		pctStr := strings.TrimSuffix(sub[0], "%")
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid percentage in stage '%s': %v\n", part, err)
			os.Exit(1)
		}
		var durSec int64 = 0
		if len(sub) > 1 && sub[1] != "" {
			d, err := time.ParseDuration(sub[1])
			if err == nil {
				durSec = int64(d.Seconds())
			}
		}
		stages = append(stages, domain.CanaryStage{
			Index:            i,
			TargetPercentage: pct,
			DurationSec:      durSec,
		})
	}

	sched := domain.CanarySchedule{
		FlagKey:     key,
		Environment: domain.Environment(env),
		Stages:      stages,
		Guardrails: domain.CanaryGuardrails{
			MaxErrorRatePct: 1.0,
			AutoRollback:    true,
		},
	}

	path := fmt.Sprintf("/api/v1/flags/%s/canary", key)
	resp, body, err := makeRequest(http.MethodPost, path, sched)
	if err != nil || resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Failed to submit canary schedule: %s\n", string(body))
		os.Exit(1)
	}

	if jsonOut {
		fmt.Println(string(body))
		return
	}

	fmt.Printf("\n🚀 Progressive Canary Auto-Ramp Scheduled for Flag '%s' (%s)\n", key, env)
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Printf("Stage Count:  %d\n", len(stages))
	fmt.Printf("Initial Step: %.1f%% Rollout (Applied immediately)\n", stages[0].TargetPercentage)
	fmt.Printf("Guardrails:   Max Error Rate < 1.0%% (Auto-Rollback Active)\n\n")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "STAGE\tTARGET ROLLOUT\tSTAGE DURATION")
	fmt.Fprintln(w, "-----\t--------------\t--------------")
	for _, s := range stages {
		durStr := "Until Completion"
		if s.DurationSec > 0 {
			durStr = (time.Duration(s.DurationSec) * time.Second).String()
		}
		fmt.Fprintf(w, "Step %d\t%.1f%%\t%s\n", s.Index+1, s.TargetPercentage, durStr)
	}
	_ = w.Flush()
	fmt.Println()
}

func runChangeRequest(subCmd string, args []string, status, comments string) {
	switch subCmd {
	case "list", "ls":
		path := "/api/v1/change-requests"
		if status != "" {
			path += "?status=" + status
		}
		resp, body, err := makeRequest(http.MethodGet, path, nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Failed to list change requests: %s\n", string(body))
			os.Exit(1)
		}
		if jsonOut {
			fmt.Println(string(body))
			return
		}

		var data struct {
			ChangeRequests []domain.ChangeRequest `json:"change_requests"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing change requests: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n🛡️  Flagura 4-Eyes Governance Change Requests")
		fmt.Println("─────────────────────────────────────────────")
		if len(data.ChangeRequests) == 0 {
			fmt.Println("No change requests found.")
			fmt.Println()
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tFLAG KEY\tENV\tAUTHOR\tSTATUS\tCREATED")
		fmt.Fprintln(w, "--\t--------\t---\t------\t------\t-------")
		for _, cr := range data.ChangeRequests {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				cr.ID, cr.FlagKey, cr.Environment, cr.AuthorEmail, cr.Status, cr.CreatedAt.Format("2006-01-02 15:04"))
		}
		_ = w.Flush()
		fmt.Println()

	case "approve", "reject":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura change-request %s <id> [--comments=\"...\"]\n", subCmd)
			os.Exit(1)
		}
		id := args[1]
		approved := subCmd == "approve"
		path := fmt.Sprintf("/api/v1/change-requests/%s/review", id)
		payload := map[string]interface{}{
			"approved": approved,
			"comments": comments,
		}
		resp, body, err := makeRequest(http.MethodPost, path, payload)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Review failed: %s\n", string(body))
			os.Exit(1)
		}
		if approved {
			fmt.Printf("✅ ChangeRequest '%s' has been APPROVED.\n", id)
		} else {
			fmt.Printf("❌ ChangeRequest '%s' has been REJECTED.\n", id)
		}

	case "apply":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura change-request apply <id>\n")
			os.Exit(1)
		}
		id := args[1]
		path := fmt.Sprintf("/api/v1/change-requests/%s/apply", id)
		resp, body, err := makeRequest(http.MethodPost, path, nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Apply failed: %s\n", string(body))
			os.Exit(1)
		}
		fmt.Printf("🚀 ChangeRequest '%s' successfully APPLIED to live feature flag config!\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown change-request subcommand: %s. Use list, approve, reject, or apply.\n", subCmd)
		os.Exit(1)
	}
}

func runAPIKey(subCmd string, args []string, name, role string) {
	switch subCmd {
	case "list", "ls":
		resp, body, err := makeRequest(http.MethodGet, "/api/v1/api-keys", nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Failed to list API keys: %s\n", string(body))
			os.Exit(1)
		}
		if jsonOut {
			fmt.Println(string(body))
			return
		}

		var data struct {
			APIKeys []domain.APIKey `json:"api_keys"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing API keys response: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n🔑 Flagura Active API Keys & Service Tokens")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		if len(data.APIKeys) == 0 {
			fmt.Println("No active API keys found. Generate one using 'flagura api-key create --name=\"...\"'")
			fmt.Println()
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tROLE\tKEY PREFIX\tCREATED BY\tCREATED\tSTATUS")
		fmt.Fprintln(w, "--\t----\t----\t----------\t----------\t-------\t------")
		for _, k := range data.APIKeys {
			status := "🟢 Active"
			if k.Revoked {
				status = "🔴 Revoked"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				k.ID, k.Name, k.Role, k.KeyPrefix, k.CreatedBy, k.CreatedAt.Format("2006-01-02 15:04"), status)
		}
		_ = w.Flush()
		fmt.Println()

	case "create", "new", "add":
		payload := map[string]interface{}{
			"name": name,
			"role": role,
		}
		resp, body, err := makeRequest(http.MethodPost, "/api/v1/api-keys", payload)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Failed to create API key: %s\n", string(body))
			os.Exit(1)
		}
		if jsonOut {
			fmt.Println(string(body))
			return
		}

		var data struct {
			APIKey  domain.APIKey `json:"api_key"`
			Message string        `json:"message"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n✨ Flagura API Key Generated Successfully!")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		fmt.Printf("ID:         %s\n", data.APIKey.ID)
		fmt.Printf("Name:       %s\n", data.APIKey.Name)
		fmt.Printf("Role:       %s\n", data.APIKey.Role)
		fmt.Printf("Created At: %s\n\n", data.APIKey.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
		fmt.Println("🔑 Secret API Key Token (STORE THIS NOW - IT WILL NOT BE SHOWN AGAIN):")
		fmt.Printf("   \033[32m%s\033[0m\n\n", data.APIKey.Key)
		fmt.Println("Usage Example:")
		fmt.Printf("   export FLAGURA_API_KEY=\"%s\"\n", data.APIKey.Key)
		fmt.Println("   flagura list")
		fmt.Println()

	case "revoke", "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura api-key revoke <key-id>\n")
			os.Exit(1)
		}
		id := args[1]
		path := fmt.Sprintf("/api/v1/api-keys/%s", id)
		resp, body, err := makeRequest(http.MethodDelete, path, nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Revoke failed: %s\n", string(body))
			os.Exit(1)
		}
		fmt.Printf("✅ API Key '%s' has been permanently REVOKED across all edge nodes.\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown api-key subcommand: %s. Use list, create, or revoke.\n", subCmd)
		os.Exit(1)
	}
}

