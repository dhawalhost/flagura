package stats

import (
	"math"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestNormalCDFAndPValues(t *testing.T) {
	// Standard golden reference tests
	tests := []struct {
		z         float64
		expectedP float64
		tolerance float64
	}{
		{z: 0.0, expectedP: 1.0, tolerance: 0.001},
		{z: 1.96, expectedP: 0.05, tolerance: 0.002},
		{z: 2.576, expectedP: 0.01, tolerance: 0.002},
		{z: 3.291, expectedP: 0.001, tolerance: 0.0005},
	}

	for _, tt := range tests {
		p := TwoTailedPValue(tt.z)
		if math.Abs(p-tt.expectedP) > tt.tolerance {
			t.Errorf("TwoTailedPValue(%f) = %f; expected %f (tolerance %f)", tt.z, p, tt.expectedP, tt.tolerance)
		}
	}
}

func TestBinaryVariantStats(t *testing.T) {
	// 1000 exposures, 100 conversions (10% conversion rate)
	stats := ComputeVariantBinaryStats("control", 1000, 100)

	if stats.ConversionRate != 0.10 {
		t.Fatalf("expected conversion rate 0.10, got %f", stats.ConversionRate)
	}

	// SE = sqrt(0.10 * 0.90 / 1000) = sqrt(0.00009) ≈ 0.0094868
	expectedSE := math.Sqrt(0.09 / 1000.0)
	if math.Abs(stats.StandardError-expectedSE) > 0.0001 {
		t.Errorf("expected SE ≈ %f, got %f", expectedSE, stats.StandardError)
	}

	// 95% CI should bracket 10%
	if stats.CI95Lower >= 0.10 || stats.CI95Upper <= 0.10 {
		t.Errorf("CI95 [%f, %f] does not bracket 0.10", stats.CI95Lower, stats.CI95Upper)
	}
}

func TestAAndBVariantComparisonWinning(t *testing.T) {
	// Control: 1,000 exposures, 100 conversions (10%)
	control := ComputeVariantBinaryStats("control", 1000, 100)
	// Treatment: 1,000 exposures, 150 conversions (15% - a 50% relative increase!)
	treatment := ComputeVariantBinaryStats("treatment", 1000, 150)

	comp := CompareBinaryVariants(control, treatment)

	if comp.Status != domain.ExpStatusWinning {
		t.Fatalf("expected status %s, got %s", domain.ExpStatusWinning, comp.Status)
	}
	if !comp.IsSignificant95 {
		t.Fatalf("expected statistically significant at 95%% level")
	}
	if math.Abs(comp.RelativeLiftPct-50.0) > 0.001 {
		t.Fatalf("expected +50%% relative lift, got %f", comp.RelativeLiftPct)
	}
	if comp.ConfidencePct < 99.0 {
		t.Fatalf("expected >99%% confidence, got %f", comp.ConfidencePct)
	}
}

func TestAAndBVariantComparisonInsufficientData(t *testing.T) {
	// Small sample size (N=10)
	control := ComputeVariantBinaryStats("control", 10, 1)
	treatment := ComputeVariantBinaryStats("treatment", 10, 2)

	comp := CompareBinaryVariants(control, treatment)
	if comp.Status != domain.ExpStatusInsufficientData {
		t.Fatalf("expected status %s for small sample, got %s", domain.ExpStatusInsufficientData, comp.Status)
	}
}

func TestAnalyzeExperimentFullReport(t *testing.T) {
	exposures := map[string]int64{
		"control":   500,
		"treatment": 500,
	}

	var events []domain.ExperimentEvent
	// 50 conversions for control (10%)
	for i := 0; i < 50; i++ {
		events = append(events, domain.ExperimentEvent{
			FlagKey:    "checkout-button",
			MetricName: "signup",
			Variant:    "control",
			Value:      1.0,
			Timestamp:  time.Now(),
		})
	}
	// 80 conversions for treatment (16%)
	for i := 0; i < 80; i++ {
		events = append(events, domain.ExperimentEvent{
			FlagKey:    "checkout-button",
			MetricName: "signup",
			Variant:    "treatment",
			Value:      1.0,
			Timestamp:  time.Now(),
		})
	}

	report := AnalyzeExperiment("checkout-button", "signup", domain.EventTypeConversion, domain.EnvProduction, "control", exposures, events)

	if report.WinnerVariant != "treatment" {
		t.Fatalf("expected winner 'treatment', got %q", report.WinnerVariant)
	}

	if report.TotalEvents != 130 {
		t.Fatalf("expected total events 130, got %d", report.TotalEvents)
	}

	comp := report.Comparisons["treatment"]
	if comp.Status != domain.ExpStatusWinning {
		t.Fatalf("expected winning status, got %s", comp.Status)
	}
}

func BenchmarkAnalyzeExperiment(b *testing.B) {
	exposures := map[string]int64{"control": 10000, "treatment": 10000}
	var events []domain.ExperimentEvent
	for i := 0; i < 1000; i++ {
		events = append(events, domain.ExperimentEvent{
			FlagKey:    "bench-flag",
			MetricName: "conversion",
			Variant:    "control",
			Value:      1.0,
		})
		events = append(events, domain.ExperimentEvent{
			FlagKey:    "bench-flag",
			MetricName: "conversion",
			Variant:    "treatment",
			Value:      1.0,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AnalyzeExperiment("bench-flag", "conversion", domain.EventTypeConversion, domain.EnvProduction, "control", exposures, events)
	}
}
