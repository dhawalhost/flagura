package domain

import (
	"testing"
	"time"
)

func TestAnalyzeFlagHealth(t *testing.T) {
	tests := []struct {
		name           string
		flag           FeatureFlag
		expectedStatus HealthStatus
		expectedStale  bool
	}{
		{
			name: "100% Rolled out permanent flag -> READY_FOR_CLEANUP",
			flag: FeatureFlag{
				Key:  "permanent-flag",
				Type: "boolean",
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled:    true,
						Strategy:   StrategyPercentage,
						Percentage: 100,
					},
				},
			},
			expectedStatus: HealthStatusStale,
			expectedStale:  true,
		},
		{
			name: "Disabled feature flag -> DEAD_FLAG",
			flag: FeatureFlag{
				Key:  "dead-flag",
				Type: "boolean",
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled: false,
					},
				},
			},
			expectedStatus: HealthStatusDead,
			expectedStale:  true,
		},
		{
			name: "Active canary rollout flag -> ACTIVE",
			flag: FeatureFlag{
				Key:  "canary-flag",
				Type: "boolean",
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled:    true,
						Strategy:   StrategyPercentage,
						Percentage: 25,
					},
				},
			},
			expectedStatus: HealthStatusActive,
			expectedStale:  false,
		},
		{
			name: "Recently disabled flag (< 14 days ago) -> ACTIVE (incident response, not dead)",
			flag: FeatureFlag{
				Key:       "incident-flag",
				Type:      "boolean",
				UpdatedAt: time.Now().Add(-1 * time.Hour),
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled: false,
					},
				},
			},
			expectedStatus: HealthStatusActive,
			expectedStale:  false,
		},
		{
			name: "Recently 100% launched flag (< 7 days ago) -> ACTIVE (in bake period)",
			flag: FeatureFlag{
				Key:       "bake-period-flag",
				Type:      "boolean",
				UpdatedAt: time.Now().Add(-24 * time.Hour),
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled:    true,
						Strategy:   StrategyPercentage,
						Percentage: 100,
					},
				},
			},
			expectedStatus: HealthStatusActive,
			expectedStale:  false,
		},
		{
			name: "Flag disabled in production but active in staging -> DEAD_FLAG with multi-env notice",
			flag: FeatureFlag{
				Key:       "staging-active-flag",
				Type:      "boolean",
				UpdatedAt: time.Now().Add(-30 * 24 * time.Hour),
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled: false,
					},
					EnvStaging: {
						Enabled: true,
					},
				},
			},
			expectedStatus: HealthStatusDead,
			expectedStale:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := AnalyzeFlagHealth(tt.flag)
			if report.Status != tt.expectedStatus {
				t.Errorf("AnalyzeFlagHealth() status = %s, expected %s", report.Status, tt.expectedStatus)
			}
			if report.IsStale != tt.expectedStale {
				t.Errorf("AnalyzeFlagHealth() isStale = %v, expected %v", report.IsStale, tt.expectedStale)
			}
		})
	}
}
