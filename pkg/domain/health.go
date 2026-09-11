package domain

import "time"

// HealthStatus represents the lifecycle health and technical debt state of a feature flag.
type HealthStatus string

const (
	// HealthStatusActive indicates the flag is actively routing traffic or evaluating rules.
	HealthStatusActive HealthStatus = "ACTIVE"
	// HealthStatusStale indicates the flag is 100% enabled across traffic and ready for code cleanup.
	HealthStatusStale HealthStatus = "READY_FOR_CLEANUP"
	// HealthStatusDead indicates the flag is permanently disabled/kill-switched.
	HealthStatusDead HealthStatus = "DEAD_FLAG"
)

// FlagHealthReport contains actionable health analysis for a feature flag.
type FlagHealthReport struct {
	Status          HealthStatus `json:"status"`
	IsStale         bool         `json:"is_stale"`
	Reason          string       `json:"reason"`
	SuggestedAction string       `json:"suggested_action"`
}

// AnalyzeFlagHealth inspects a feature flag's environment configurations to detect stale flag debt.
func AnalyzeFlagHealth(flag FeatureFlag) FlagHealthReport {
	prodEnv, prodExists := flag.Environments[EnvProduction]
	if !prodExists {
		return FlagHealthReport{
			Status:          HealthStatusActive,
			IsStale:         false,
			Reason:          "Flag has no production environment configuration",
			SuggestedAction: "Configure production environment rules",
		}
	}

	otherEnvActive := false
	for envName, envCfg := range flag.Environments {
		if envName != EnvProduction && envCfg.Enabled {
			otherEnvActive = true
			break
		}
	}

	// 1. Check if disabled in production
	if !prodEnv.Enabled {
		// If disabled recently (< 14 days), do not classify as DEAD_FLAG or recommend deletion
		if !flag.UpdatedAt.IsZero() && time.Since(flag.UpdatedAt) < 14*24*time.Hour {
			reason := "Kill-switch recently engaged in production; confirm this was intentional before taking action"
			if otherEnvActive {
				reason += " (other environments are still enabled)"
			}
			return FlagHealthReport{
				Status:          HealthStatusActive,
				IsStale:         false,
				Reason:          reason,
				SuggestedAction: "Recently disabled — confirm incident response resolution before decommissioning flag.",
			}
		}

		reason := "Kill-switch is engaged in production (0% traffic for 14+ days)"
		suggestedAction := "If this feature was cancelled, remove the code branch and delete this flag."
		if otherEnvActive {
			reason = "Kill-switch engaged in production for 14+ days, but other environments are still active"
			suggestedAction = "Confirm feature is cancelled across all environments before deleting flag."
		}

		return FlagHealthReport{
			Status:          HealthStatusDead,
			IsStale:         true,
			Reason:          reason,
			SuggestedAction: suggestedAction,
		}
	}

	// 2. Check if 100% rolled out with no active targeting rules
	isFullRollout := (prodEnv.Strategy == StrategyPercentage && prodEnv.Percentage >= 100 && len(prodEnv.Rules) == 0) ||
		(prodEnv.Strategy == StrategyBoolean && len(prodEnv.Rules) == 0)

	if isFullRollout {
		// Check bake period (< 7 days)
		if !flag.UpdatedAt.IsZero() && time.Since(flag.UpdatedAt) < 7*24*time.Hour {
			return FlagHealthReport{
				Status:          HealthStatusActive,
				IsStale:         false,
				Reason:          "100% rolled out to production (within 7-day bake period)",
				SuggestedAction: "Allow bake period to conclude before removing code branch from repository.",
			}
		}

		return FlagHealthReport{
			Status:          HealthStatusStale,
			IsStale:         true,
			Reason:          "100% rolled out to all users in production with no custom rules",
			SuggestedAction: "Feature is fully launched. Clean up flag check in your codebase to eliminate technical debt.",
		}
	}

	// 3. Otherwise active (partial rollout or targeting rules active)
	return FlagHealthReport{
		Status:          HealthStatusActive,
		IsStale:         false,
		Reason:          "Actively evaluating traffic splits or custom targeting rules",
		SuggestedAction: "Monitor canary health and increment percentage when confident.",
	}
}
