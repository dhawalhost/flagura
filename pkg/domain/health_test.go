package domain_test

import (
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestFlagHealthAnalyzer(t *testing.T) {
	// 1. 100% Rolled out flag should be READY_FOR_CLEANUP
	staleFlag := domain.FeatureFlag{
		Key:  "permanent-flag",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 100,
			},
		},
	}
	report1 := domain.AnalyzeFlagHealth(staleFlag)
	if report1.Status != domain.HealthStatusStale {
		t.Fatalf("expected HealthStatusStale for 100%% flag, got %s", report1.Status)
	}
	if !report1.IsStale {
		t.Fatalf("expected IsStale true")
	}

	// 2. Disabled flag should be DEAD_FLAG
	deadFlag := domain.FeatureFlag{
		Key:  "dead-flag",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled: false,
			},
		},
	}
	report2 := domain.AnalyzeFlagHealth(deadFlag)
	if report2.Status != domain.HealthStatusDead {
		t.Fatalf("expected HealthStatusDead for disabled flag, got %s", report2.Status)
	}

	// 3. Active canary rollout flag should be ACTIVE
	activeFlag := domain.FeatureFlag{
		Key:  "canary-flag",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 25,
			},
		},
	}
	report3 := domain.AnalyzeFlagHealth(activeFlag)
	if report3.Status != domain.HealthStatusActive {
		t.Fatalf("expected HealthStatusActive for canary flag, got %s", report3.Status)
	}
	if report3.IsStale {
		t.Fatalf("expected IsStale false")
	}
}
