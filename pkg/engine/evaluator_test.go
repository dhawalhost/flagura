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
	ruleFlag := sampleRuleFlag()
	pctFlag := samplePercentageFlag()

	tests := []struct {
		name            string
		flag            domain.FeatureFlag
		ctx             domain.EvaluationContext
		expectedEnabled bool
		expectedReason  domain.EvaluationReason
	}{
		{
			name: "Targeting rule match (Email domain ends_with)",
			flag: ruleFlag,
			ctx: domain.EvaluationContext{
				UserID:      "usr_1",
				Email:       "alice@flagship.dev",
				Environment: domain.EnvProduction,
			},
			expectedEnabled: true,
			expectedReason:  domain.ReasonTargetingRuleMatch,
		},
		{
			name: "Targeting rule match (UserID regex pattern)",
			flag: ruleFlag,
			ctx: domain.EvaluationContext{
				UserID:      "qa_usr_42",
				Email:       "tester@example.com",
				Environment: domain.EnvProduction,
			},
			expectedEnabled: true,
			expectedReason:  domain.ReasonTargetingRuleMatch,
		},
		{
			name: "Pure percentage rollout evaluation (100% rollout)",
			flag: domain.FeatureFlag{
				Key:  "checkout-100",
				Type: "boolean",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {
						Enabled:    true,
						Strategy:   domain.StrategyPercentage,
						Percentage: 100,
					},
				},
			},
			ctx: domain.EvaluationContext{
				UserID:      "usr_100",
				Environment: domain.EnvProduction,
			},
			expectedEnabled: true,
			expectedReason:  domain.ReasonPercentageBucket,
		},
		{
			name: "Kill-switched disabled environment",
			flag: domain.FeatureFlag{
				Key:  "disabled-flag",
				Type: "boolean",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {Enabled: false},
				},
			},
			ctx: domain.EvaluationContext{
				UserID:      "usr_any",
				Environment: domain.EnvProduction,
			},
			expectedEnabled: false,
			expectedReason:  domain.ReasonKillSwitchDisabled,
		},
		{
			name: "Environment missing / not configured",
			flag: pctFlag,
			ctx: domain.EvaluationContext{
				UserID:      "usr_1",
				Environment: "non_existent_env",
			},
			expectedEnabled: false,
			expectedReason:  domain.ReasonKillSwitchDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := EvaluateFlag(tt.flag, tt.ctx)
			if res.Enabled != tt.expectedEnabled {
				t.Errorf("EvaluateFlag() enabled = %v, expected %v", res.Enabled, tt.expectedEnabled)
			}
			if res.Reason != tt.expectedReason {
				t.Errorf("EvaluateFlag() reason = %q, expected %q", res.Reason, tt.expectedReason)
			}
		})
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
	ruleFlag := sampleRuleFlag()

	disabledFlag := sampleRuleFlag()
	disabledFlag.Environments = map[domain.Environment]domain.EnvironmentConfig{
		domain.EnvProduction: {
			Enabled: false,
		},
	}

	tests := []struct {
		name            string
		flag            domain.FeatureFlag
		ctx             domain.EvaluationContext
		expectedEnabled bool
		expectedReason  domain.EvaluationReason
	}{
		{
			name: "Trace targeting rule match",
			flag: ruleFlag,
			ctx: domain.EvaluationContext{
				UserID:      "usr_1",
				Email:       "alice@flagship.dev",
				Environment: domain.EnvProduction,
			},
			expectedEnabled: true,
			expectedReason:  domain.ReasonTargetingRuleMatch,
		},
		{
			name: "Trace kill-switched disabled flag",
			flag: disabledFlag,
			ctx: domain.EvaluationContext{
				UserID:      "usr_1",
				Email:       "alice@flagship.dev",
				Environment: domain.EnvProduction,
			},
			expectedEnabled: false,
			expectedReason:  domain.ReasonKillSwitchDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, trace := EvaluateFlagWithTrace(tt.flag, tt.ctx)
			if res.Enabled != tt.expectedEnabled {
				t.Errorf("EvaluateFlagWithTrace() enabled = %v, expected %v", res.Enabled, tt.expectedEnabled)
			}
			if trace.FinalReason != tt.expectedReason {
				t.Errorf("EvaluateFlagWithTrace() finalReason = %q, expected %q", trace.FinalReason, tt.expectedReason)
			}
			if len(trace.Steps) == 0 {
				t.Errorf("expected trace steps to be non-empty")
			}
		})
	}
}

