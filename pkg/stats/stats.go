package stats

import (
	"fmt"
	"math"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// MinSampleSizeForSignificance defines the minimum exposures per variant before calculating statistical significance.
const MinSampleSizeForSignificance = 30

// NormalCDF computes the cumulative distribution function of the standard normal distribution using math.Erf.
func NormalCDF(z float64) float64 {
	return 0.5 * (1.0 + math.Erf(z/math.Sqrt2))
}

// TwoTailedPValue computes the two-tailed p-value from a Z-score.
func TwoTailedPValue(z float64) float64 {
	absZ := math.Abs(z)
	cdf := NormalCDF(absZ)
	pValue := 2.0 * (1.0 - cdf)
	if pValue < 0 {
		return 0
	}
	if pValue > 1 {
		return 1
	}
	return pValue
}

// ComputeVariantBinaryStats computes conversion rates, standard error, and confidence intervals for binary metrics.
func ComputeVariantBinaryStats(variant string, exposures, conversions int64) domain.VariantMetricStats {
	stats := domain.VariantMetricStats{
		Variant:     variant,
		Exposures:   exposures,
		Conversions: conversions,
	}

	if exposures <= 0 {
		return stats
	}

	p := float64(conversions) / float64(exposures)
	stats.ConversionRate = p
	stats.Mean = p

	// Variance of Bernoulli distribution is p * (1 - p)
	variance := p * (1.0 - p)
	stats.Variance = variance

	// Standard error SE = sqrt(p(1-p)/N)
	se := math.Sqrt(variance / float64(exposures))
	stats.StandardError = se

	// 95% Confidence Interval (Z = 1.96)
	stats.CI95Lower = math.Max(0.0, p-1.96*se)
	stats.CI95Upper = math.Min(1.0, p+1.96*se)

	// 99% Confidence Interval (Z = 2.576)
	stats.CI99Lower = math.Max(0.0, p-2.576*se)
	stats.CI99Upper = math.Min(1.0, p+2.576*se)

	return stats
}

// CompareBinaryVariants performs a two-proportion pooled Z-test between Treatment and Control.
func CompareBinaryVariants(control, treatment domain.VariantMetricStats) domain.VariantComparison {
	comp := domain.VariantComparison{
		TreatmentVariant: treatment.Variant,
		ControlVariant:   control.Variant,
	}

	// Calculate sample sizes and conversion rates
	nC := float64(control.Exposures)
	nT := float64(treatment.Exposures)
	pC := control.ConversionRate
	pT := treatment.ConversionRate

	comp.AbsoluteLift = pT - pC
	if pC > 0 {
		comp.RelativeLiftPct = ((pT - pC) / pC) * 100.0
	}

	// Sample size guardrail
	if control.Exposures < MinSampleSizeForSignificance || treatment.Exposures < MinSampleSizeForSignificance {
		comp.Status = domain.ExpStatusInsufficientData
		comp.RecommendedAction = fmt.Sprintf("Collect more data (current sample: Control=%d, Treatment=%d; minimum required=%d per variant).",
			control.Exposures, treatment.Exposures, MinSampleSizeForSignificance)
		comp.RequiredSampleSize = MinSampleSizeForSignificance
		return comp
	}

	// Pooled proportion: p_pool = (k_C + k_T) / (n_C + n_T)
	totalConversions := float64(control.Conversions + treatment.Conversions)
	totalExposures := nC + nT
	pPool := totalConversions / totalExposures

	// Pooled standard error: SE_pool = sqrt(p_pool * (1 - p_pool) * (1/n_C + 1/n_T))
	sePool := math.Sqrt(pPool * (1.0 - pPool) * (1.0/nC + 1.0/nT))

	if sePool == 0 {
		comp.Status = domain.ExpStatusInconclusive
		comp.ConfidencePct = 0
		comp.RecommendedAction = "No variance detected between variants."
		return comp
	}

	zScore := (pT - pC) / sePool
	comp.ZScore = zScore

	pValue := TwoTailedPValue(zScore)
	comp.PValue = pValue
	comp.ConfidencePct = math.Max(0.0, (1.0-pValue)*100.0)

	comp.IsSignificant95 = pValue < 0.05
	comp.IsSignificant99 = pValue < 0.01

	// Determine status and recommendation
	if comp.IsSignificant95 {
		if comp.AbsoluteLift > 0 {
			comp.Status = domain.ExpStatusWinning
			comp.RecommendedAction = fmt.Sprintf("Treatment '%s' is outperforming Control by +%.2f%% (Statistically Significant with %.1f%% confidence, p=%.4f). Safe to roll out to 100%%.",
				treatment.Variant, comp.RelativeLiftPct, comp.ConfidencePct, comp.PValue)
		} else {
			comp.Status = domain.ExpStatusLosing
			comp.RecommendedAction = fmt.Sprintf("Treatment '%s' is underperforming Control by %.2f%% (Statistically Significant with %.1f%% confidence, p=%.4f). Recommended to rollback or iterate.",
				treatment.Variant, comp.RelativeLiftPct, comp.ConfidencePct, comp.PValue)
		}
	} else {
		comp.Status = domain.ExpStatusInconclusive
		comp.RecommendedAction = fmt.Sprintf("Results are not yet statistically significant (p=%.4f, confidence=%.1f%%). Continue running the experiment to gather more samples.",
			comp.PValue, comp.ConfidencePct)
	}

	return comp
}

// AnalyzeExperiment builds a complete statistical report for an A/B test.
func AnalyzeExperiment(
	flagKey, metricName string,
	eventType domain.EventType,
	env domain.Environment,
	controlVariant string,
	exposures map[string]int64,
	events []domain.ExperimentEvent,
) domain.ExperimentReport {
	if controlVariant == "" {
		controlVariant = "control"
	}

	report := domain.ExperimentReport{
		FlagKey:        flagKey,
		MetricName:     metricName,
		EventType:      eventType,
		Environment:    env,
		ControlVariant: controlVariant,
		VariantStats:   make(map[string]domain.VariantMetricStats),
		Comparisons:    make(map[string]domain.VariantComparison),
		GeneratedAt:    time.Now(),
	}

	// 1. Aggregate conversions and values by variant
	variantConversions := make(map[string]int64)
	variantSumValue := make(map[string]float64)

	for _, ev := range events {
		if ev.FlagKey != flagKey || ev.MetricName != metricName {
			continue
		}
		if ev.Environment != "" && ev.Environment != env {
			continue
		}

		report.TotalEvents++
		variantConversions[ev.Variant]++
		variantSumValue[ev.Variant] += ev.Value
	}

	// Ensure all variants with exposures or events are represented
	allVariants := make(map[string]bool)
	for v := range exposures {
		allVariants[v] = true
		report.TotalExposures += exposures[v]
	}
	for v := range variantConversions {
		allVariants[v] = true
	}
	allVariants[controlVariant] = true

	// 2. Compute individual variant metrics
	for v := range allVariants {
		n := exposures[v]
		k := variantConversions[v]
		report.VariantStats[v] = ComputeVariantBinaryStats(v, n, k)
	}

	// 3. Compare treatments against control
	controlStats, hasControl := report.VariantStats[controlVariant]
	if !hasControl {
		controlStats = domain.VariantMetricStats{Variant: controlVariant}
		report.VariantStats[controlVariant] = controlStats
	}

	var bestTreatment string
	var highestLift float64 = -math.MaxFloat64

	for v, stats := range report.VariantStats {
		if v == controlVariant {
			continue
		}
		comp := CompareBinaryVariants(controlStats, stats)
		report.Comparisons[v] = comp

		if comp.Status == domain.ExpStatusWinning && comp.RelativeLiftPct > highestLift {
			highestLift = comp.RelativeLiftPct
			bestTreatment = v
		}
	}

	if bestTreatment != "" {
		report.WinnerVariant = bestTreatment
	}

	return report
}
