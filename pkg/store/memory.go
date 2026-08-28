package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dhawalhost/flagura/pkg/domain"
)

type MemoryStore struct {
	mu        sync.RWMutex
	flags     []domain.FeatureFlag
	auditLogs []domain.AuditLogEntry
	users     map[string]domain.User    // indexed by email and id
	sessions  map[string]domain.Session // indexed by token
}

func getSeedFlags() []domain.FeatureFlag {
	now := time.Now().UTC()
	return []domain.FeatureFlag{
		{
			ID:          "flag_01_ai_search",
			Key:         "ai-smart-search",
			Name:        "AI Smart Search & Query Rewrite",
			Description: "Autonomous semantic embedding and LLM query expansion engine before DB search query execution.",
			Type:        "boolean",
			Tags:        []string{"core-ai", "search", "performance"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        true,
					Strategy:       domain.StrategyRules,
					Percentage:     35,
					DefaultVariant: "treatment",
					OffVariant:     "control",
					Rules: []domain.TargetingRule{
						{
							ID:        "rule_staff_domain",
							Name:      "Staff & Internal Testing Domain",
							Attribute: domain.AttrEmail,
							Operator:  domain.OpEndsWith,
							Values:    []string{"@flagura.dev", "@company.com", "@google.com"},
							Action:    domain.ActionForceEnabled,
						},
						{
							ID:        "rule_enterprise_tier",
							Name:      "Enterprise VIP Customers",
							Attribute: domain.AttrTier,
							Operator:  domain.OpEquals,
							Values:    []string{"enterprise"},
							Action:    domain.ActionForceEnabled,
						},
					},
					Variants: []domain.FlagVariant{
						{Key: "control", Name: "Legacy Keyword Search", Value: false, Weight: 65},
						{Key: "treatment", Name: "AI Hybrid Search", Value: true, Weight: 35},
					},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyPercentage,
					Percentage:     100,
					DefaultVariant: "treatment",
					OffVariant:     "control",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-9 * 24 * time.Hour),
			UpdatedAt: now.Add(-1 * 24 * time.Hour),
		},
		{
			ID:          "flag_02_checkout",
			Key:         "new-checkout-flow",
			Name:        "Stripe Instant 1-Click Checkout Flow",
			Description: "Zero-friction checkout drawer with Apple Pay & Google Pay express buttons.",
			Type:        "multivariate",
			Tags:        []string{"billing", "growth", "experiments"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        true,
					Strategy:       domain.StrategyMultivariate,
					Percentage:     50,
					DefaultVariant: "standard_v1",
					OffVariant:     "legacy_checkout",
					Rules: []domain.TargetingRule{
						{
							ID:           "rule_qa_whitelists",
							Name:         "QA & Finance Internal Testers",
							Attribute:    domain.AttrRole,
							Operator:     domain.OpInList,
							Values:       []string{"admin", "qa_engineer", "finance_lead"},
							Action:       domain.ActionServeVariant,
							ServeVariant: "instant_1click",
						},
					},
					Variants: []domain.FlagVariant{
						{Key: "legacy_checkout", Name: "Legacy 3-Step Checkout", Value: map[string]interface{}{"layout": "multi_step", "express_pay": false}, Weight: 20},
						{Key: "instant_1click", Name: "Instant 1-Click Drawer", Value: map[string]interface{}{"layout": "drawer", "express_pay": true, "autofill": true}, Weight: 40},
						{Key: "accordion_summary", Name: "Sticky Accordion Summary", Value: map[string]interface{}{"layout": "accordion", "express_pay": true, "discount_nudge": true}, Weight: 40},
					},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyMultivariate,
					Percentage:     100,
					DefaultVariant: "instant_1click",
					OffVariant:     "legacy_checkout",
					Rules:          []domain.TargetingRule{},
					Variants: []domain.FlagVariant{
						{Key: "instant_1click", Name: "Instant 1-Click Drawer", Value: map[string]interface{}{"layout": "drawer", "express_pay": true}, Weight: 100},
					},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-14 * 24 * time.Hour),
			UpdatedAt: now.Add(-2 * 24 * time.Hour),
		},
		{
			ID:          "flag_03_crypto",
			Key:         "crypto-web3-settlement",
			Name:        "USDC & Web3 Instant Settlement Gateway",
			Description: "Direct blockchain on-ramp and USDC stablecoin treasury settlement rail.",
			Type:        "boolean",
			Tags:        []string{"payments", "web3", "experimental"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        false,
					Strategy:       domain.StrategyBoolean,
					Percentage:     0,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyPercentage,
					Percentage:     50,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-12 * 24 * time.Hour),
			UpdatedAt: now.Add(-4 * 24 * time.Hour),
		},
		{
			ID:          "flag_04_dark_mode",
			Key:         "dark-mode-obsidian",
			Name:        "Obsidian Cyber-Glass Dark UI Theme",
			Description: "High-contrast neon cyberpunk dark theme with glassmorphism surface layers.",
			Type:        "boolean",
			Tags:        []string{"ui", "design-system", "core"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        true,
					Strategy:       domain.StrategyPercentage,
					Percentage:     100,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyPercentage,
					Percentage:     100,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "on",
					OffVariant:     "off",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-23 * 24 * time.Hour),
			UpdatedAt: now.Add(-6 * 24 * time.Hour),
		},
		{
			ID:          "flag_05_rate_limiter",
			Key:         "rate-limiter-v2",
			Name:        "Dynamic Edge Token Bucket Rate Limiter",
			Description: "JSON-driven dynamic concurrency, RPM limits and burst capacity configuration per tenant.",
			Type:        "json",
			Tags:        []string{"infra", "security", "edge"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        true,
					Strategy:       domain.StrategyRules,
					Percentage:     100,
					DefaultVariant: "standard_tier",
					OffVariant:     "legacy_limits",
					Rules: []domain.TargetingRule{
						{
							ID:           "rule_enterprise_burst",
							Name:         "Enterprise Ultra High Throughput",
							Attribute:    domain.AttrTier,
							Operator:     domain.OpEquals,
							Values:       []string{"enterprise"},
							Action:       domain.ActionServeVariant,
							ServeVariant: "enterprise_tier",
						},
					},
					Variants: []domain.FlagVariant{
						{
							Key:    "standard_tier",
							Name:   "Standard Tier Quota",
							Value:  map[string]interface{}{"max_rpm": 6000, "burst_capacity": 10000, "sliding_window_sec": 60},
							Weight: 50,
						},
						{
							Key:    "enterprise_tier",
							Name:   "Enterprise Tier Quota",
							Value:  map[string]interface{}{"max_rpm": 60000, "burst_capacity": 100000, "sliding_window_sec": 60},
							Weight: 50,
						},
					},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "standard_tier",
					OffVariant:     "legacy_limits",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "dev_unlimited",
					OffVariant:     "legacy_limits",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-8 * 24 * time.Hour),
			UpdatedAt: now.Add(-1 * 24 * time.Hour),
		},
		{
			ID:          "flag_06_collab",
			Key:         "realtime-collaboration-engine",
			Name:        "CRDT WebSocket Collaborative Canvas",
			Description: "Multi-cursor live conflict-free document and canvas editing.",
			Type:        "boolean",
			Tags:        []string{"realtime", "canvas", "beta"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        true,
					Strategy:       domain.StrategyRules,
					Percentage:     20,
					DefaultVariant: "treatment",
					OffVariant:     "control",
					Rules: []domain.TargetingRule{
						{
							ID:        "rule_geo_whitelist",
							Name:      "North America & UK Region Access",
							Attribute: domain.AttrCountry,
							Operator:  domain.OpInList,
							Values:    []string{"US", "CA", "GB"},
							Action:    domain.ActionForceEnabled,
						},
						{
							ID:        "rule_beta_testers",
							Name:      "Registered Beta Program Members",
							Attribute: domain.AttrRole,
							Operator:  domain.OpEquals,
							Values:    []string{"beta_tester"},
							Action:    domain.ActionForceEnabled,
						},
					},
					Variants: []domain.FlagVariant{
						{Key: "control", Name: "Standard Single Player", Value: false, Weight: 80},
						{Key: "treatment", Name: "Realtime Multiplayer CRDT", Value: true, Weight: 20},
					},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "treatment",
					OffVariant:     "control",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyBoolean,
					Percentage:     100,
					DefaultVariant: "treatment",
					OffVariant:     "control",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-6 * 24 * time.Hour),
			UpdatedAt: now.Add(-1 * 24 * time.Hour),
		},
	}
}