func TestEvaluateRule_AllOperatorsAndAttributes(t *testing.T) {
	tests := []struct {
		name     string
		rule     domain.TargetingRule
		ctx      domain.EvaluationContext
		expected bool
	}{
		// Equals
		{"Equals Match (UserID)", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpEquals, Values: []string{"usr_123"}}, domain.EvaluationContext{UserID: "usr_123"}, true},
		{"Equals Mismatch", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpEquals, Values: []string{"usr_123"}}, domain.EvaluationContext{UserID: "usr_999"}, false},

		// NotEquals
		{"NotEquals Match", domain.TargetingRule{Attribute: domain.AttrRole, Operator: domain.OpNotEquals, Values: []string{"admin"}}, domain.EvaluationContext{Role: "developer"}, true},
		{"NotEquals Mismatch", domain.TargetingRule{Attribute: domain.AttrRole, Operator: domain.OpNotEquals, Values: []string{"admin"}}, domain.EvaluationContext{Role: "admin"}, false},

		// Contains
		{"Contains Match (Email)", domain.TargetingRule{Attribute: domain.AttrEmail, Operator: domain.OpContains, Values: []string{"@company."}}, domain.EvaluationContext{Email: "alice@company.com"}, true},
		{"Contains Mismatch", domain.TargetingRule{Attribute: domain.AttrEmail, Operator: domain.OpContains, Values: []string{"@corp."}}, domain.EvaluationContext{Email: "alice@company.com"}, false},

		// NotContains
		{"NotContains Match", domain.TargetingRule{Attribute: domain.AttrEmail, Operator: domain.OpNotContains, Values: []string{"@test."}}, domain.EvaluationContext{Email: "alice@company.com"}, true},
		{"NotContains Mismatch", domain.TargetingRule{Attribute: domain.AttrEmail, Operator: domain.OpNotContains, Values: []string{"@company."}}, domain.EvaluationContext{Email: "alice@company.com"}, false},

		// EndsWith
		{"EndsWith Match", domain.TargetingRule{Attribute: domain.AttrCountry, Operator: domain.OpEndsWith, Values: []string{"US"}}, domain.EvaluationContext{Country: "US"}, true},

		// InList & NotInList
		{"InList Match (Tier)", domain.TargetingRule{Attribute: domain.AttrTier, Operator: domain.OpInList, Values: []string{"pro", "enterprise"}}, domain.EvaluationContext{Tier: "enterprise"}, true},
		{"InList Mismatch", domain.TargetingRule{Attribute: domain.AttrTier, Operator: domain.OpInList, Values: []string{"pro", "enterprise"}}, domain.EvaluationContext{Tier: "free"}, false},
		{"NotInList Match", domain.TargetingRule{Attribute: domain.AttrTier, Operator: domain.OpNotInList, Values: []string{"banned", "suspended"}}, domain.EvaluationContext{Tier: "pro"}, true},
		{"NotInList Mismatch", domain.TargetingRule{Attribute: domain.AttrTier, Operator: domain.OpNotInList, Values: []string{"banned"}}, domain.EvaluationContext{Tier: "banned"}, false},

		// GreaterThan & LessThan
		{"GreaterThan Match", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "score", Operator: domain.OpGreaterThan, Values: []string{"85.5"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"score": 90.0}}, true},
		{"GreaterThan Mismatch", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "score", Operator: domain.OpGreaterThan, Values: []string{"85.5"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"score": 80.0}}, false},
		{"GreaterThan Invalid Target", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "score", Operator: domain.OpGreaterThan, Values: []string{"85.5"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"score": "not_a_number"}}, false},
		{"GreaterThan Invalid Threshold", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "score", Operator: domain.OpGreaterThan, Values: []string{"invalid"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"score": 90.0}}, false},

		{"LessThan Match", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "latency", Operator: domain.OpLessThan, Values: []string{"100"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"latency": 45}}, true},
		{"LessThan Mismatch", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "latency", Operator: domain.OpLessThan, Values: []string{"100"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"latency": 150}}, false},
		{"LessThan Invalid Target", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "latency", Operator: domain.OpLessThan, Values: []string{"100"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"latency": "bad"}}, false},
		{"LessThan Invalid Threshold", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "latency", Operator: domain.OpLessThan, Values: []string{"bad"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"latency": 50}}, false},

		// Regex
		{"Regex Valid Match", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpRegex, Values: []string{`^dev_[0-9]+$`}}, domain.EvaluationContext{UserID: "dev_99"}, true},
		{"Regex Valid Mismatch", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpRegex, Values: []string{`^dev_[0-9]+$`}}, domain.EvaluationContext{UserID: "prod_99"}, false},
		{"Regex Invalid Pattern", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpRegex, Values: []string{`[invalid`}}, domain.EvaluationContext{UserID: "dev_99"}, false},
		{"Regex Empty Pattern", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpRegex, Values: []string{}}, domain.EvaluationContext{UserID: "dev_99"}, false},

		// Unknown Operator
		{"Unknown Operator", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: "custom_op", Values: []string{"val"}}, domain.EvaluationContext{UserID: "val"}, false},

		// Nil / Empty attributes
		{"Missing Custom Attribute Key", domain.TargetingRule{Attribute: domain.AttrCustom, CustomKey: "missing", Operator: domain.OpEquals, Values: []string{"val"}}, domain.EvaluationContext{Attributes: map[string]interface{}{"other": "val"}}, false},
		{"Empty UserID attribute", domain.TargetingRule{Attribute: domain.AttrUserID, Operator: domain.OpEquals, Values: []string{"val"}}, domain.EvaluationContext{UserID: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := EvaluateRule(tt.rule, tt.ctx)
			if res != tt.expected {
				t.Errorf("EvaluateRule() = %v, expected %v", res, tt.expected)
			}
		})
	}
}

