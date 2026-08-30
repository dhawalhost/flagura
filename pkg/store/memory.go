package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// FlagSnapshot is an immutable, read-optimized in-memory snapshot of feature flags.
type FlagSnapshot struct {
	flagsMap  map[string]domain.FeatureFlag
	flagsList []domain.FeatureFlag
}

func newFlagSnapshot(flags []domain.FeatureFlag) *FlagSnapshot {
	snap := &FlagSnapshot{
		flagsMap:  make(map[string]domain.FeatureFlag, len(flags)*2),
		flagsList: make([]domain.FeatureFlag, len(flags)),
	}
	for i, f := range flags {
		fCopy := f.DeepCopy()
		snap.flagsList[i] = fCopy
		snap.flagsMap[f.Key] = fCopy
		if f.ID != "" {
			snap.flagsMap[f.ID] = fCopy
		}
	}
	return snap
}

type MemoryStore struct {
	flagsSnapshot  atomic.Pointer[FlagSnapshot]
	writeMu        sync.Mutex // serializes writes and atomic snapshot updates
	mu             sync.RWMutex // protects mutable tables: users, sessions, auditLogs, events, changeRequests
	auditLogs      []domain.AuditLogEntry
	events         []domain.ExperimentEvent
	users          map[string]domain.User    // indexed by email and id
	sessions       map[string]domain.Session // indexed by token
	changeRequests map[string]domain.ChangeRequest
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
					OffVariant:     "standard_v1",
					Rules: []domain.TargetingRule{
						{
							ID:        "rule_geo_us_eu",
							Name:      "Tier-1 Countries (US, DE, GB)",
							Attribute: domain.AttrCountry,
							Operator:  domain.OpInList,
							Values:    []string{"US", "DE", "GB"},
							Action:    domain.ActionForceEnabled,
						},
					},
					Variants: []domain.FlagVariant{
						{Key: "standard_v1", Name: "Multi-step Standard Form", Value: "legacy_form", Weight: 50},
						{Key: "express_1click", Name: "Express 1-Click Apple/Google Pay", Value: "express_drawer", Weight: 50},
					},
				},
				domain.EnvStaging: {
					Enabled:        true,
					Strategy:       domain.StrategyPercentage,
					Percentage:     100,
					DefaultVariant: "express_1click",
					OffVariant:     "standard_v1",
					Rules:          []domain.TargetingRule{},
					Variants: []domain.FlagVariant{
						{Key: "standard_v1", Name: "Multi-step Standard Form", Value: "legacy_form", Weight: 50},
						{Key: "express_1click", Name: "Express 1-Click Apple/Google Pay", Value: "express_drawer", Weight: 50},
					},
				},
				domain.EnvDevelopment: {
					Enabled:        true,
					Strategy:       domain.StrategyPercentage,
					Percentage:     100,
					DefaultVariant: "express_1click",
					OffVariant:     "standard_v1",
					Rules:          []domain.TargetingRule{},
					Variants:       []domain.FlagVariant{},
				},
			},
			CreatedAt: now.Add(-7 * 24 * time.Hour),
			UpdatedAt: now.Add(-2 * 24 * time.Hour),
		},
		{
			ID:          "flag_03_dark_mode",
			Key:         "beta-dark-theme",
			Name:        "OLED Midnight Obsidian Dark Theme",
			Description: "High-contrast dark mode with custom emerald neon accents and glassmorphic panels.",
			Type:        "boolean",
			Tags:        []string{"ui", "theme", "frontend"},
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
					Strategy:       domain.StrategyBoolean,
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
			CreatedAt: now.Add(-14 * 24 * time.Hour),
			UpdatedAt: now.Add(-3 * 24 * time.Hour),
		},
		{
			ID:          "flag_04_crypto_settlement",
			Key:         "crypto-web3-settlement",
			Name:        "Solana & USDC Treasury Settlement",
			Description: "Automated sub-second merchant invoice settlement on Solana Mainnet-Beta.",
			Type:        "boolean",
			Tags:        []string{"web3", "crypto", "treasury"},
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
					Strategy:       domain.StrategyBoolean,
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
			CreatedAt: now.Add(-20 * 24 * time.Hour),
			UpdatedAt: now.Add(-4 * 24 * time.Hour),
		},
		{
			ID:          "flag_05_multiplayer_canvas",
			Key:         "multiplayer-live-collab",
			Name:        "Real-Time Multiplayer Canvas CRDT Engine",
			Description: "Collaborative canvas state syncing with conflict-free replicated data types over WebSockets.",
			Type:        "multivariate",
			Tags:        []string{"collab", "websocket", "experimental"},
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:        true,
					Strategy:       domain.StrategyRules,
					Percentage:     20,
					DefaultVariant: "treatment",
					OffVariant:     "control",
					Rules: []domain.TargetingRule{
						{
							ID:        "rule_na_region",
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
		auditLogs:      getSeedAuditLogs(),
		users:          make(map[string]domain.User),
		sessions:       make(map[string]domain.Session),
		changeRequests: make(map[string]domain.ChangeRequest),
	}

	initialSnap := newFlagSnapshot(getSeedFlags())
	store.flagsSnapshot.Store(initialSnap)

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

// ListFlags retrieves all flags from the current immutable snapshot.
func (s *MemoryStore) ListFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	snap := s.flagsSnapshot.Load()
	if snap == nil {
		return nil, nil
	}
	result := make([]domain.FeatureFlag, len(snap.flagsList))
	for i, f := range snap.flagsList {
		result[i] = f.DeepCopy()
	}
	return result, nil
}

// GetFlag looks up a flag by key or ID from the current immutable snapshot.
func (s *MemoryStore) GetFlag(ctx context.Context, keyOrID string) (*domain.FeatureFlag, error) {
	snap := s.flagsSnapshot.Load()
	if snap == nil {
		return nil, fmt.Errorf("flag not found: %s", keyOrID)
	}
	flag, ok := snap.flagsMap[keyOrID]
	if !ok {
		return nil, fmt.Errorf("flag not found: %s", keyOrID)
	}
	clone := flag.DeepCopy()
	return &clone, nil
}

func (s *MemoryStore) SaveFlag(ctx context.Context, flag domain.FeatureFlag, actor string) (*domain.AuditLogEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if actor == "" {
		actor = "developer@flagura.dev"
	}

	currentSnap := s.flagsSnapshot.Load()
	var currentList []domain.FeatureFlag
	if currentSnap != nil {
		currentList = currentSnap.flagsList
	}

	newList := make([]domain.FeatureFlag, len(currentList))
	copy(newList, currentList)

	now := time.Now().UTC()
	found := false
	flagCopy := flag.DeepCopy()
	var log domain.AuditLogEntry

	for i, f := range newList {
		if f.ID == flagCopy.ID || f.Key == flagCopy.Key {
			flagCopy.ID = f.ID
			flagCopy.CreatedAt = f.CreatedAt
			flagCopy.UpdatedAt = now
			newList[i] = flagCopy
			found = true

			log = domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   now,
				Actor:       actor,
				Action:      "FLAG_UPDATED",
				FlagKey:     flagCopy.Key,
				Environment: "all",
				Details:     fmt.Sprintf("Updated configuration and rules for '%s'.", flagCopy.Key),
			}
			break
		}
	}

	if !found {
		if flagCopy.ID == "" {
			b := make([]byte, 4)
			_, _ = rand.Read(b)
			flagCopy.ID = fmt.Sprintf("flag_%d_%s", time.Now().Unix(), hex.EncodeToString(b))
		}
		flagCopy.CreatedAt = now
		flagCopy.UpdatedAt = now
		newList = append([]domain.FeatureFlag{flagCopy}, newList...)

		log = domain.AuditLogEntry{
			ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
			Timestamp:   now,
			Actor:       actor,
			Action:      "FLAG_CREATED",
			FlagKey:     flagCopy.Key,
			Environment: "all",
			Details:     fmt.Sprintf("Created new %s flag '%s' [%s].", flagCopy.Type, flagCopy.Name, flagCopy.Key),
		}
	}

	newSnap := newFlagSnapshot(newList)
	s.flagsSnapshot.Store(newSnap)

	s.mu.Lock()
	s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
	s.mu.Unlock()

	return &log, nil
}

