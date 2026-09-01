package flagura

import "time"

// Environment represents deployment environments (e.g. production, staging, development).
type Environment string

const (
	EnvProduction  Environment = "production"
	EnvStaging     Environment = "staging"
	EnvDevelopment Environment = "development"
)

// Header constants for HTTP requests.
const (
	HeaderProjectID = "X-Project-ID"
	HeaderActor     = "X-Actor"
)

// Strategy represents the flag rollout strategy.
type Strategy string

const (
	StrategyBoolean      Strategy = "boolean"
	StrategyPercentage   Strategy = "percentage"
	StrategyRules        Strategy = "rules"
	StrategyMultivariate Strategy = "multivariate"
)

// FlagType represents the data type of a flag.
type FlagType string

const (
	FlagTypeBoolean FlagType = "boolean"
	FlagTypeString  FlagType = "string"
	FlagTypeNumber  FlagType = "number"
	FlagTypeJSON    FlagType = "json"
)

// Variant represents a specific option in a multivariate feature flag.
type Variant struct {
	Key         string      `json:"key"`
	Value       interface{} `json:"value"`
	Weight      int         `json:"weight"` // Out of 100 for multivariate distribution
	Description string      `json:"description,omitempty"`
}

// RuleCondition represents a rule predicate for targeted evaluation.
type RuleCondition struct {
	Attribute string      `json:"attribute"`
	Operator  string      `json:"operator"` // "EQUALS", "NOT_EQUALS", "IN", "NOT_IN", "CONTAINS", "GREATER_THAN", "LESS_THAN"
	Value     interface{} `json:"value"`
}

// Rule represents a conditional evaluation rule.
type Rule struct {
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	Conditions  []RuleCondition `json:"conditions"`
	Variant     string          `json:"variant,omitempty"`
	Value       interface{}     `json:"value,omitempty"`
	Enabled     bool            `json:"enabled"`
}

// EnvironmentConfig represents per-environment configuration of a flag.
type EnvironmentConfig struct {
	Enabled        bool        `json:"enabled"`
	Strategy       Strategy    `json:"strategy"`
	Percentage     int         `json:"percentage,omitempty"`
	Rules          []Rule      `json:"rules,omitempty"`
	DefaultVariant string      `json:"default_variant,omitempty"`
	DefaultValue   interface{} `json:"default_value,omitempty"`
}

// FeatureFlag represents a complete feature flag definition.
type FeatureFlag struct {
	ID           string                       `json:"id"`
	ProjectID    string                       `json:"project_id"`
	Key          string                       `json:"key"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Type         FlagType                     `json:"type"`
	Tags         []string                     `json:"tags,omitempty"`
	Variants     []Variant                    `json:"variants,omitempty"`
	Environments map[Environment]EnvironmentConfig `json:"environments"`
	CreatedAt    time.Time                    `json:"created_at"`
	UpdatedAt    time.Time                    `json:"updated_at"`
}

// Context represents user or request targeting context for flag evaluation.
type Context struct {
	UserID      string                 `json:"user_id"`
	Email       string                 `json:"email,omitempty"`
	Country     string                 `json:"country,omitempty"`
	Role        string                 `json:"role,omitempty"`
	Tier        string                 `json:"tier,omitempty"`
	Environment Environment            `json:"environment,omitempty"`
	Custom      map[string]interface{} `json:"custom,omitempty"`
}

// EvaluationResult represents the computed result of evaluating a feature flag.
type EvaluationResult struct {
	FlagKey             string      `json:"flag_key"`
	Enabled             bool        `json:"enabled"`
	Variant             string      `json:"variant"`
	Value               interface{} `json:"value"`
	Reason              string      `json:"reason"`
	Bucket              float64     `json:"bucket"`
	EvaluationLatencyNs int64       `json:"latency_ns"`
	EvaluationLatencyUs float64     `json:"latency_us"`
}

// BatchEvaluationResponse holds multiple flag evaluation results.
type BatchEvaluationResponse struct {
	Results map[string]EvaluationResult `json:"results"`
}
