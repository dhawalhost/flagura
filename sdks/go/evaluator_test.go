package flagura

import (
	"testing"
)

func TestEngineEvaluation_TableDriven(t *testing.T) {
	flag := FeatureFlag{
		Key:  "checkout-gate",
		Name: "Checkout Gate",
		Type: FlagTypeBoolean,
		Variants: []Variant{
			{Key: "control", Value: "control_val", Weight: 50},
			{Key: "treatment", Value: "treatment_val", Weight: 50},
		},
		Environments: map[Environment]EnvironmentConfig{
			EnvProduction: {
				Enabled:  true,
				Strategy: StrategyRules,
				Rules: []Rule{
					{
						ID:      "rule-enterprise",
						Enabled: true,
						Variant: "treatment",
						Value:   "treatment_val",
						Conditions: []RuleCondition{
							{Attribute: "tier", Operator: "EQUALS", Value: "enterprise"},
							{Attribute: "country", Operator: "IN", Value: "US,CA,UK"},
						},
					},
					{
						ID:      "rule-regex-email",
						Enabled: true,
						Variant: "treatment",
						Conditions: []RuleCondition{
							{Attribute: "email", Operator: "REGEX", Value: `.*@flagura\.dev$`},
						},
					},
					{
						ID:      "rule-numeric-gt",
						Enabled: true,
						Conditions: []RuleCondition{
							{Attribute: "age", Operator: ">", Value: "21"},
						},
					},
				},
				DefaultVariant: "control",
				DefaultValue:   "control_val",
			},
			EnvStaging: {
				Enabled:  false,
				Strategy: StrategyBoolean,
			},
		},
	}

	tests := []struct {
		name        string
		flag        FeatureFlag
		ctx         Context
		wantEnabled bool
		wantVariant string
		wantReason  string
	}{
		{
			name: "Enterprise rule match",
			flag: flag,
			ctx: Context{
				Tier:        "enterprise",
				Country:     "US",
				Environment: EnvProduction,
			},
			wantEnabled: true,
			wantVariant: "treatment",
		},
		{
			name: "Regex email rule match",
			flag: flag,
			ctx: Context{
				Email:       "alice@flagura.dev",
				Environment: EnvProduction,
			},
			wantEnabled: true,
			wantVariant: "treatment",
		},
		{
			name: "Numeric comparison > 21 rule match",
			flag: flag,
			ctx: Context{
				Custom:      map[string]interface{}{"age": 25},
				Environment: EnvProduction,
			},
			wantEnabled: true,
		},
		{
			name: "Rules fallback to default",
			flag: flag,
			ctx: Context{
				Tier:        "free",
				Country:     "FR",
				Email:       "bob@other.com",
				Environment: EnvProduction,
			},
			wantEnabled: false,
			wantVariant: "control",
		},
		{
			name: "Disabled environment",
			flag: flag,
			ctx: Context{
				Environment: EnvStaging,
			},
			wantEnabled: false,
			wantReason:  "FLAG_DISABLED",
		},
		{
			name: "Non-existent environment",
			flag: flag,
			ctx: Context{
				Environment: Environment("non_existent_env"),
			},
			wantEnabled: false,
			wantReason:  "ENVIRONMENT_NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Evaluate(tt.flag, tt.ctx)
			if res.Enabled != tt.wantEnabled {
				t.Errorf("Evaluate() Enabled = %v, want %v", res.Enabled, tt.wantEnabled)
			}
			if tt.wantVariant != "" && res.Variant != tt.wantVariant {
				t.Errorf("Evaluate() Variant = %v, want %v", res.Variant, tt.wantVariant)
			}
			if tt.wantReason != "" && res.Reason != tt.wantReason {
				t.Errorf("Evaluate() Reason = %v, want %v", res.Reason, tt.wantReason)
			}
			if res.EvaluationLatencyNs < 0 {
				t.Errorf("expected positive latency, got %d", res.EvaluationLatencyNs)
			}
		})
	}
}

func TestEngineConditions_Operators(t *testing.T) {
	ctx := Context{
		UserID:  "usr_999",
		Email:   "dev@flagura.dev",
		Role:    "admin",
		Tier:    "premium",
		Country: "US",
		Custom: map[string]interface{}{
			"score": 85.5,
			"plan":  "gold",
		},
	}

	operators := []struct {
		cond RuleCondition
		want bool
	}{
		{cond: RuleCondition{Attribute: "role", Operator: "EQUALS", Value: "admin"}, want: true},
		{cond: RuleCondition{Attribute: "role", Operator: "NOT_EQUALS", Value: "viewer"}, want: true},
		{cond: RuleCondition{Attribute: "email", Operator: "CONTAINS", Value: "flagura"}, want: true},
		{cond: RuleCondition{Attribute: "email", Operator: "NOT_CONTAINS", Value: "gmail"}, want: true},
		{cond: RuleCondition{Attribute: "email", Operator: "STARTS_WITH", Value: "dev"}, want: true},
		{cond: RuleCondition{Attribute: "email", Operator: "ENDS_WITH", Value: ".dev"}, want: true},
		{cond: RuleCondition{Attribute: "plan", Operator: "IN", Value: "silver,gold,platinum"}, want: true},
		{cond: RuleCondition{Attribute: "plan", Operator: "NOT_IN", Value: "bronze,basic"}, want: true},
		{cond: RuleCondition{Attribute: "score", Operator: "GREATER_THAN", Value: "50"}, want: true},
		{cond: RuleCondition{Attribute: "score", Operator: "LESS_THAN", Value: "100"}, want: true},
		{cond: RuleCondition{Attribute: "unknown_attr", Operator: "EQUALS", Value: "x"}, want: false},
	}

	for _, tt := range operators {
		t.Run(tt.cond.Operator+"_"+tt.cond.Attribute, func(t *testing.T) {
			got := EvaluateCondition(tt.cond, ctx)
			if got != tt.want {
				t.Errorf("EvaluateCondition(%+v) = %v, want %v", tt.cond, got, tt.want)
			}
		})
	}
}

func TestEngineMultivariateAndPercentage(t *testing.T) {
	// Percentage Flag
	pctFlag := FeatureFlag{
		Key:  "pct-flag",
		Type: FlagTypeBoolean,
		Environments: map[Environment]EnvironmentConfig{
			EnvProduction: {
				Enabled:    true,
				Strategy:   StrategyPercentage,
				Percentage: 100,
			},
		},
	}
	resPct := Evaluate(pctFlag, Context{UserID: "u_any"})
	if !resPct.Enabled {
		t.Fatalf("expected 100%% percentage flag to be enabled")
	}

	// Multivariate Flag
	multiFlag := FeatureFlag{
		Key:  "multi-flag",
		Type: FlagTypeString,
		Variants: []Variant{
			{Key: "v_all", Value: "all_val", Weight: 100},
		},
		Environments: map[Environment]EnvironmentConfig{
			EnvProduction: {
				Enabled:  true,
				Strategy: StrategyMultivariate,
			},
		},
	}
	resMulti := Evaluate(multiFlag, Context{UserID: "u_any"})
	if !resMulti.Enabled || resMulti.Variant != "v_all" {
		t.Fatalf("expected multivariate match v_all, got: %+v", resMulti)
	}
}