func (s *MemoryStore) DeleteFlag(ctx context.Context, keyOrID string, actor string) (*domain.AuditLogEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if actor == "" {
		actor = "admin@flagura.dev"
	}

	currentSnap := s.flagsSnapshot.Load()
	if currentSnap == nil {
		return nil, fmt.Errorf("flag not found: %s", keyOrID)
	}

	var newList []domain.FeatureFlag
	var deletedKey string
	found := false

	for _, f := range currentSnap.flagsList {
		if f.Key == keyOrID || f.ID == keyOrID {
			deletedKey = f.Key
			found = true
			continue
		}
		newList = append(newList, f)
	}

	if !found {
		return nil, fmt.Errorf("flag not found: %s", keyOrID)
	}

	log := domain.AuditLogEntry{
		ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:   time.Now().UTC(),
		Actor:       actor,
		Action:      "FLAG_DELETED",
		FlagKey:     deletedKey,
		Environment: "all",
		Details:     fmt.Sprintf("Permanently removed feature flag '%s'.", deletedKey),
	}

	newSnap := newFlagSnapshot(newList)
	s.flagsSnapshot.Store(newSnap)

	s.mu.Lock()
	s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
	s.mu.Unlock()

	return &log, nil
}

func (s *MemoryStore) ToggleFlag(ctx context.Context, keyOrID string, env domain.Environment, enabled *bool, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if actor == "" {
		actor = "admin@flagura.dev"
	}
	if env == "" {
		env = domain.EnvProduction
	}

	currentSnap := s.flagsSnapshot.Load()
	if currentSnap == nil {
		return nil, nil, fmt.Errorf("flag not found: %s", keyOrID)
	}

	newList := make([]domain.FeatureFlag, len(currentSnap.flagsList))
	var updatedFlag domain.FeatureFlag
	var log domain.AuditLogEntry
	found := false

	for i, f := range currentSnap.flagsList {
		flagCopy := f.DeepCopy()
		if flagCopy.Key == keyOrID || flagCopy.ID == keyOrID {
			cfg := flagCopy.Environments[env]
			if enabled != nil {
				cfg.Enabled = *enabled
			} else {
				cfg.Enabled = !cfg.Enabled
			}
			flagCopy.Environments[env] = cfg
			flagCopy.UpdatedAt = time.Now().UTC()
			newList[i] = flagCopy
			updatedFlag = flagCopy
			found = true

			statusText := "Disabled (Kill Switch)"
			if cfg.Enabled {
				statusText = "Enabled"
			}

			log = domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   time.Now().UTC(),
				Actor:       actor,
				Action:      "KILL_SWITCH_TOGGLED",
				FlagKey:     flagCopy.Key,
				Environment: env,
				Details:     fmt.Sprintf("%s flag for %s environment.", statusText, env),
			}
		} else {
			newList[i] = flagCopy
		}
	}

	if !found {
		return nil, nil, fmt.Errorf("flag not found: %s", keyOrID)
	}

	newSnap := newFlagSnapshot(newList)
	s.flagsSnapshot.Store(newSnap)

	s.mu.Lock()
	s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
	s.mu.Unlock()

	return &updatedFlag, &log, nil
}