func getSeedAuditLogs() []domain.AuditLogEntry {
	now := time.Now().UTC()
	return []domain.AuditLogEntry{
		{
			ID:          "log_01",
			Timestamp:   now.Add(-1 * time.Hour),
			Actor:       "dhawal@flagura.dev",
			Action:      "ROLLOUT_CHANGED",
			FlagKey:     "ai-smart-search",
			Environment: domain.EnvProduction,
			Details:     "Increased percentage rollout from 20% to 35% after latency verification (< 4.8µs).",
		},
		{
			ID:          "log_02",
			Timestamp:   now.Add(-2 * time.Hour),
			Actor:       "dhawal@flagura.dev",
			Action:      "RULE_MODIFIED",
			FlagKey:     "new-checkout-flow",
			Environment: domain.EnvProduction,
			Details:     "Added QA whitelist rule for instant 1-click checkout variant.",
		},
		{
			ID:          "log_03",
			Timestamp:   now.Add(-24 * time.Hour),
			Actor:       "security-admin@flagura.dev",
			Action:      "KILL_SWITCH_TOGGLED",
			FlagKey:     "crypto-web3-settlement",
			Environment: domain.EnvProduction,
			Details:     "Engaged kill switch for production environment pending smart contract audit sign-off.",
		},
	}
}

