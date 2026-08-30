package domain

import (
	"time"
)

// EventType categorizes experiment metrics.
type EventType string

const (
	EventTypeConversion EventType = "conversion" // Binary metric (0 or 1, e.g. click, signup, checkout)
	EventTypeContinuous EventType = "continuous" // Numeric metric (e.g. order value $, latency ms)
)

// ExperimentEvent represents a metric observation tagged with a feature flag variant.
type ExperimentEvent struct {
	ID          string      `json:"id"`
	FlagKey     string      `json:"flag_key"`
	Variant     string      `json:"variant"`
	MetricName  string      `json:"metric_name"`
	EventType   EventType   `json:"event_type"`
	Value       float64     `json:"value"`
	UserID      string      `json:"user_id"`
	Environment Environment `json:"environment"`
	Timestamp   time.Time   `json:"timestamp"`
}

// VariantMetricStats contains raw aggregated metrics for a single variant.
type VariantMetricStats struct {
	Variant        string  `json:"variant"`
	Exposures      int64   `json:"exposures"`       // Total evaluations/impressions (N)
	Conversions    int64   `json:"conversions"`     // Total conversion successes (K)
	ConversionRate float64 `json:"conversion_rate"` // K / N
	SumValue       float64 `json:"sum_value"`       // Sum of continuous values
	Mean           float64 `json:"mean"`            // Average value per exposure
	Variance       float64 `json:"variance"`        // Sample variance
	StandardError  float64 `json:"standard_error"`  // SE = sqrt(p(1-p)/N) or s/sqrt(N)
	CI95Lower      float64 `json:"ci95_lower"`      // 95% Confidence Interval lower bound
	CI95Upper      float64 `json:"ci95_upper"`      // 95% Confidence Interval upper bound
	CI99Lower      float64 `json:"ci99_lower"`      // 99% Confidence Interval lower bound
	CI99Upper      float64 `json:"ci99_upper"`      // 99% Confidence Interval upper bound
}

// ExperimentStatus describes statistical conclusion of an A/B test.
type ExperimentStatus string

const (
	ExpStatusInsufficientData ExperimentStatus = "INSUFFICIENT_DATA"
	ExpStatusInconclusive     ExperimentStatus = "INCONCLUSIVE"
	ExpStatusWinning          ExperimentStatus = "WINNING"
	ExpStatusLosing           ExperimentStatus = "LOSING"
)

// VariantComparison provides the statistical test results between Treatment and Control.
type VariantComparison struct {
	TreatmentVariant   string           `json:"treatment_variant"`
	ControlVariant     string           `json:"control_variant"`
	AbsoluteLift       float64          `json:"absolute_lift"`        // p_T - p_C
	RelativeLiftPct    float64          `json:"relative_lift_pct"`    // ((p_T - p_C) / p_C) * 100
	ZScore             float64          `json:"z_score"`              // Z-statistic
	PValue             float64          `json:"p_value"`              // Two-tailed p-value
	ConfidencePct      float64          `json:"confidence_pct"`       // (1 - p_value) * 100%
	IsSignificant95    bool             `json:"is_significant_95"`    // True if p < 0.05
	IsSignificant99    bool             `json:"is_significant_99"`    // True if p < 0.01
	Status             ExperimentStatus `json:"status"`               // WINNING, LOSING, INCONCLUSIVE, etc.
	RecommendedAction  string           `json:"recommended_action"`   // Human guidance
	RequiredSampleSize int64            `json:"required_sample_size"` // Estimated N for 80% power at 5% alpha
}

// ExperimentReport is the complete analytical report for a flag's experiment.
type ExperimentReport struct {
	FlagKey        string                        `json:"flag_key"`
	MetricName     string                        `json:"metric_name"`
	EventType      EventType                     `json:"event_type"`
	Environment    Environment                   `json:"environment"`
	ControlVariant string                        `json:"control_variant"`
	TotalExposures int64                         `json:"total_exposures"`
	TotalEvents    int64                         `json:"total_events"`
	VariantStats   map[string]VariantMetricStats `json:"variant_stats"`
	Comparisons    map[string]VariantComparison  `json:"comparisons"` // key = treatment variant
	WinnerVariant  string                        `json:"winner_variant,omitempty"`
	GeneratedAt    time.Time                     `json:"generated_at"`
}