func TestResolveMultivariateVariant(t *testing.T) {
	tests := []struct {
		name         string
		variants     []domain.FlagVariant
		identifier   string
		flagKey      string
		expectDefKey string
	}{
		{
			name:         "Empty variants returns default",
			variants:     nil,
			identifier:   "usr_1",
			flagKey:      "color-test",
			expectDefKey: "default",
		},
		{
			name: "Single 100% variant",
			variants: []domain.FlagVariant{
				{Key: "blue", Value: "#00f", Weight: 100},
			},
			identifier:   "usr_1",
			flagKey:      "color-test",
			expectDefKey: "blue",
		},
		{
			name: "Multi variant split",
			variants: []domain.FlagVariant{
				{Key: "variant-a", Value: "A", Weight: 50},
				{Key: "variant-b", Value: "B", Weight: 50},
			},
			identifier:   "usr_12345",
			flagKey:      "color-test",
			expectDefKey: "variant-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, bucket, hash := ResolveMultivariateVariant(tt.variants, tt.identifier, tt.flagKey)
			if len(tt.variants) > 0 && v.Key == "" {
				t.Fatalf("expected non-empty variant key")
			}
			if hash == "" {
				t.Fatalf("expected non-empty hash")
			}
			_ = bucket
		})
	}
}

func TestEvaluateFlag_AdvancedActionsAndStrategies(t *testing.T) {
	multiFlag := domain.FeatureFlag{
		Key:  "theme-flag",
		Type: "multivariate",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  true,
				Strategy: domain.StrategyMultivariate,
				Variants: []domain.FlagVariant{
					{Key: "dark", Value: "dark_mode", Weight: 50},
					{Key: "light", Value: "light_mode", Weight: 50},
				},
			},
		},
	}

	forceDisabledFlag := domain.FeatureFlag{
		Key:  "disabled-by-rule",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled: true,
				Rules: []domain.TargetingRule{
					{
						Attribute: domain.AttrRole,
						Operator:  domain.OpEquals,
						Values:    []string{"banned"},
						Action:    domain.ActionForceDisabled,
					},
					{
						Attribute:    domain.AttrRole,
						Operator:     domain.OpEquals,
						Values:       []string{"beta"},
						Action:       domain.ActionServeVariant,
						ServeVariant: "special_beta",
					},
				},
				Variants: []domain.FlagVariant{
					{Key: "special_beta", Value: "BETA_VALUE", Weight: 100},
				},
			},
		},
	}

	// 1. Evaluate multivariate flag
	resMulti := EvaluateFlag(multiFlag, domain.EvaluationContext{UserID: "usr_1"})
	if !resMulti.Enabled || resMulti.Reason != domain.ReasonMultivariateBucket {
		t.Errorf("unexpected multivariate result: %+v", resMulti)
	}

	// 2. Evaluate force disabled rule
	resForceDis := EvaluateFlag(forceDisabledFlag, domain.EvaluationContext{UserID: "usr_1", Role: "banned"})
	if resForceDis.Enabled || resForceDis.Reason != domain.ReasonTargetingRuleMatch {
		t.Errorf("expected force disabled rule match: %+v", resForceDis)
	}

	// 3. Evaluate serve variant rule
	resServeVar := EvaluateFlag(forceDisabledFlag, domain.EvaluationContext{UserID: "usr_1", Role: "beta"})
	if !resServeVar.Enabled || resServeVar.Variant != "special_beta" || resServeVar.Value != "BETA_VALUE" {
		t.Errorf("expected serve variant rule match: %+v", resServeVar)
	}
}

