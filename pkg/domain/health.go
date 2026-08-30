package domain

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

	// 1. Check if disabled (Dead flag)
	if !prodEnv.Enabled {
		return FlagHealthReport{
			Status:          HealthStatusDead,
			IsStale:         true,
			Reason:          "Kill-switch is engaged in production (0% traffic)",
			SuggestedAction: "If this feature was cancelled, remove the code branch and delete this flag.",
		}
	}

	// 2. Check if 100% rolled out with no active targeting rules
	if prodEnv.Strategy == StrategyPercentage && prodEnv.Percentage >= 100 && len(prodEnv.Rules) == 0 {
		return FlagHealthReport{
			Status:          HealthStatusStale,
			IsStale:         true,
			Reason:          "100% rolled out to all users in production with no custom rules",
			SuggestedAction: "Feature is fully launched. Clean up flag check in your codebase to eliminate technical debt.",
		}
	}

	if prodEnv.Strategy == StrategyBoolean && len(prodEnv.Rules) == 0 {
		return FlagHealthReport{
			Status:          HealthStatusStale,
			IsStale:         true,
			Reason:          "Permanently enabled for all users in production with no custom rules",
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