func (s *MemoryStore) UpdateRollout(ctx context.Context, keyOrID string, env domain.Environment, pct float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

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

	currentSnap := s.flagsSnapshot.Load()
	if currentSnap == nil {
		return nil, nil, fmt.Errorf("flag not found: %s", keyOrID)
	}

	newList := make([]domain.FeatureFlag, len(currentSnap.flagsList))
	var updatedFlag domain.FeatureFlag
	var log domain.AuditLogEntry
	found := false

	for i, f := range currentSnap.flagsList {
		flagCopy := f.DeepCopy()
		if flagCopy.Key == keyOrID || flagCopy.ID == keyOrID {
			cfg := flagCopy.Environments[env]
			oldPct := cfg.Percentage
			cfg.Percentage = pct
			if cfg.Strategy == domain.StrategyBoolean && pct < 100 {
				cfg.Strategy = domain.StrategyPercentage
			}
			flagCopy.Environments[env] = cfg
			flagCopy.UpdatedAt = time.Now().UTC()
			newList[i] = flagCopy
			updatedFlag = flagCopy
			found = true

			log = domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				Timestamp:   time.Now().UTC(),
				Actor:       actor,
				Action:      "ROLLOUT_UPDATED",
				FlagKey:     flagCopy.Key,
				Environment: env,
				Details:     fmt.Sprintf("Updated percentage rollout from %.0f%% to %.0f%% for %s.", oldPct, pct, env),
			}
		} else {
			newList[i] = flagCopy
		}
	}

	if !found {
		return nil, nil, fmt.Errorf("flag not found: %s", keyOrID)
	}

	newSnap := newFlagSnapshot(newList)
	s.flagsSnapshot.Store(newSnap)

	s.mu.Lock()
	s.auditLogs = append([]domain.AuditLogEntry{log}, s.auditLogs...)
	s.mu.Unlock()

	return &updatedFlag, &log, nil
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	newSnap := newFlagSnapshot(getSeedFlags())
	s.flagsSnapshot.Store(newSnap)
	s.events = nil

	// Append-only audit record of the reset action
	resetLog := domain.AuditLogEntry{
		ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:   time.Now().UTC(),
		Actor:       "admin@flagura.dev",
		Action:      "DATABASE_RESET",
		FlagKey:     "all",
		Environment: "all",
		Details:     "Reset feature flags to default seed template.",
	}
	s.auditLogs = append([]domain.AuditLogEntry{resetLog}, s.auditLogs...)
	return nil
}

