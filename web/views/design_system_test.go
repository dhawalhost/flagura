package views

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

var bannedPatterns = map[string]*regexp.Regexp{
	"gradient":          regexp.MustCompile(`bg-gradient-|linear-gradient`),
	"backdrop blur":     regexp.MustCompile(`backdrop-blur`),
	"looping animation": regexp.MustCompile(`animate-(pulse|ping|float|pulse-slow)\b`),
	"glass classes":     regexp.MustCompile(`glass-(panel|card)|glow-border|shimmer-text|laser-grid`),
	"oversized radius":  regexp.MustCompile(`rounded-(2xl|3xl)\b`),
	"arbitrary size":    regexp.MustCompile(`text-\[\d+px\]`),
	"raw palette":       regexp.MustCompile(`\b(bg|text|border|from|to|via|ring|divide|placeholder)-(slate|gray|zinc|indigo|purple|violet|emerald|green|red|rose|amber|yellow|orange|sky|cyan|blue|teal)-\d`),
	"emoji icon":        regexp.MustCompile(`[\x{2728}\x{26A1}\x{27A1}\x{2794}\x{2600}-\x{26FF}\x{1F300}-\x{1FAFF}]`),
}

func TestNoDecorativePatterns(t *testing.T) {
	templs, err := filepath.Glob("*.templ")
	if err != nil {
		t.Fatal(err)
	}
	files := append(templs, "../static/app.css", "../static/js/app.js")
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for name, pattern := range bannedPatterns {
			if loc := pattern.Find(src); loc != nil {
				t.Errorf("%s: contains %s: %q", file, name, loc)
			}
		}
	}
}

func renderToString(t *testing.T, render func(*bytes.Buffer) error) string {
	t.Helper()
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestLayoutThemeAndScripts(t *testing.T) {
	out := renderToString(t, func(b *bytes.Buffer) error {
		return Layout("Test").Render(context.Background(), b)
	})
	for _, want := range []string{"flagura-theme", "data-theme", "prefers-color-scheme"} {
		if !strings.Contains(out, want) {
			t.Errorf("layout missing %q", want)
		}
	}
	for _, banned := range []string{"three.min.js", "gsap.min.js", "ScrollTrigger"} {
		if strings.Contains(out, banned) {
			t.Errorf("layout still loads %q", banned)
		}
	}
}

func TestOverviewAndAnalyticsShowNoInventedFigures(t *testing.T) {
	flags := []domain.FeatureFlag{
		{Key: "on-full", Name: "On", Type: "boolean", Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean, Percentage: 100},
		}},
		{Key: "on-canary", Name: "Canary", Type: "boolean", Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 25},
		}},
		{Key: "off", Name: "Off", Type: "boolean"},
	}
	audits := []domain.AuditLogEntry{{FlagKey: "on-full", Action: "TOGGLE", Actor: "a@example.test", Timestamp: time.Now()}}

	overview := renderToString(t, func(b *bytes.Buffer) error {
		return BentoOverview(flags, audits, "Memory").Render(context.Background(), b)
	})
	for _, banned := range []string{"85ns", "100% Sync", "circuits live", "(Active)"} {
		if strings.Contains(overview, banned) {
			t.Errorf("overview contains invented copy %q", banned)
		}
	}
	for _, want := range []string{`data-metric="enabled">2<`, `data-metric="partial">1<`, `data-metric="total">3<`} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview missing computed metric %q", want)
		}
	}

	analytics := renderToString(t, func(b *bytes.Buffer) error {
		return AnalyticsDashboard(flags).Render(context.Background(), b)
	})
	for _, banned := range []string{"2.8M", "2,841,920", "99.98%", "482,100", "~85", "+18.4%"} {
		if strings.Contains(analytics, banned) {
			t.Errorf("analytics contains invented figure %q", banned)
		}
	}
}

func TestDynamicIconsAreSafeToHydrate(t *testing.T) {
	// Lucide replaces <i> with <svg>, which strips Alpine bindings from the node.
	boundIcon := regexp.MustCompile(`<i\s[^\n]*?(x-show|:class)="[^\n]*?</i>`)
	templs, err := filepath.Glob("*.templ")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range templs {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if match := boundIcon.Find(src); match != nil {
			t.Errorf("%s: icon carries an Alpine binding: %q", file, match)
		}
		if bytes.Contains(src, []byte("flagura-logo.png")) {
			t.Errorf("%s: still uses the raster logo", file)
		}
	}
}

func TestToolScreensShowNoInventedFigures(t *testing.T) {
	benchmark := renderToString(t, func(b *bytes.Buffer) error {
		return BenchmarkModal().Render(context.Background(), b)
	})
	for _, banned := range []string{"~85", "11,700,000", "261.0", "397.8", "1,304.0"} {
		if strings.Contains(benchmark, banned) {
			t.Errorf("benchmark contains invented figure %q", banned)
		}
	}
	evaluator := renderToString(t, func(b *bytes.Buffer) error {
		return LiveEvaluator(nil).Render(context.Background(), b)
	})
	if strings.Contains(evaluator, "0.05ms") {
		t.Error("evaluator contains invented batch timing")
	}
}

func TestLogoMarkIsFlatAndThemed(t *testing.T) {
	out := renderToString(t, func(b *bytes.Buffer) error {
		return LogoMark("h-7 w-7").Render(context.Background(), b)
	})
	for _, want := range []string{"fill-neutral-900", "fill-surface", "logo-leaf", "h-7 w-7"} {
		if !strings.Contains(out, want) {
			t.Errorf("logo mark missing %q", want)
		}
	}
	if strings.Contains(out, "Gradient") {
		t.Error("logo mark must be flat")
	}
}

func TestLogoFilesAreOutlinedVectors(t *testing.T) {
	for _, name := range []string{"flagura-mark", "flagura-icon-light", "flagura-icon-dark", "flagura-logo-light", "flagura-logo-dark", "flagura-symbol-light", "flagura-symbol-dark", "flagura-logo-plain-light", "flagura-logo-plain-dark"} {
		src, err := os.ReadFile(filepath.Join("../static/img", name+".svg"))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"<text", "<image", "Gradient", "base64"} {
			if bytes.Contains(src, []byte(banned)) {
				t.Errorf("%s contains %q", name, banned)
			}
		}
	}
}

func TestLogoWordmarkUsesTokens(t *testing.T) {
	out := renderToString(t, func(b *bytes.Buffer) error {
		return LogoWordmark("h-5").Render(context.Background(), b)
	})
	for _, want := range []string{"fill-neutral-900", `aria-label="Flagura"`} {
		if !strings.Contains(out, want) {
			t.Errorf("wordmark missing %q", want)
		}
	}
}
