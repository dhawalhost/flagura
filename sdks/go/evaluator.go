package flagura

import (
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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
func GetStickyBucket(identifier string, salt string) float64 {
	combined := identifier + ":" + salt
	hash := FNV1a64(combined)
	slot := float64(hash % 10000)
	bucket := math.Round((slot/100.0)*100) / 100
	return bucket
}

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

// EvaluateCondition evaluates a single RuleCondition against context.
func EvaluateCondition(cond RuleCondition, ctx Context) bool {
	var targetVal interface{}

	switch strings.ToLower(cond.Attribute) {
	case "user_id", "userid":
		targetVal = ctx.UserID
	case "email":
		targetVal = ctx.Email
	case "country":
		targetVal = ctx.Country
	case "role":
		targetVal = ctx.Role
	case "tier":
		targetVal = ctx.Tier
	default:
		if ctx.Custom != nil {
			targetVal = ctx.Custom[cond.Attribute]
		}
	}

	if targetVal == nil {
		return false
	}

	strTarget := toStringFast(targetVal)
	strExpected := toStringFast(cond.Value)

	switch strings.ToUpper(cond.Operator) {
	case "EQUALS", "EQ", "==":
		return strTarget == strExpected
	case "NOT_EQUALS", "NEQ", "!=":
		return strTarget != strExpected
	case "CONTAINS":
		return strings.Contains(strTarget, strExpected)
	case "NOT_CONTAINS":
		return !strings.Contains(strTarget, strExpected)
	case "STARTS_WITH":
		return strings.HasPrefix(strTarget, strExpected)
	case "ENDS_WITH":
		return strings.HasSuffix(strTarget, strExpected)
	case "IN":
		parts := strings.Split(strExpected, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == strTarget {
				return true
			}
		}
		return false
	case "NOT_IN":
		parts := strings.Split(strExpected, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == strTarget {
				return false
			}
		}
		return true
	case "MATCHES_REGEX", "REGEX":
		re, err := getOrCompileRegex(fmt.Sprintf("%v", cond.Value))
		if err != nil {
			return false
		}
		return re.MatchString(fmt.Sprintf("%v", targetVal))
	case "GREATER_THAN", "GT", ">":
		tNum, err1 := strconv.ParseFloat(strTarget, 64)
		eNum, err2 := strconv.ParseFloat(strExpected, 64)
		if err1 == nil && err2 == nil {
			return tNum > eNum
		}
		return false
	case "LESS_THAN", "LT", "<":
		tNum, err1 := strconv.ParseFloat(strTarget, 64)
		eNum, err2 := strconv.ParseFloat(strExpected, 64)
		if err1 == nil && err2 == nil {
			return tNum < eNum
		}
		return false
	default:
		return false
	}
}

// Evaluate performs sub-microsecond local feature flag evaluation.
func Evaluate(flag FeatureFlag, ctx Context) EvaluationResult {
	start := time.Now()

	env := ctx.Environment
	if env == "" {
		env = EnvProduction
	}

	envCfg, exists := flag.Environments[env]
	if !exists {
		lat := time.Since(start)
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             false,
			Reason:              "ENVIRONMENT_NOT_FOUND",
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}
	}

	if !envCfg.Enabled {
		lat := time.Since(start)
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             false,
			Reason:              "FLAG_DISABLED",
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}
	}

	// Strategy routing
	switch envCfg.Strategy {
	case StrategyBoolean:
		lat := time.Since(start)
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             true,
			Reason:              "STRATEGY_BOOLEAN",
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}

	case StrategyPercentage:
		id := ctx.UserID
		if id == "" {
			id = "anonymous"
		}
		bucket := GetStickyBucket(id, flag.Key)
		enabled := bucket < float64(envCfg.Percentage)
		lat := time.Since(start)
		reason := "PERCENTAGE_ROLLOUT_EXCLUDED"
		if enabled {
			reason = "PERCENTAGE_ROLLOUT_INCLUDED"
		}
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             enabled,
			Bucket:              bucket,
			Reason:              reason,
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}

	case StrategyRules:
		for _, rule := range envCfg.Rules {
			allMatch := true
			for _, cond := range rule.Conditions {
				if !EvaluateCondition(cond, ctx) {
					allMatch = false
					break
				}
			}
			if allMatch {
				lat := time.Since(start)
				return EvaluationResult{
					FlagKey:             flag.Key,
					Enabled:             rule.Enabled,
					Variant:             rule.Variant,
					Value:               rule.Value,
					Reason:              "RULE_MATCH: " + rule.ID,
					EvaluationLatencyNs: lat.Nanoseconds(),
					EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
				}
			}
		}

		lat := time.Since(start)
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             false,
			Variant:             envCfg.DefaultVariant,
			Value:               envCfg.DefaultValue,
			Reason:              "RULES_DEFAULT_FALLBACK",
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}

	case StrategyMultivariate:
		id := ctx.UserID
		if id == "" {
			id = "anonymous"
		}
		bucket := GetStickyBucket(id, flag.Key)
		var cumulative float64
		for _, variant := range flag.Variants {
			cumulative += float64(variant.Weight)
			if bucket < cumulative {
				lat := time.Since(start)
				return EvaluationResult{
					FlagKey:             flag.Key,
					Enabled:             true,
					Variant:             variant.Key,
					Value:               variant.Value,
					Bucket:              bucket,
					Reason:              "MULTIVARIATE_MATCH",
					EvaluationLatencyNs: lat.Nanoseconds(),
					EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
				}
			}
		}

		lat := time.Since(start)
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             true,
			Variant:             envCfg.DefaultVariant,
			Value:               envCfg.DefaultValue,
			Bucket:              bucket,
			Reason:              "MULTIVARIATE_FALLBACK",
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}

	default:
		lat := time.Since(start)
		return EvaluationResult{
			FlagKey:             flag.Key,
			Enabled:             true,
			Reason:              "DEFAULT_ENABLED",
			EvaluationLatencyNs: lat.Nanoseconds(),
			EvaluationLatencyUs: float64(lat.Nanoseconds()) / 1000.0,
		}
	}
}