func (s *MemoryStore) RecordExperimentEvents(ctx context.Context, events []domain.ExperimentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ev := range events {
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now().UTC()
		}
		s.events = append(s.events, ev)
	}

	// Cap memory storage to last 100,000 events to prevent unbounded growth
	if len(s.events) > 100000 {
		s.events = s.events[len(s.events)-100000:]
	}

	return nil
}

func (s *MemoryStore) GetExperimentEvents(ctx context.Context, flagKey string, limit int) ([]domain.ExperimentEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []domain.ExperimentEvent
	for i := len(s.events) - 1; i >= 0; i-- {
		ev := s.events[i]
		if flagKey == "" || ev.FlagKey == flagKey {
			matched = append(matched, ev)
			if limit > 0 && len(matched) >= limit {
				break
			}
		}
	}
	return matched, nil
}

func (s *MemoryStore) CreateChangeRequest(ctx context.Context, cr domain.ChangeRequest) (*domain.ChangeRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if cr.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		cr.ID = fmt.Sprintf("cr_%d_%s", now.UnixNano(), hex.EncodeToString(b))
	}
	cr.Status = domain.ChangeRequestStatusPending
	cr.CreatedAt = now

	s.changeRequests[cr.ID] = cr
	return &cr, nil
}

func (s *MemoryStore) GetChangeRequest(ctx context.Context, id string) (*domain.ChangeRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cr, ok := s.changeRequests[id]
	if !ok {
		return nil, fmt.Errorf("change request not found: %s", id)
	}
	return &cr, nil
}

func (s *MemoryStore) ListChangeRequests(ctx context.Context, status domain.ChangeRequestStatus) ([]domain.ChangeRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []domain.ChangeRequest
	for _, cr := range s.changeRequests {
		if status == "" || cr.Status == status {
			result = append(result, cr)
		}
	}
	return result, nil
}

func (s *MemoryStore) ReviewChangeRequest(ctx context.Context, id, reviewerID, reviewerEmail, reviewerName string, approved bool, comments string) (*domain.ChangeRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cr, ok := s.changeRequests[id]
	if !ok {
		return nil, fmt.Errorf("change request not found: %s", id)
	}

	if err := cr.Review(reviewerID, reviewerEmail, reviewerName, approved, comments); err != nil {
		return nil, err
	}

	s.changeRequests[id] = cr
	return &cr, nil
}

func (s *MemoryStore) ApplyChangeRequest(ctx context.Context, id string, actor string) (*domain.FeatureFlag, *domain.ChangeRequest, *domain.AuditLogEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.Lock()
	cr, ok := s.changeRequests[id]
	if !ok {
		s.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("change request not found: %s", id)
	}

	if cr.Status != domain.ChangeRequestStatusApproved {
		s.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("cannot apply change request with status '%s' (must be APPROVED)", cr.Status)
	}

	currentSnap := s.flagsSnapshot.Load()
	if currentSnap == nil {
		s.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("target flag not found: %s", cr.FlagKey)
	}

	newList := make([]domain.FeatureFlag, len(currentSnap.flagsList))
	var updatedFlag domain.FeatureFlag
	found := false

	for i, f := range currentSnap.flagsList {
		flagCopy := f.DeepCopy()
		if flagCopy.Key == cr.FlagKey || flagCopy.ID == cr.FlagKey {
			if flagCopy.Environments == nil {
				flagCopy.Environments = make(map[domain.Environment]domain.EnvironmentConfig)
			}
			flagCopy.Environments[cr.Environment] = cr.ProposedConfig.DeepCopy()
			flagCopy.UpdatedAt = time.Now().UTC()
			newList[i] = flagCopy
			updatedFlag = flagCopy
			found = true
		} else {
			newList[i] = flagCopy
		}
	}

	if !found {
		s.mu.Unlock()
		return nil, nil, nil, fmt.Errorf("target flag not found: %s", cr.FlagKey)
	}

	now := time.Now().UTC()
	cr.Status = domain.ChangeRequestStatusApplied
	cr.AppliedAt = &now
	s.changeRequests[id] = cr

	audit := domain.AuditLogEntry{
		ID:          fmt.Sprintf("audit_%d", now.UnixNano()),
		FlagKey:     updatedFlag.Key,
		Action:      "APPLY_CHANGE_REQUEST",
		Environment: cr.Environment,
		Actor:       actor,
		Timestamp:   now,
		Details:     fmt.Sprintf("Applied ChangeRequest %s by reviewer %s for %s", id, cr.ReviewerEmail, cr.Environment),
	}
	s.auditLogs = append([]domain.AuditLogEntry{audit}, s.auditLogs...)
	s.mu.Unlock()

	newSnap := newFlagSnapshot(newList)
	s.flagsSnapshot.Store(newSnap)

	return &updatedFlag, &cr, &audit, nil
}
