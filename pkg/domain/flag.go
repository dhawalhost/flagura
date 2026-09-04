package domain

import (
	"encoding/json"
	"time"
)

type StrategyType string

const (
	StrategyBoolean      StrategyType = "boolean"
	StrategyPercentage   StrategyType = "percentage"
	StrategyRules        StrategyType = "rules"
	StrategyMultivariate StrategyType = "multivariate"
)

type AttributeType string

const (
	AttrUserID  AttributeType = "user_id"
	AttrEmail   AttributeType = "email"
	AttrCountry AttributeType = "country"
	AttrRole    AttributeType = "role"
	AttrTier    AttributeType = "tier"
	AttrCustom  AttributeType = "custom"
)

type RuleOperator string

const (
	OpEquals      RuleOperator = "equals"
	OpNotEquals   RuleOperator = "not_equals"
	OpContains    RuleOperator = "contains"
	OpNotContains RuleOperator = "not_contains"
	OpEndsWith    RuleOperator = "ends_with"
	OpInList      RuleOperator = "in_list"
	OpNotInList   RuleOperator = "not_in_list"
	OpGreaterThan RuleOperator = "greater_than"
	OpLessThan    RuleOperator = "less_than"
	OpRegex       RuleOperator = "regex"
)

type RuleAction string

const (
	ActionForceEnabled  RuleAction = "force_enabled"
	ActionForceDisabled RuleAction = "force_disabled"
	ActionServeVariant  RuleAction = "serve_variant"
)

type TargetingRule struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Attribute    AttributeType `json:"attribute"`
	CustomKey    string        `json:"customKey,omitempty"`
	Operator     RuleOperator  `json:"operator"`
	Values       []string      `json:"values"`
	Action       RuleAction    `json:"action"`
	ServeVariant string        `json:"serveVariant,omitempty"`
}

type FlagVariant struct {
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Value       interface{} `json:"value"`
	Weight      float64     `json:"weight"`
	Description string      `json:"description,omitempty"`
}

type EnvironmentConfig struct {
	Enabled          bool            `json:"enabled"`
	Strategy         StrategyType    `json:"strategy"`
	Percentage       float64         `json:"percentage"`
	Rules            []TargetingRule `json:"rules"`
	Variants         []FlagVariant   `json:"variants"`
	DefaultVariant   string          `json:"defaultVariant,omitempty"`
	OffVariant       string          `json:"offVariant,omitempty"`
	RequiresApproval bool            `json:"requiresApproval,omitempty"`
}

type FeatureFlag struct {
	ID            string                            `json:"id"`
	ProjectID     string                            `json:"projectId,omitempty"`
	ConfigVersion uint64                            `json:"configVersion,omitempty"`
	Key           string                            `json:"key"`
	Name          string                            `json:"name"`
	Description   string                            `json:"description"`
	Type          string                            `json:"type"` // boolean, multivariate, json
	Tags          []string                          `json:"tags"`
	Environments  map[Environment]EnvironmentConfig `json:"environments"`
	CreatedAt     time.Time                         `json:"createdAt"`
	UpdatedAt     time.Time                         `json:"updatedAt"`
}

func (f FeatureFlag) EnvConfig(env string) EnvironmentConfig {
	if cfg, ok := f.Environments[Environment(env)]; ok {
		return cfg
	}
	return EnvironmentConfig{}
}

func (f FeatureFlag) ProdConfig() EnvironmentConfig {
	return f.EnvConfig("production")
}

func (f FeatureFlag) IsProdEnabled() bool {
	return f.ProdConfig().Enabled
}

func (f FeatureFlag) ProdPercentage() float64 {
	return f.ProdConfig().Percentage
}

func (f FeatureFlag) ProdStrategy() string {
	strat := f.ProdConfig().Strategy
	if strat == "" {
		return "boolean"
	}
	return string(strat)
}

func (f FeatureFlag) ProdRulesCount() int {
	return len(f.ProdConfig().Rules)
}

func (f FeatureFlag) EnvironmentsJSON() string {
	b, err := json.Marshal(f.Environments)
	if err != nil {
		return "{}"
	}
	return string(b)
}

