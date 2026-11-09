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
	"gradient":          regexp.MustCompile(`bg-gradient-|linear-gradient\([^v]`),
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
		return BentoOverview(flags, audits, nil, "Memory").Render(context.Background(), b)
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

func TestLandingUsesLiveRolloutAndVerifiableClaims(t *testing.T) {
	out := renderToString(t, func(b *bytes.Buffer) error {
		return LandingPage().Render(context.Background(), b)
	})
	for _, want := range []string{`x-data="rolloutField()"`, `id="section-bento"`, `id="section-benchmark"`, "installCliComponent()", "-bench . -benchmem"} {
		if !strings.Contains(out, want) {
			t.Errorf("landing missing %q", want)
		}
	}
	for _, banned := range []string{"Zero Egress", "0 heap", "15MB", "10ms", "1.5%", "~85", "SOC2", "Sub-microsecond"} {
		if strings.Contains(out, banned) {
			t.Errorf("landing contains unverified claim %q", banned)
		}
	}
}

func TestConsoleRegressionsStayFixed(t *testing.T) {
	for file, banned := range map[string]string{"header.templ": "prevent.slash", "profile_settings.templ": "; if ("} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(src, []byte(banned)) {
			t.Errorf("%s still contains %q", file, banned)
		}
	}
	for _, file := range []string{"experiment_modal.templ", "hygiene_modal.templ", "governance_modal.templ", "project_modal.templ", "apikeys.templ"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`role="dialog"`, "keydown.escape.window"} {
			if !bytes.Contains(src, []byte(want)) {
				t.Errorf("%s missing %s", file, want)
			}
		}
	}
}

func TestOverviewLeadsWithWorkNeedingAttention(t *testing.T) {
	flags := []domain.FeatureFlag{{Key: "on-canary", Name: "Canary", Type: "boolean", Environments: map[domain.Environment]domain.EnvironmentConfig{
		domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 25},
	}}}
	requests := []domain.ChangeRequest{{ID: "cr_1", FlagKey: "on-canary", Title: "Ramp canary", Status: domain.ChangeRequestStatusPending}}
	out := renderToString(t, func(b *bytes.Buffer) error {
		return BentoOverview(flags, nil, requests, "Memory").Render(context.Background(), b)
	})
	for _, want := range []string{"Needs you", "Ramp canary", "rollbar", ">Full<"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview missing %q", want)
		}
	}
}

func TestProductionToggleConfirmationIsRendered(t *testing.T) {
	out := renderToString(t, func(b *bytes.Buffer) error {
		return ToggleConfirm().Render(context.Background(), b)
	})
	if !strings.Contains(out, `role="dialog"`) || !strings.Contains(out, "confirmToggleNow") {
		t.Error("toggle confirmation dialog is incomplete")
	}
}

func TestAPIKeysLiveOutsideSDKDocs(t *testing.T) {
	sdk := renderToString(t, func(b *bytes.Buffer) error {
		return SDKModal().Render(context.Background(), b)
	})
	for _, banned := range []string{"showKeyModal", "fetchAPIKeys", "sdkTab === 'apikeys'"} {
		if strings.Contains(sdk, banned) {
			t.Errorf("SDK docs still contain %q", banned)
		}
	}
	if !strings.Contains(sdk, "createKey()") {
		t.Error("SDK docs do not link to creating a key")
	}
	keys := renderToString(t, func(b *bytes.Buffer) error {
		return APIKeysPage().Render(context.Background(), b)
	})
	for _, want := range []string{"activeView === 'apikeys'", "fetchAPIKeys", "showKeyModal", "newlyCreatedKey", "revokeAPIKey"} {
		if !strings.Contains(keys, want) {
			t.Errorf("API keys page missing %q", want)
		}
	}
}

func TestNoBrowserDrawnControls(t *testing.T) {
	js, err := os.ReadFile("../static/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if match := regexp.MustCompile(`\b(alert|confirm|prompt)\(`).Find(js); match != nil {
		t.Errorf("app.js still calls browser popup %q", match)
	}
	for _, want := range []string{"installCustomSelects", "installValidationMessages", "installTooltips"} {
		if !bytes.Contains(js, []byte(want)) {
			t.Errorf("app.js missing %s", want)
		}
	}
	css, err := os.ReadFile("../static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".select-pop", `input[type="checkbox"]:checked`, "::-webkit-slider-thumb", ".field-error", ".tip"} {
		if !bytes.Contains(css, []byte(want)) {
			t.Errorf("app.css missing %s", want)
		}
	}
}

