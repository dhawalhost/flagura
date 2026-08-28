package engine

import (
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func sampleFlag() domain.FeatureFlag {
	return domain.FeatureFlag{
		ID:          "flag_test",
		Key:         "ai-smart-search",
		Name:        "AI Smart Search",
		Type:        "boolean",
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

func TestEvaluateFlag(t *testing.T) {
	flag := sampleFlag()

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

	// 2. Test Non-Staff
	ctxOther := domain.EvaluationContext{
		UserID:      "usr_2",
		Email:       "bob@example.com",
		Environment: domain.EnvProduction,
	}
	res2 := EvaluateFlag(flag, ctxOther)
	if !res2.Enabled {
		t.Logf("Evaluated default: %s (enabled=%v)", res2.Reason, res2.Enabled)
	}
}

func BenchmarkEvaluateFlag(b *testing.B) {
	flag := sampleFlag()
	ctx := domain.EvaluationContext{
		UserID:      "bench_usr_123",
		Email:       "user@example.com",
		Country:     "US",
		Tier:        "enterprise",
		Environment: domain.EnvProduction,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateFlag(flag, ctx)
	}
}
