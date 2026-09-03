package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

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
