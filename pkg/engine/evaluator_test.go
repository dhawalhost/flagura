package engine

import (
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func sampleRuleFlag() domain.FeatureFlag {
	return domain.FeatureFlag{
		ID:   "flag_test_rules",
		Key:  "ai-smart-search",
		Name: "AI Smart Search",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyRules,
				Percentage: 35,
				Rules: []domain.TargetingRule{
					{
						ID:        "rule_staff",
						Name:      "Staff Domain",
						Attribute: domain.AttrEmail,
						Operator:  domain.OpEndsWith,
						Values:    []string{"@flagship.dev"},
						Action:    domain.ActionForceEnabled,
					},
					{
						ID:        "rule_regex_qa",
						Name:      "QA Regex Pattern",
						Attribute: domain.AttrUserID,
						Operator:  domain.OpRegex,
						Values:    []string{`^qa_usr_[0-9]+$`},
						Action:    domain.ActionForceEnabled,
					},
				},
				Variants: []domain.FlagVariant{
					{Key: "control", Name: "Control", Value: false, Weight: 65},
					{Key: "treatment", Name: "Treatment", Value: true, Weight: 35},
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func samplePercentageFlag() domain.FeatureFlag {
	return domain.FeatureFlag{
		ID:   "flag_test_percentage",
		Key:  "checkout-v2",
		Name: "New Checkout Flow",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestEvaluateFlag(t *testing.T) {
	flag := sampleRuleFlag()

	// 1. Test Staff Match Rule
	ctxStaff := domain.EvaluationContext{
		UserID:      "usr_1",
		Email:       "alice@flagship.dev",
		Environment: domain.EnvProduction,
	}
	res1 := EvaluateFlag(flag, ctxStaff)
	if !res1.Enabled || res1.Reason != domain.ReasonTargetingRuleMatch {
		t.Fatalf("Expected targeting rule match, got: %v (%s)", res1.Enabled, res1.Reason)
	}

	// 2. Test Regex Match Rule
	ctxQA := domain.EvaluationContext{
		UserID:      "qa_usr_42",
		Email:       "tester@example.com",
		Environment: domain.EnvProduction,
	}
	resQA := EvaluateFlag(flag, ctxQA)
	if !resQA.Enabled || resQA.Reason != domain.ReasonTargetingRuleMatch {
		t.Fatalf("Expected regex targeting rule match, got: %v (%s)", resQA.Enabled, resQA.Reason)
	}

	// 3. Test Pure Percentage Rollout
	pctFlag := samplePercentageFlag()
	ctxUserA := domain.EvaluationContext{
		UserID:      "usr_100",
		Environment: domain.EnvProduction,
	}
	resPct := EvaluateFlag(pctFlag, ctxUserA)
	if resPct.Reason != domain.ReasonPercentageBucket && resPct.Reason != domain.ReasonPercentageExcluded {
		t.Fatalf("Expected percentage rollout evaluation reason, got: %s", resPct.Reason)
	}
}

func BenchmarkEvaluateFlag_PercentageRollout(b *testing.B) {
	flag := samplePercentageFlag()
	ctx := domain.EvaluationContext{
		UserID:      "bench_usr_123",
		Email:       "user@external.com",
		Country:     "US",
		Environment: domain.EnvProduction,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateFlag(flag, ctx)
	}
}

func BenchmarkEvaluateFlag_TargetingRuleMatch(b *testing.B) {
	flag := sampleRuleFlag()
	ctx := domain.EvaluationContext{
		UserID:      "bench_usr_staff",
		Email:       "alice@flagship.dev",
		Environment: domain.EnvProduction,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateFlag(flag, ctx)
	}
}

func BenchmarkEvaluateFlag_RegexRule(b *testing.B) {
	flag := sampleRuleFlag()
	ctx := domain.EvaluationContext{
		UserID:      "qa_usr_999",
		Email:       "external@gmail.com",
		Environment: domain.EnvProduction,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateFlag(flag, ctx)
	}
}

func BenchmarkFNV1a_HashOnly(b *testing.B) {
	input := "usr_dhawal_01:ai-smart-search"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FNV1a64(input)
	}
}

func BenchmarkGetStickyBucket(b *testing.B) {
	userID := "usr_dhawal_01"
	flagKey := "ai-smart-search"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetStickyBucket(userID, flagKey)
	}
}

func TestEvaluateFlagWithTrace(t *testing.T) {
	flag := sampleRuleFlag()

	// 1. Trace rule match
	ctxStaff := domain.EvaluationContext{
		UserID:      "usr_1",
		Email:       "alice@flagship.dev",
		Environment: domain.EnvProduction,
	}
	res, trace := EvaluateFlagWithTrace(flag, ctxStaff)
	if !res.Enabled || trace.FinalReason != domain.ReasonTargetingRuleMatch {
		t.Fatalf("expected rule match trace, got %v (%s)", res.Enabled, trace.FinalReason)
	}
	if len(trace.Steps) == 0 {
		t.Fatalf("expected trace steps to be populated")
	}

	// 2. Trace kill-switched flag
	flagDisabled := flag
	flagDisabled.Environments[domain.EnvProduction] = domain.EnvironmentConfig{
		Enabled: false,
	}
	resOff, traceOff := EvaluateFlagWithTrace(flagDisabled, ctxStaff)
	if resOff.Enabled || traceOff.FinalReason != domain.ReasonKillSwitchDisabled {
		t.Fatalf("expected kill-switch trace, got %v (%s)", resOff.Enabled, traceOff.FinalReason)
	}
	if len(traceOff.Steps) != 1 || traceOff.Steps[0].Passed {
		t.Fatalf("expected first step in trace to be kill-switch failed")
	}
}