type EvaluationContext struct {
	UserID      string                 `json:"user_id,omitempty"`
	Email       string                 `json:"email,omitempty"`
	Country     string                 `json:"country,omitempty"`
	Role        string                 `json:"role,omitempty"`
	Tier        string                 `json:"tier,omitempty"`
	Environment Environment            `json:"environment,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}

type EvaluationReason string

const (
	ReasonKillSwitchDisabled EvaluationReason = "KILL_SWITCH_DISABLED"
	ReasonEnvDisabled        EvaluationReason = "ENV_DISABLED"
	ReasonTargetingRuleMatch EvaluationReason = "TARGETING_RULE_MATCH"
	ReasonPercentageBucket   EvaluationReason = "PERCENTAGE_ROLLOUT_BUCKET"
	ReasonPercentageExcluded EvaluationReason = "PERCENTAGE_ROLLOUT_EXCLUDED"
	ReasonMultivariateBucket EvaluationReason = "MULTIVARIATE_BUCKET"
	ReasonDefaultEnabled     EvaluationReason = "DEFAULT_ENABLED"
	ReasonDefaultOff         EvaluationReason = "DEFAULT_OFF"
	ReasonFlagNotFound       EvaluationReason = "FLAG_NOT_FOUND"
)

type EvaluationResult struct {
	FlagKey             string           `json:"flag_key"`
	Enabled             bool             `json:"enabled"`
	Variant             string           `json:"variant"`
	Value               interface{}      `json:"value"`
	Reason              EvaluationReason `json:"reason"`
	MatchedRuleID       string           `json:"matched_rule_id,omitempty"`
	MatchedRuleName     string           `json:"matched_rule_name,omitempty"`
	BucketVal           *float64         `json:"bucket_val,omitempty"`
	BucketThreshold     *float64         `json:"bucket_threshold,omitempty"`
	HashRaw             string           `json:"hash_raw,omitempty"`
	EvaluationLatencyNs int64            `json:"evaluation_latency_ns"`
	EvaluationLatencyUs float64          `json:"evaluation_latency_us"`
}

type BatchEvaluationResponse struct {
	Results         map[string]EvaluationResult `json:"results"`
	TotalFlags      int                         `json:"total_flags"`
	Environment     Environment                 `json:"environment"`
	TotalDurationUs float64                     `json:"total_duration_us"`
	EvaluatedAt     time.Time                   `json:"evaluated_at"`
}

type AuditLogEntry struct {
	ID          string      `json:"id"`
	ProjectID   string      `json:"projectId,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
	Actor       string      `json:"actor"`
	Action      string      `json:"action"`
	FlagKey     string      `json:"flagKey"`
	Environment Environment `json:"environment"`
	Details     string      `json:"details"`
}

type BenchmarkMetrics struct {
	Iterations      int      `json:"iterations"`
	TotalDurationMs float64  `json:"totalDurationMs"`
	OpsPerSec       int64    `json:"opsPerSec"`
	P50Ns           int64    `json:"p50Ns"`
	P90Ns           int64    `json:"p90Ns"`
	P99Ns           int64    `json:"p99Ns"`
	P999Ns          int64    `json:"p999Ns"`
	MinNs           int64    `json:"minNs"`
	MaxNs           int64    `json:"maxNs"`
	AvgNs           int64    `json:"avgNs"`
	HashBuckets     [100]int `json:"hashBuckets"`
}

// DeepCopy returns a fully independent clone of FeatureFlag to guarantee memory isolation.
func (f FeatureFlag) DeepCopy() FeatureFlag {
	clone := f
	if f.Tags != nil {
		clone.Tags = make([]string, len(f.Tags))
		copy(clone.Tags, f.Tags)
	}
	if f.Environments != nil {
		clone.Environments = make(map[Environment]EnvironmentConfig, len(f.Environments))
		for k, v := range f.Environments {
			clone.Environments[k] = v.DeepCopy()
		}
	}
	return clone
}

// DeepCopy returns a fully independent clone of EnvironmentConfig.
func (c EnvironmentConfig) DeepCopy() EnvironmentConfig {
	clone := c
	if c.Rules != nil {
		clone.Rules = make([]TargetingRule, len(c.Rules))
		for i, r := range c.Rules {
			clone.Rules[i] = r.DeepCopy()
		}
	}
	if c.Variants != nil {
		clone.Variants = make([]FlagVariant, len(c.Variants))
		copy(clone.Variants, c.Variants)
	}
	return clone
}

// DeepCopy returns a fully independent clone of TargetingRule.
func (r TargetingRule) DeepCopy() TargetingRule {
	clone := r
	if r.Values != nil {
		clone.Values = make([]string, len(r.Values))
		copy(clone.Values, r.Values)
	}
	return clone
}
