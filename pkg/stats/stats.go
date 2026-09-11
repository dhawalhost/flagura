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
	return CompareBinaryVariantsWithCorrection(control, treatment, 1)
}

// CompareBinaryVariantsWithCorrection performs a two-proportion pooled Z-test with Bonferroni correction for multiple variants.
func CompareBinaryVariantsWithCorrection(control, treatment domain.VariantMetricStats, numComparisons int) domain.VariantComparison {
	if numComparisons < 1 {
		numComparisons = 1
	}

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
	} else if pT > 0 {
		comp.RelativeLiftPct = math.Inf(1)
	} else {
		comp.RelativeLiftPct = 0.0
	}

	// Baseline sample size guardrail
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

	// Rate-scaled sample size requirement: normal approximation requires n*p >= 5 and n*(1-p) >= 5
	minExpectedSuccess := math.Min(nC*pPool, nT*pPool)
	minExpectedFailure := math.Min(nC*(1.0-pPool), nT*(1.0-pPool))
	if minExpectedSuccess < 5.0 || minExpectedFailure < 5.0 {
		var reqN int64 = 100
		if pPool > 0 && pPool < 1.0 {
			rate := math.Min(pPool, 1.0-pPool)
			reqN = int64(math.Ceil(5.0 / rate))
			if reqN < MinSampleSizeForSignificance {
				reqN = MinSampleSizeForSignificance
			}
		}
		comp.Status = domain.ExpStatusInsufficientData
		comp.RequiredSampleSize = reqN
		comp.RecommendedAction = fmt.Sprintf("Observed conversion rate (%.1f%%) requires at least %d exposures per variant for statistical test validity (n·p ≥ 5 rule). Current: Control=%d, Treatment=%d.",
			pPool*100.0, reqN, control.Exposures, treatment.Exposures)
		return comp
	}

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

	rawPValue := TwoTailedPValue(zScore)
	// Bonferroni correction for multiple hypothesis testing
	adjPValue := math.Min(1.0, rawPValue*float64(numComparisons))
	comp.PValue = adjPValue
	comp.ConfidencePct = math.Max(0.0, (1.0-adjPValue)*100.0)

	comp.IsSignificant95 = adjPValue < 0.05
	comp.IsSignificant99 = adjPValue < 0.01

	correctionNote := ""
	if numComparisons > 1 {
		correctionNote = fmt.Sprintf(" (Bonferroni-corrected for %d variants)", numComparisons)
	}

	// Determine status and recommendation
	if comp.IsSignificant95 {
		if comp.AbsoluteLift > 0 {
			comp.Status = domain.ExpStatusWinning
			liftStr := fmt.Sprintf("+%.2f%%", comp.RelativeLiftPct)
			if math.IsInf(comp.RelativeLiftPct, 1) {
				liftStr = fmt.Sprintf("+%.2f%% absolute (baseline 0.00%%)", comp.AbsoluteLift*100.0)
			}
			comp.RecommendedAction = fmt.Sprintf("Treatment '%s' is outperforming Control by %s (Statistically Significant with %.1f%% confidence, p=%.4f%s). Safe to roll out to 100%%.",
				treatment.Variant, liftStr, comp.ConfidencePct, comp.PValue, correctionNote)
		} else {
			comp.Status = domain.ExpStatusLosing
			comp.RecommendedAction = fmt.Sprintf("Treatment '%s' is underperforming Control by %.2f%% (Statistically Significant with %.1f%% confidence, p=%.4f%s). Recommended to rollback or iterate.",
				treatment.Variant, comp.RelativeLiftPct, comp.ConfidencePct, comp.PValue, correctionNote)
		}
	} else {
		comp.Status = domain.ExpStatusInconclusive
		comp.RecommendedAction = fmt.Sprintf("Results are not yet statistically significant (p=%.4f%s, confidence=%.1f%%). Continue running the experiment to gather more samples.",
			comp.PValue, correctionNote, comp.ConfidencePct)
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

	// 1. Aggregate conversions and values by variant (deduplicating by UserID for binary proportion independence)
	variantConversions := make(map[string]int64)
	variantSumValue := make(map[string]float64)
	variantUniqueConvertedUsers := make(map[string]map[string]struct{})

	for _, ev := range events {
		if ev.FlagKey != flagKey || ev.MetricName != metricName {
			continue
		}
		if ev.Environment != "" && ev.Environment != env {
			continue
		}

		report.TotalEvents++
		if ev.UserID != "" {
			if _, ok := variantUniqueConvertedUsers[ev.Variant]; !ok {
				variantUniqueConvertedUsers[ev.Variant] = make(map[string]struct{})
			}
			variantUniqueConvertedUsers[ev.Variant][ev.UserID] = struct{}{}
		} else {
			variantConversions[ev.Variant]++
		}
		variantSumValue[ev.Variant] += ev.Value
	}

	for v, users := range variantUniqueConvertedUsers {
		variantConversions[v] += int64(len(users))
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
		if n < k {
			n = k // Sample size cannot be less than converted unique users
		}
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

	numComparisons := len(report.VariantStats) - 1
	if numComparisons < 1 {
		numComparisons = 1
	}

	for v, stats := range report.VariantStats {
		if v == controlVariant {
			continue
		}
		comp := CompareBinaryVariantsWithCorrection(controlStats, stats, numComparisons)
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
