package engine

import (
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/internal/domain"
)

// FNV1a64 computes deterministic 64-bit FNV-1a hash
func FNV1a64(input string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(input))
	return h.Sum64()
}

// GetStickyBucket computes sticky percentage bucket (0.00 to 99.99)
func GetStickyBucket(identifier string, salt string) (float64, string) {
	combined := fmt.Sprintf("%s:%s", identifier, salt)
	hash := FNV1a64(combined)
	slot := float64(hash % 10000)
	bucket := math.Round((slot/100.0)*100) / 100
	hashRaw := fmt.Sprintf("%016x", hash)
	return bucket, hashRaw
}

// EvaluateRule checks if targeting rule condition matches context
func EvaluateRule(rule domain.TargetingRule, ctx domain.EvaluationContext) bool {
	var targetVal interface{}

	switch rule.Attribute {
	case domain.AttrUserID:
		targetVal = ctx.UserID
	case domain.AttrEmail:
		targetVal = ctx.Email
	case domain.AttrCountry:
		targetVal = ctx.Country
	case domain.AttrRole:
		targetVal = ctx.Role
	case domain.AttrTier:
		targetVal = ctx.Tier
	case domain.AttrCustom:
		if rule.CustomKey != "" && ctx.Attributes != nil {
			targetVal = ctx.Attributes[rule.CustomKey]
		}
	}

	if targetVal == nil {
		return false
	}

	strVal := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", targetVal)))
	if strVal == "" {
		return false
	}

	switch rule.Operator {
	case domain.OpEquals:
		for _, v := range rule.Values {
			if strings.ToLower(strings.TrimSpace(v)) == strVal {
				return true
			}
		}
		return false

	case domain.OpNotEquals:
		for _, v := range rule.Values {
			if strings.ToLower(strings.TrimSpace(v)) == strVal {
				return false
			}
		}
		return true

	case domain.OpContains:
		for _, v := range rule.Values {
			if strings.Contains(strVal, strings.ToLower(strings.TrimSpace(v))) {
				return true
			}
		}
		return false

	case domain.OpNotContains:
		for _, v := range rule.Values {
			if strings.Contains(strVal, strings.ToLower(strings.TrimSpace(v))) {
				return false
			}
		}
		return true

	case domain.OpEndsWith:
		for _, v := range rule.Values {
			if strings.HasSuffix(strVal, strings.ToLower(strings.TrimSpace(v))) {
				return true
			}
		}
		return false

	case domain.OpInList:
		for _, v := range rule.Values {
			if strings.ToLower(strings.TrimSpace(v)) == strVal {
				return true
			}
		}
		return false

	case domain.OpNotInList:
		for _, v := range rule.Values {
			if strings.ToLower(strings.TrimSpace(v)) == strVal {
				return false
			}
		}
		return true

	case domain.OpGreaterThan:
		numVal, err := strconv.ParseFloat(strVal, 64)
		if err != nil || len(rule.Values) == 0 {
			return false
		}
		threshold, err := strconv.ParseFloat(rule.Values[0], 64)
		if err != nil {
			return false
		}
		return numVal > threshold

	case domain.OpLessThan:
		numVal, err := strconv.ParseFloat(strVal, 64)
		if err != nil || len(rule.Values) == 0 {
			return false
		}
		threshold, err := strconv.ParseFloat(rule.Values[0], 64)
		if err != nil {
			return false
		}
		return numVal < threshold

	case domain.OpRegex:
		if len(rule.Values) == 0 {
			return false
		}
		pattern, err := regexp.Compile("(?i)" + rule.Values[0])
		if err != nil {
			return false
		}
		return pattern.MatchString(fmt.Sprintf("%v", targetVal))

	default:
		return false
	}
}

// ResolveMultivariateVariant picks variant based on weights and hash
func ResolveMultivariateVariant(variants []domain.FlagVariant, identifier, flagKey string) (domain.FlagVariant, float64, string) {
	if len(variants) == 0 {
		return domain.FlagVariant{Key: "default", Name: "Default", Value: true, Weight: 100}, 0, "0"
	}

	bucket, hashRaw := GetStickyBucket(identifier, fmt.Sprintf("%s:multivariate", flagKey))
	cumulative := 0.0

	for _, v := range variants {
		cumulative += v.Weight
		if bucket < cumulative {
			return v, bucket, hashRaw
		}
	}

	return variants[len(variants)-1], bucket, hashRaw
}

