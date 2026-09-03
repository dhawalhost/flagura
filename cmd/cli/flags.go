package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/dhawalhost/flagura/pkg/domain"
)

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
			FinalReason  domain.EvaluationReason `json:"final_reason"`
			FinalVariant string                  `json:"final_variant"`
			FinalEnabled bool                    `json:"final_enabled"`
			Bucket       float64                 `json:"bucket"`
			ElapsedNs    int64                   `json:"elapsed_ns"`
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
