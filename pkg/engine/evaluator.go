package engine

import (
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

var regexCache sync.Map // pattern string -> *regexp.Regexp

func getOrCompileRegex(patternStr string) (*regexp.Regexp, error) {
	if val, ok := regexCache.Load(patternStr); ok {
		return val.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile("(?i)" + patternStr)
	if err != nil {
		return nil, err
	}
	regexCache.Store(patternStr, re)
	return re, nil
}

// FNV1a64 computes deterministic 64-bit FNV-1a hash
func FNV1a64(input string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(input))
	return h.Sum64()
}

// GetStickyBucket computes sticky percentage bucket (0.00 to 99.99)
func GetStickyBucket(identifier string, salt string) (float64, string) {
	combined := identifier + ":" + salt
	hash := FNV1a64(combined)
	slot := float64(hash % 10000)
	bucket := math.Round((slot/100.0)*100) / 100
	hashRaw := strconv.FormatUint(hash, 16)
	return bucket, hashRaw
}

// toStringFast converts primitive types to string with minimal allocations.
func toStringFast(val interface{}) string {
	switch v := val.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(v))
	case fmt.Stringer:
		return strings.ToLower(strings.TrimSpace(v.String()))
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
	}
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

	strVal := toStringFast(targetVal)
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
		pattern, err := getOrCompileRegex(rule.Values[0])
		if err != nil {
			return false
		}
		return pattern.MatchString(strVal)

	default:
		return false
	}
}