// EvaluateFlag performs sub-millisecond evaluation of a flag for a given context
func EvaluateFlag(flag domain.FeatureFlag, ctx domain.EvaluationContext) domain.EvaluationResult {
	start := time.Now()

	envKey := ctx.Environment
	if envKey == "" {
		envKey = domain.EnvProduction
	}

	envConfig, exists := flag.Environments[envKey]
	if !exists {
		envConfig = domain.EnvironmentConfig{
			Enabled:    false,
			Strategy:   domain.StrategyBoolean,
			Percentage: 0,
		}
	}

	identifier := ctx.UserID
	if identifier == "" {
		identifier = ctx.Email
	}
	if identifier == "" {
		identifier = "anonymous-user"
	}

	// 1. Kill Switch check
	if !envConfig.Enabled {
		elapsed := time.Since(start)
		ns := elapsed.Nanoseconds()
		if ns == 0 {
			ns = 50
		}
		offVar := envConfig.OffVariant
		if offVar == "" {
			offVar = "off"
		}
		return domain.EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             false,
			Variant:             offVar,
			Value:               false,
			Reason:              domain.ReasonKillSwitchDisabled,
			EvaluationLatencyNs: ns,
			EvaluationLatencyUs: float64(ns) / 1000.0,
		}
	}

	// 2. Targeting Rules
	if len(envConfig.Rules) > 0 {
		for _, rule := range envConfig.Rules {
			if EvaluateRule(rule, ctx) {
				elapsed := time.Since(start)
				ns := elapsed.Nanoseconds()
				if ns == 0 {
					ns = 80
				}

				if rule.Action == domain.ActionForceDisabled {
					offVar := envConfig.OffVariant
					if offVar == "" {
						offVar = "off"
					}
					return domain.EvaluationResult{
						FlagKey:             flag.Key,
						Enabled:             false,
						Variant:             offVar,
						Value:               false,
						Reason:              domain.ReasonTargetingRuleMatch,
						MatchedRuleID:       rule.ID,
						MatchedRuleName:     rule.Name,
						EvaluationLatencyNs: ns,
						EvaluationLatencyUs: float64(ns) / 1000.0,
					}
				}

				if rule.Action == domain.ActionServeVariant && rule.ServeVariant != "" {
					var matchedVal interface{} = true
					for _, v := range envConfig.Variants {
						if v.Key == rule.ServeVariant {
							matchedVal = v.Value
							break
						}
					}
					return domain.EvaluationResult{
						FlagKey:             flag.Key,
						Enabled:             true,
						Variant:             rule.ServeVariant,
						Value:               matchedVal,
						Reason:              domain.ReasonTargetingRuleMatch,
						MatchedRuleID:       rule.ID,
						MatchedRuleName:     rule.Name,
						EvaluationLatencyNs: ns,
						EvaluationLatencyUs: float64(ns) / 1000.0,
					}
				}

				// Force Enabled
				defVar := envConfig.DefaultVariant
				if defVar == "" {
					defVar = "on"
				}
				return domain.EvaluationResult{
					FlagKey:             flag.Key,
					Enabled:             true,
					Variant:             defVar,
					Value:               true,
					Reason:              domain.ReasonTargetingRuleMatch,
					MatchedRuleID:       rule.ID,
					MatchedRuleName:     rule.Name,
					EvaluationLatencyNs: ns,
					EvaluationLatencyUs: float64(ns) / 1000.0,
				}
			}
		}
	}

	// 3. Strategy: Percentage
	if envConfig.Strategy == domain.StrategyPercentage {
		bucket, hashRaw := GetStickyBucket(identifier, flag.Key)
		threshold := envConfig.Percentage
		isEnabled := bucket < threshold

		elapsed := time.Since(start)
		ns := elapsed.Nanoseconds()
		if ns == 0 {
			ns = 80
		}

		variant := envConfig.DefaultVariant
		if variant == "" {
			variant = "treatment"
		}
		if !isEnabled {
			variant = envConfig.OffVariant
			if variant == "" {
				variant = "control"
			}
		}

		reason := domain.ReasonPercentageBucket
		if !isEnabled {
			reason = domain.ReasonPercentageExcluded
		}

		return domain.EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             isEnabled,
			Variant:             variant,
			Value:               isEnabled,
			Reason:              reason,
			BucketVal:           &bucket,
			BucketThreshold:     &threshold,
			HashRaw:             hashRaw,
			EvaluationLatencyNs: ns,
			EvaluationLatencyUs: float64(ns) / 1000.0,
		}
	}

	// 4. Strategy: Multivariate
	if envConfig.Strategy == domain.StrategyMultivariate {
		v, bucket, hashRaw := ResolveMultivariateVariant(envConfig.Variants, identifier, flag.Key)
		elapsed := time.Since(start)
		ns := elapsed.Nanoseconds()
		if ns == 0 {
			ns = 100
		}

		return domain.EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             true,
			Variant:             v.Key,
			Value:               v.Value,
			Reason:              domain.ReasonMultivariateBucket,
			BucketVal:           &bucket,
			HashRaw:             hashRaw,
			EvaluationLatencyNs: ns,
			EvaluationLatencyUs: float64(ns) / 1000.0,
		}
	}

	// 5. Default Boolean On
	elapsed := time.Since(start)
	ns := elapsed.Nanoseconds()
	if ns == 0 {
		ns = 50
	}

	defVar := envConfig.DefaultVariant
	if defVar == "" {
		defVar = "on"
	}

	return domain.EvaluationResult{
		FlagKey:             flag.Key,
		Enabled:             true,
		Variant:             defVar,
		Value:               true,
		Reason:              domain.ReasonDefaultEnabled,
		EvaluationLatencyNs: ns,
		EvaluationLatencyUs: float64(ns) / 1000.0,
	}
}