func NewMemoryStore() *MemoryStore {
	store := &MemoryStore{
		flags:     getSeedFlags(),
		auditLogs: getSeedAuditLogs(),
		users:     make(map[string]domain.User),
		sessions:  make(map[string]domain.Session),
	}

	// Seed a default administrator with hashed password for immediate local access
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	admin := domain.User{
		ID:           "usr_admin_default",
		Email:        "dhawal@flagura.dev",
		PasswordHash: string(hash),
		Name:         "Dhawal Dyavanpalli",
		Role:         domain.RoleAdmin,
		AvatarURL:    "",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	store.users[admin.Email] = admin
	store.users[admin.ID] = admin

	return store
}

func (s *MemoryStore) DriverName() string {
	return "In-Memory Edge Store"
}

func (s *MemoryStore) Ping(ctx context.Context) error {
	return nil
}

func (s *MemoryStore) CreateUser(ctx context.Context, user domain.User) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.Email]; exists {
		return nil, fmt.Errorf("user with email %s already exists", user.Email)
	}

	if user.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		user.ID = fmt.Sprintf("usr_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	s.users[user.Email] = user
	s.users[user.ID] = user

	return &user, nil
}

func (s *MemoryStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[email]
	if !exists {
		return nil, fmt.Errorf("user not found with email: %s", email)
	}
	return &user, nil
}

func (s *MemoryStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found with ID: %s", id)
	}
	return &user, nil
}

func (s *MemoryStore) CreateSession(ctx context.Context, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.Token] = session
	return nil
}

func (s *MemoryStore) GetSession(ctx context.Context, token string) (*domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, exists := s.sessions[token]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}
	if sess.IsExpired() {
		return nil, fmt.Errorf("session expired")
	}

	// Attach user
	if user, ok := s.users[sess.UserID]; ok {
		sess.User = &user
	}

	return &sess, nil
}

func (s *MemoryStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, token)
	return nil
}

func (s *MemoryStore) ListFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.FeatureFlag, len(s.flags))
	copy(result, s.flags)
	return result, nil
}