// ResolveMultivariateVariant picks variant based on weights and hash
func ResolveMultivariateVariant(variants []domain.FlagVariant, identifier, flagKey string) (domain.FlagVariant, float64, string) {
	if len(variants) == 0 {
		return domain.FlagVariant{Key: "default", Name: "Default", Value: true, Weight: 100}, 0, "0"
	}

	bucket, hashRaw := GetStickyBucket(identifier, flagKey+":multivariate")
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

// RuleEvaluationStep represents an individual targeting condition check
type RuleEvaluationStep struct {
	StepIndex int    `json:"step_index"`
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	Detail    string `json:"detail"`
}

// EvaluationTrace contains comprehensive diagnostics for an evaluation request
type EvaluationTrace struct {
	FlagKey        string               `json:"flag_key"`
	Environment    domain.Environment   `json:"environment"`
	IdentifierUsed string               `json:"identifier_used"`
	Steps          []RuleEvaluationStep `json:"steps"`
	FinalReason    domain.EvaluationReason `json:"final_reason"`
	FinalVariant   string               `json:"final_variant"`
	FinalEnabled   bool                 `json:"final_enabled"`
	Bucket         float64              `json:"bucket"`
	ElapsedNs      int64                `json:"elapsed_ns"`
}

// EvaluateFlagWithTrace performs evaluation and builds a step-by-step diagnostics trace
func EvaluateFlagWithTrace(flag domain.FeatureFlag, ctx domain.EvaluationContext) (domain.EvaluationResult, EvaluationTrace) {
	start := time.Now()
	steps := make([]RuleEvaluationStep, 0, 4)
	stepIdx := 1

	env := ctx.Environment
	if env == "" {
		env = domain.EnvProduction
	}

	identifier := ctx.UserID
	if identifier == "" {
		identifier = ctx.Email
	}
	if identifier == "" {
		identifier = "anonymous-user"
	}

	envConfig, envExists := flag.Environments[env]
	if !envExists {
		steps = append(steps, RuleEvaluationStep{
			StepIndex: stepIdx,
			Name:      "Environment Configuration Check",
			Passed:    false,
			Detail:    fmt.Sprintf("Environment %q is not configured on this flag.", env),
		})
		res := domain.EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             false,
			Variant:             "off",
			Value:               false,
			Reason:              domain.ReasonEnvDisabled,
			EvaluationLatencyNs: time.Since(start).Nanoseconds(),
		}
		return res, EvaluationTrace{
			FlagKey:        flag.Key,
			Environment:    env,
			IdentifierUsed: identifier,
			Steps:          steps,
			FinalReason:    res.Reason,
			FinalVariant:   res.Variant,
			FinalEnabled:   res.Enabled,
			ElapsedNs:      res.EvaluationLatencyNs,
		}
	}

	// 1. Kill Switch check
	if !envConfig.Enabled {
		steps = append(steps, RuleEvaluationStep{
			StepIndex: stepIdx,
			Name:      "Master Kill-Switch Check",
			Passed:    false,
			Detail:    fmt.Sprintf("Kill-Switch is engaged in %s environment (Flag is turned OFF).", env),
		})
		offVar := envConfig.OffVariant
		if offVar == "" {
			offVar = "off"
		}
		res := domain.EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             false,
			Variant:             offVar,
			Value:               false,
			Reason:              domain.ReasonKillSwitchDisabled,
			EvaluationLatencyNs: time.Since(start).Nanoseconds(),
		}
		return res, EvaluationTrace{
			FlagKey:        flag.Key,
			Environment:    env,
			IdentifierUsed: identifier,
			Steps:          steps,
			FinalReason:    res.Reason,
			FinalVariant:   res.Variant,
			FinalEnabled:   res.Enabled,
			ElapsedNs:      res.EvaluationLatencyNs,
		}
	}

	steps = append(steps, RuleEvaluationStep{
		StepIndex: stepIdx,
		Name:      "Master Kill-Switch Check",
		Passed:    true,
		Detail:    fmt.Sprintf("Flag is active and enabled in %s environment.", env),
	})
	stepIdx++

	// 2. Targeting Rules
	if len(envConfig.Rules) > 0 {
		for _, rule := range envConfig.Rules {
			matched := EvaluateRule(rule, ctx)
			if matched {
				steps = append(steps, RuleEvaluationStep{
					StepIndex: stepIdx,
					Name:      fmt.Sprintf("Targeting Rule Match: %s", rule.Name),
					Passed:    true,
					Detail:    fmt.Sprintf("Condition matched (%s %s %v). Action: %s.", rule.Attribute, rule.Operator, rule.Values, rule.Action),
				})

				res := EvaluateFlag(flag, ctx)
				return res, EvaluationTrace{
					FlagKey:        flag.Key,
					Environment:    env,
					IdentifierUsed: identifier,
					Steps:          steps,
					FinalReason:    res.Reason,
					FinalVariant:   res.Variant,
					FinalEnabled:   res.Enabled,
					ElapsedNs:      time.Since(start).Nanoseconds(),
				}
			} else {
				steps = append(steps, RuleEvaluationStep{
					StepIndex: stepIdx,
					Name:      fmt.Sprintf("Targeting Rule Check: %s", rule.Name),
					Passed:    false,
					Detail:    fmt.Sprintf("Did not match context condition (%s %s %v).", rule.Attribute, rule.Operator, rule.Values),
				})
				stepIdx++
			}
		}
	}

	// 3. Strategy Resolution (Percentage / Boolean / Multivariate)
	bucket, _ := GetStickyBucket(identifier, flag.Key)

	if envConfig.Strategy == domain.StrategyPercentage {
		passed := bucket < envConfig.Percentage
		steps = append(steps, RuleEvaluationStep{
			StepIndex: stepIdx,
			Name:      "Deterministic FNV-1a Percentage Rollout",
			Passed:    passed,
			Detail:    fmt.Sprintf("User sticky bucket #%.2f%% evaluated against rollout threshold %.1f%%.", bucket, envConfig.Percentage),
		})
	} else if envConfig.Strategy == domain.StrategyMultivariate {
		steps = append(steps, RuleEvaluationStep{
			StepIndex: stepIdx,
			Name:      "Multivariate Traffic Split",
			Passed:    true,
			Detail:    fmt.Sprintf("User assigned to weighted variant bucket #%.2f%%.", bucket),
		})
	} else {
		steps = append(steps, RuleEvaluationStep{
			StepIndex: stepIdx,
			Name:      "Default Boolean Strategy",
			Passed:    true,
			Detail:    "Standard default boolean rollout applied.",
		})
	}

	res := EvaluateFlag(flag, ctx)
	return res, EvaluationTrace{
		FlagKey:        flag.Key,
		Environment:    env,
		IdentifierUsed: identifier,
		Steps:          steps,
		FinalReason:    res.Reason,
		FinalVariant:   res.Variant,
		FinalEnabled:   res.Enabled,
		Bucket:         bucket,
		ElapsedNs:      time.Since(start).Nanoseconds(),
	}
}
