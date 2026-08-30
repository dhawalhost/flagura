package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

var (
	version  = "v1.2.0"
	endpoint string
	apiKey   string
	env      string
	jsonOut  bool
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

	cmd := os.Args[1]

	// Global Flags
	fs := flag.NewFlagSet("flagura", flag.ExitOnError)
	fs.StringVar(&endpoint, "endpoint", defaultEndpoint, "Flagura control plane URL")
	fs.StringVar(&apiKey, "api-key", defaultApiKey, "API key for authentication")
	fs.StringVar(&env, "env", "production", "Target environment (production, staging, development)")
	fs.BoolVar(&jsonOut, "json", false, "Output results as formatted JSON")

	switch cmd {
	case "version", "-v", "--version":
		fmt.Printf("Flagura CLI %s\n", version)
	case "list", "ls":
		_ = fs.Parse(os.Args[2:])
		runList()
	case "get":
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura get <flag-key>\n")
			os.Exit(1)
		}
		runGet(fs.Arg(0))
	case "toggle":
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura toggle <flag-key> [--env=production]\n")
			os.Exit(1)
		}
		runToggle(fs.Arg(0))
	case "rollout":
		_ = fs.Parse(os.Args[2:])
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
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura evaluate <flag-key> [--user=<id>] [--email=<email>] [--trace]\n")
			os.Exit(1)
		}
		runEvaluate(fs.Arg(0), *userFlag, *emailFlag, *traceFlag)
	case "promote":
		fromFlag := fs.String("from", "staging", "Source environment")
		toFlag := fs.String("to", "production", "Destination environment")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura promote <flag-key> [--from=staging] [--to=production]\n")
			os.Exit(1)
		}
		runPromote(fs.Arg(0), *fromFlag, *toFlag)
	case "clean-up", "cleanup":
		_ = fs.Parse(os.Args[2:])
		runCleanup()
	case "health":
		_ = fs.Parse(os.Args[2:])
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
  clean-up                  Scan and report technical debt & stale flags
  health                    Check connection to the Flagura control plane
  version                   Print CLI version
  help                      Show this help message

Flags:
  --endpoint <url>          Flagura control plane URL (default: $FLAGURA_ENDPOINT or http://localhost:3000)
  --api-key <key>           API key for authentication (default: $FLAGURA_API_KEY)
  --env <name>              Target environment: production, staging, development (default: production)
  --json                    Output results as raw formatted JSON

Examples:
  flagura list
  flagura toggle ai-smart-search --env=production
  flagura rollout ai-smart-search 50%% --env=production
  flagura evaluate ai-smart-search --user=usr_alex_42 --trace
  flagura promote ai-smart-search --from=staging --to=production
  flagura clean-up
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
	w.Flush()
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
	_ = json.Unmarshal(body, &data)

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
	resp, body, err := makeRequest(http.MethodPatch, fmt.Sprintf("/api/v1/flags/%s/rollout?percentage=%g&env=%s", key, pct, env), nil)
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

func runCleanup() {
	_, body, err := makeRequest(http.MethodGet, "/api/v1/flags", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var data struct {
		Flags []domain.FeatureFlag `json:"flags"`
	}
	_ = json.Unmarshal(body, &data)

	staleCount := 0
	fmt.Println("\n🧹 Flagura Technical Debt & Stale Flag Report")
	fmt.Println("─────────────────────────────────────────────")

	for _, f := range data.Flags {
		report := domain.AnalyzeFlagHealth(f)
		if report.IsStale {
			staleCount++
			icon := "⚠️"
			if report.Status == domain.HealthStatusStale {
				icon = "✨"
			}
			fmt.Printf("\n%s Flag: %s (%s)\n", icon, f.Key, report.Status)
			fmt.Printf("  Reason: %s\n", report.Reason)
			fmt.Printf("  Action: %s\n", report.SuggestedAction)
		}
	}

	if staleCount == 0 {
		fmt.Println("\n✅ No technical debt detected! All feature flags are actively routing traffic.")
	} else {
		fmt.Printf("\nFound %d flag(s) ready for code cleanup.\n", staleCount)
	}
	fmt.Println()
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