func (s *MemoryStore) GetFlag(ctx context.Context, keyOrID string) (*domain.FeatureFlag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, f := range s.flags {
		if f.Key == keyOrID || f.ID == keyOrID {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("flag not found: %s", keyOrID)
}

func (s *MemoryStore) SaveFlag(ctx context.Context, flag domain.FeatureFlag, actor string) (*domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if actor == "" {
		actor = "developer@flagura.dev"
	}

	now := time.Now().UTC()
	found := false
	var log domain.AuditLogEntry

	for i, f := range s.flags {
		if f.ID == flag.ID || f.Key == flag.Key {
			flag.ID = f.ID
			flag.CreatedAt = f.CreatedAt
			flag.UpdatedAt = now
			s.flags[i] = flag
			found = true

			log = domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   now,
				Actor:       actor,
				Action:      "FLAG_UPDATED",
				FlagKey:     flag.Key,
				Environment: "all",
				Details:     fmt.Sprintf("Updated configuration and rules for '%s'.", flag.Key),
			}
			break
		}
	}

	if !found {
		if flag.ID == "" {
			b := make([]byte, 4)
			_, _ = rand.Read(b)
			flag.ID = fmt.Sprintf("flag_%d_%s", time.Now().Unix(), hex.EncodeToString(b))
		}
		flag.CreatedAt = now
		flag.UpdatedAt = now
		s.flags = append([]domain.FeatureFlag{flag}, s.flags...)

		log = domain.AuditLogEntry{
			ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
			Timestamp:   now,
			Actor:       actor,
			Action:      "FLAG_CREATED",
			FlagKey:     flag.Key,
			Environment: "all",
			Details:     fmt.Sprintf("Created new %s flag '%s' [%s].", flag.Type, flag.Name, flag.Key),
		}
	}

	s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
	return &log, nil
}

func (s *MemoryStore) DeleteFlag(ctx context.Context, keyOrID string, actor string) (*domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if actor == "" {
		actor = "admin@flagura.dev"
	}

	for i, f := range s.flags {
		if f.Key == keyOrID || f.ID == keyOrID {
			deletedKey := f.Key
			s.flags = append(s.flags[:i], s.flags[i+1:]...)

			log := domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   time.Now().UTC(),
				Actor:       actor,
				Action:      "FLAG_DELETED",
				FlagKey:     deletedKey,
				Environment: "all",
				Details:     fmt.Sprintf("Permanently removed feature flag '%s'.", deletedKey),
			}
			s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
			return &log, nil
		}
	}

	return nil, fmt.Errorf("flag not found: %s", keyOrID)
}

func (s *MemoryStore) ToggleFlag(ctx context.Context, keyOrID string, env domain.Environment, enabled *bool, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if actor == "" {
		actor = "admin@flagura.dev"
	}
	if env == "" {
		env = domain.EnvProduction
	}

	for i, f := range s.flags {
		if f.Key == keyOrID || f.ID == keyOrID {
			cfg := f.Environments[env]
			if enabled != nil {
				cfg.Enabled = *enabled
			} else {
				cfg.Enabled = !cfg.Enabled
			}
			f.Environments[env] = cfg
			f.UpdatedAt = time.Now().UTC()
			s.flags[i] = f

			statusText := "Disabled (Kill Switch)"
			if cfg.Enabled {
				statusText = "Enabled"
			}

			log := domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   time.Now().UTC(),
				Actor:       actor,
				Action:      "KILL_SWITCH_TOGGLED",
				FlagKey:     f.Key,
				Environment: env,
				Details:     fmt.Sprintf("%s flag for %s environment.", statusText, env),
			}
			s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
			return &s.flags[i], &log, nil
		}
	}

	return nil, nil, fmt.Errorf("flag not found: %s", keyOrID)
}

func (s *MemoryStore) UpdateRollout(ctx context.Context, keyOrID string, env domain.Environment, pct float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if actor == "" {
		actor = "engineer@flagura.dev"
	}
	if env == "" {
		env = domain.EnvProduction
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	for i, f := range s.flags {
		if f.Key == keyOrID || f.ID == keyOrID {
			cfg := f.Environments[env]
			oldPct := cfg.Percentage
			cfg.Percentage = pct
			if cfg.Strategy == domain.StrategyBoolean && pct < 100 {
				cfg.Strategy = domain.StrategyPercentage
			}
			f.Environments[env] = cfg
			f.UpdatedAt = time.Now().UTC()
			s.flags[i] = f

			log := domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   time.Now().UTC(),
				Actor:       actor,
				Action:      "ROLLOUT_CHANGED",
				FlagKey:     f.Key,
				Environment: env,
				Details:     fmt.Sprintf("Shifted percentage rollout from %.0f%% to %.0f%% in %s.", oldPct, pct, env),
			}
			s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
			return &s.flags[i], &log, nil
		}
	}

	return nil, nil, fmt.Errorf("flag not found: %s", keyOrID)
}

func (s *MemoryStore) ListAuditLogs(ctx context.Context, limit int) ([]domain.AuditLogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.auditLogs) {
		limit = len(s.auditLogs)
	}
	result := make([]domain.AuditLogEntry, limit)
	copy(result, s.auditLogs[:limit])
	return result, nil
}

func (s *MemoryStore) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.flags = getSeedFlags()
	s.auditLogs = getSeedAuditLogs()
	return nil
}