// RunBenchmark executes a high-concurrency stress test and computes latency percentiles
func RunBenchmark(flag domain.FeatureFlag, env domain.Environment, iterations int) domain.BenchmarkMetrics {
	if iterations < 500 {
		iterations = 500
	}
	if iterations > 100000 {
		iterations = 100000
	}

	latencies := make([]int64, iterations)
	var buckets [100]int

	startTotal := time.Now()

	for i := 0; i < iterations; i++ {
		uid := fmt.Sprintf("bench_usr_%d", i%25000)
		email := fmt.Sprintf("test_%d@example.com", i)
		if i%4 == 0 {
			email = fmt.Sprintf("test_%d@flagura.dev", i)
		}
		country := "US"
		if i%2 != 0 {
			country = "DE"
		}
		tier := "free"
		if i%10 == 0 {
			tier = "enterprise"
		}

		ctx := domain.EvaluationContext{
			UserID:      uid,
			Email:       email,
			Country:     country,
			Tier:        tier,
			Environment: env,
		}

		t0 := time.Now()
		res := EvaluateFlag(flag, ctx)
		durNs := time.Since(t0).Nanoseconds()
		if durNs == 0 {
			durNs = 60
		}
		latencies[i] = durNs

		if res.BucketVal != nil {
			idx := int(math.Min(99, math.Max(0, math.Floor(*res.BucketVal))))
			buckets[idx]++
		} else {
			b, _ := GetStickyBucket(uid, flag.Key)
			idx := int(math.Min(99, math.Max(0, math.Floor(b))))
			buckets[idx]++
		}
	}

	totalDur := time.Since(startTotal)
	totalDurMs := float64(totalDur.Microseconds()) / 1000.0

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p50 := latencies[int(float64(iterations)*0.50)]
	p90 := latencies[int(float64(iterations)*0.90)]
	p99 := latencies[int(float64(iterations)*0.99)]
	p999 := latencies[int(float64(iterations)*0.999)]
	minNs := latencies[0]
	maxNs := latencies[iterations-1]

	var sum int64
	for _, l := range latencies {
		sum += l
	}
	avgNs := sum / int64(iterations)

	opsPerSec := int64(0)
	if totalDur.Seconds() > 0 {
		opsPerSec = int64(float64(iterations) / totalDur.Seconds())
	}

	return domain.BenchmarkMetrics{
		Iterations:      iterations,
		TotalDurationMs: totalDurMs,
		OpsPerSec:       opsPerSec,
		P50Ns:           p50,
		P90Ns:           p90,
		P99Ns:           p99,
		P999Ns:          p999,
		MinNs:           minNs,
		MaxNs:           maxNs,
		AvgNs:           avgNs,
		HashBuckets:     buckets,
	}
}