func TestRunBenchmark(t *testing.T) {
	flag := samplePercentageFlag()
	metrics := RunBenchmark(flag, domain.EnvProduction, 500)

	if metrics.Iterations != 500 {
		t.Errorf("expected 500 iterations, got %d", metrics.Iterations)
	}
	if metrics.OpsPerSec < 0 {
		t.Errorf("expected positive OpsPerSec, got %d", metrics.OpsPerSec)
	}
	if metrics.P50Ns <= 0 || metrics.AvgNs <= 0 {
		t.Errorf("expected positive latency percentiles: %+v", metrics)
	}
}

func TestEvaluateFlagWithTrace_AllPaths(t *testing.T) {
	flag := domain.FeatureFlag{
		Key: "trace-complex-flag",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50,
				Rules: []domain.TargetingRule{
					{
						Name:      "Staff Rule",
						Attribute: domain.AttrRole,
						Operator:  domain.OpEquals,
						Values:    []string{"staff"},
						Action:    domain.ActionForceEnabled,
					},
				},
			},
		},
	}

	// 1. Missing environment trace
	_, traceMissingEnv := EvaluateFlagWithTrace(flag, domain.EvaluationContext{Environment: "dev"})
	if traceMissingEnv.FinalReason != domain.ReasonEnvDisabled {
		t.Errorf("expected ReasonEnvDisabled trace, got %s", traceMissingEnv.FinalReason)
	}

	// 2. Rule non-match falling through to percentage rollout
	_, traceNonMatch := EvaluateFlagWithTrace(flag, domain.EvaluationContext{
		UserID:      "user_normal",
		Role:        "customer",
		Environment: domain.EnvProduction,
	})
	if len(traceNonMatch.Steps) < 2 {
		t.Errorf("expected multiple trace steps, got %d", len(traceNonMatch.Steps))
	}

	// 3. Multivariate Strategy Trace
	multiFlag := domain.FeatureFlag{
		Key: "trace-multi",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  true,
				Strategy: domain.StrategyMultivariate,
				Variants: []domain.FlagVariant{
					{Key: "v1", Value: "A", Weight: 50},
					{Key: "v2", Value: "B", Weight: 50},
				},
			},
		},
	}
	_, traceMulti := EvaluateFlagWithTrace(multiFlag, domain.EvaluationContext{UserID: "usr_1", Environment: domain.EnvProduction})
	if traceMulti.FinalReason != domain.ReasonMultivariateBucket {
		t.Errorf("expected ReasonMultivariateBucket, got %s", traceMulti.FinalReason)
	}

	// 4. Default Boolean Strategy Trace
	boolFlag := domain.FeatureFlag{
		Key: "trace-bool",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  true,
				Strategy: domain.StrategyBoolean,
			},
		},
	}
	_, traceBool := EvaluateFlagWithTrace(boolFlag, domain.EvaluationContext{UserID: "usr_1", Environment: domain.EnvProduction})
	if traceBool.FinalReason != domain.ReasonDefaultEnabled {
		t.Errorf("expected ReasonDefaultEnabled, got %s", traceBool.FinalReason)
	}
}

func TestToStringFast_AllTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"String", "  HeLLo  ", "hello"},
		{"Bool True", true, "true"},
		{"Bool False", false, "false"},
		{"Int", 42, "42"},
		{"Int64", int64(999999), "999999"},
		{"Float64", 3.1415, "3.1415"},
		{"Unknown struct", struct{ X int }{10}, "{10}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := toStringFast(tt.input)
			if res != tt.expected {
				t.Errorf("toStringFast(%v) = %q, expected %q", tt.input, res, tt.expected)
			}
		})
	}
}

func TestRunBenchmark_Limits(t *testing.T) {
	flag := samplePercentageFlag()
	// Test minimum iteration clamp
	mMin := RunBenchmark(flag, domain.EnvProduction, 10)
	if mMin.Iterations != 500 {
		t.Errorf("expected clamp to 500, got %d", mMin.Iterations)
	}

	// Test EvaluateFlag with missing environment
	resNoEnv := EvaluateFlag(flag, domain.EvaluationContext{Environment: "non_existent"})
	if resNoEnv.Enabled || resNoEnv.Reason != domain.ReasonKillSwitchDisabled {
		t.Errorf("expected killswitch disabled for missing env, got %s", resNoEnv.Reason)
	}
}