func TestSwitchComponent(t *testing.T) {
	out := renderToString(t, func(b *bytes.Buffer) error {
		return Switch("demo in production", "envOn", "envPct", "false", "toggleCurrent").Render(context.Background(), b)
	})
	for _, want := range []string{`role="switch"`, `aria-label="demo in production"`, "switch-thumb", "--pct"} {
		if !strings.Contains(out, want) {
			t.Errorf("switch missing %q", want)
		}
	}
	templs, _ := filepath.Glob("*.templ")
	for _, file := range templs {
		src, _ := os.ReadFile(file)
		if bytes.Contains(src, []byte("toggle-hit")) {
			t.Errorf("%s still hand-rolls a toggle", file)
		}
	}
}

func TestSharedComponentsReplaceInlineStyles(t *testing.T) {
	templs, _ := filepath.Glob("*.templ")
	primary := regexp.MustCompile(`<button[^>]*bg-neutral-900 hover:bg-neutral-700`)
	input := regexp.MustCompile(`<input[^>]*class="w-full bg-surface border border-neutral-200`)
	for _, file := range templs {
		if file == "landing.templ" {
			continue
		}
		src, _ := os.ReadFile(file)
		if match := primary.Find(src); match != nil {
			t.Errorf("%s has inline primary button", file)
		}
		if match := input.Find(src); match != nil {
			t.Errorf("%s has inline input", file)
		}
	}
	out := renderToString(t, func(b *bytes.Buffer) error {
		return BentoOverview([]domain.FeatureFlag{{Key: "a", Name: "A", Type: "boolean"}}, nil, nil, "Memory").Render(context.Background(), b)
	})
	if !strings.Contains(out, "leaf-strip") {
		t.Error("overview is missing the leaf strip")
	}
	js, _ := os.ReadFile("../static/js/app.js")
	for _, dead := range []string{"selectedWallpaper", "handleTabClick", "Enterprise Component"} {
		if bytes.Contains(js, []byte(dead)) {
			t.Errorf("app.js still contains %q", dead)
		}
	}
}

func TestPolishPassRules(t *testing.T) {
	css, _ := os.ReadFile("../static/app.css")
	for _, banned := range []string{"1.4, 0.5", "view-in", "transition: width", "transition: transform 0.2s cubic-bezier(0.3"} {
		if bytes.Contains(css, []byte(banned)) {
			t.Errorf("app.css still contains %q", banned)
		}
	}
	templs, _ := filepath.Glob("*.templ")
	offGrid := regexp.MustCompile(`\b(gap|gap-x|gap-y|space-y|space-x|mt|mb)-(1\.5|2\.5|3\.5)\b`)
	for _, file := range templs {
		src, _ := os.ReadFile(file)
		if match := offGrid.Find(src); match != nil {
			t.Errorf("%s has off-grid spacing %q", file, match)
		}
		for _, banned := range []string{"transition-all", "Copied!", "CONVERSION RATE", `>•<`} {
			if bytes.Contains(src, []byte(banned)) {
				t.Errorf("%s still contains %q", file, banned)
			}
		}
	}
}

func TestBenchmarkIsReachedFromEvaluator(t *testing.T) {
	sidebar, _ := os.ReadFile("sidebar.templ")
	if bytes.Contains(sidebar, []byte("navigateTo('benchmark')")) {
		t.Error("benchmark should not be a sidebar item")
	}
	evaluator := renderToString(t, func(b *bytes.Buffer) error {
		return LiveEvaluator(nil).Render(context.Background(), b)
	})
	if !strings.Contains(evaluator, "navigateTo('benchmark')") {
		t.Error("evaluator does not link to the benchmark")
	}
}

func TestPremiumLibraryPrimitives(t *testing.T) {
	css, _ := os.ReadFile("../static/app.css")
	for _, want := range []string{"--shell:", "--hairline:", "--shadow-card:", ".badge-ok", ".nav-active", ".app-sheet"} {
		if !bytes.Contains(css, []byte(want)) {
			t.Errorf("app.css missing %s", want)
		}
	}
	templs, _ := filepath.Glob("*.templ")
	inline := regexp.MustCompile(`class="[^"]*\bpx-2 py-0\.5 rounded[^"]*\bborder-(ok|warn|danger|accent)-200`)
	for _, file := range templs {
		src, _ := os.ReadFile(file)
		if match := inline.Find(src); match != nil {
			t.Errorf("%s has an inline badge", file)
		}
	}
}
