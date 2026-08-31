package stats

import (
	"math"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestTwoTailedPValue(t *testing.T) {
	tests := []struct {
		name      string
		z         float64
		expectedP float64
		tolerance float64
	}{
		{name: "Z = 0.0 (No difference)", z: 0.0, expectedP: 1.0, tolerance: 0.001},
		{name: "Z = 1.96 (p = 0.05 threshold)", z: 1.96, expectedP: 0.05, tolerance: 0.002},
		{name: "Z = 2.576 (p = 0.01 threshold)", z: 2.576, expectedP: 0.01, tolerance: 0.002},
		{name: "Z = 3.291 (p = 0.001 threshold)", z: 3.291, expectedP: 0.001, tolerance: 0.0005},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := TwoTailedPValue(tt.z)
			if math.Abs(p-tt.expectedP) > tt.tolerance {
				t.Errorf("TwoTailedPValue(%f) = %f; expected %f (tolerance %f)", tt.z, p, tt.expectedP, tt.tolerance)
			}
		})
	}
}

func TestComputeVariantBinaryStats(t *testing.T) {
	tests := []struct {
		name         string
		variant      string
		exposures    int64
		conversions  int64
		expectedRate float64
	}{
		{
			name:         "10% Conversion Rate (100/1000)",
			variant:      "control",
			exposures:    1000,
			conversions:  100,
			expectedRate: 0.10,
		},
		{
			name:         "25% Conversion Rate (250/1000)",
			variant:      "treatment",
			exposures:    1000,
			conversions:  250,
			expectedRate: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := ComputeVariantBinaryStats(tt.variant, tt.exposures, tt.conversions)
			if math.Abs(stats.ConversionRate-tt.expectedRate) > 0.0001 {
				t.Fatalf("expected conversion rate %f, got %f", tt.expectedRate, stats.ConversionRate)
			}
			if stats.CI95Lower >= tt.expectedRate || stats.CI95Upper <= tt.expectedRate {
				t.Errorf("CI95 [%f, %f] does not bracket %f", stats.CI95Lower, stats.CI95Upper, tt.expectedRate)
			}
		})
	}
}

func TestCompareBinaryVariants(t *testing.T) {
	tests := []struct {
		name              string
		controlExposures  int64
		controlConvs      int64
		treatmentExpos    int64
		treatmentConvs    int64
		expectedStatus    domain.ExperimentStatus
		expectSignificant bool
	}{
		{
			name:              "Statistically significant winner (+50% relative lift)",
			controlExposures:  1000,
			controlConvs:      100,
			treatmentExpos:    1000,
			treatmentConvs:    150,
			expectedStatus:    domain.ExpStatusWinning,
			expectSignificant: true,
		},
		{
			name:              "Insufficient sample size (N=10)",
			controlExposures:  10,
			controlConvs:      1,
			treatmentExpos:    10,
			treatmentConvs:    2,
			expectedStatus:    domain.ExpStatusInsufficientData,
			expectSignificant: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			control := ComputeVariantBinaryStats("control", tt.controlExposures, tt.controlConvs)
			treatment := ComputeVariantBinaryStats("treatment", tt.treatmentExpos, tt.treatmentConvs)

			comp := CompareBinaryVariants(control, treatment)
			if comp.Status != tt.expectedStatus {
				t.Errorf("CompareBinaryVariants() status = %s, expected %s", comp.Status, tt.expectedStatus)
			}
			if comp.IsSignificant95 != tt.expectSignificant {
				t.Errorf("CompareBinaryVariants() isSignificant95 = %v, expected %v", comp.IsSignificant95, tt.expectSignificant)
			}
		})
	}
}

func TestAnalyzeExperiment(t *testing.T) {
	exposures := map[string]int64{
		"control":   500,
		"treatment": 500,
	}

	var events []domain.ExperimentEvent
	for i := 0; i < 50; i++ {
		events = append(events, domain.ExperimentEvent{
			FlagKey:    "checkout-button",
			MetricName: "signup",
			Variant:    "control",
			Value:      1.0,
			Timestamp:  time.Now(),
		})
	}
	for i := 0; i < 80; i++ {
		events = append(events, domain.ExperimentEvent{
			FlagKey:    "checkout-button",
			MetricName: "signup",
			Variant:    "treatment",
			Value:      1.0,
			Timestamp:  time.Now(),
		})
	}

	tests := []struct {
		name           string
		flagKey        string
		metricName     string
		expectedWinner string
		expectedEvents int64
	}{
		{
			name:           "Analyze experiment with winning treatment",
			flagKey:        "checkout-button",
			metricName:     "signup",
			expectedWinner: "treatment",
			expectedEvents: 130,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := AnalyzeExperiment(tt.flagKey, tt.metricName, domain.EventTypeConversion, domain.EnvProduction, "control", exposures, events)
			if report.WinnerVariant != tt.expectedWinner {
				t.Errorf("AnalyzeExperiment() winner = %q, expected %q", report.WinnerVariant, tt.expectedWinner)
			}
			if report.TotalEvents != tt.expectedEvents {
				t.Errorf("AnalyzeExperiment() totalEvents = %d, expected %d", report.TotalEvents, tt.expectedEvents)
			}
		})
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
