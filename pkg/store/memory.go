package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// FlagSnapshot is an immutable, read-optimized in-memory snapshot of feature flags.
type FlagSnapshot struct {
	flagsMap  map[string]domain.FeatureFlag
	flagsList []domain.FeatureFlag
}

func newFlagSnapshot(flags []domain.FeatureFlag) *FlagSnapshot {
	snap := &FlagSnapshot{
		flagsMap:  make(map[string]domain.FeatureFlag, len(flags)*4),
		flagsList: make([]domain.FeatureFlag, len(flags)),
	}
	for i, f := range flags {
		fCopy := f.DeepCopy()
		if fCopy.ProjectID == "" {
			fCopy.ProjectID = DefaultProjectID
		}
		snap.flagsList[i] = fCopy
		snap.flagsMap[fCopy.Key] = fCopy
		if fCopy.ID != "" {
			snap.flagsMap[fCopy.ID] = fCopy
		}
		snap.flagsMap[fCopy.ProjectID+":"+fCopy.Key] = fCopy
		if fCopy.ID != "" {
			snap.flagsMap[fCopy.ProjectID+":"+fCopy.ID] = fCopy
		}
	}
	return snap
}

type MemoryStore struct {
	flagsSnapshot       atomic.Pointer[FlagSnapshot]
	writeMu             sync.Mutex   // serializes writes and atomic snapshot updates
	mu                  sync.RWMutex // protects mutable tables
	orgs                map[string]domain.Organization
	projects            map[string]domain.Project
	orgMembers          map[string]domain.OrgMember
	orgInvitations      map[string]domain.OrgInvitation
	auditLogs           []domain.AuditLogEntry
	events              []domain.ExperimentEvent
	users               map[string]domain.User // indexed by email and id
	sessions            map[string]domain.Session // indexed by token
	changeRequests      map[string]domain.ChangeRequest
	apiKeys             map[string]domain.APIKey // indexed by key ID
	apiKeysByHash       map[string]string        // hash -> key ID
	passwordResetTokens map[string]domain.PasswordResetToken
}

func NewMemoryStore() *MemoryStore {
	store := &MemoryStore{
		orgs:                make(map[string]domain.Organization),
		projects:            make(map[string]domain.Project),
		orgMembers:          make(map[string]domain.OrgMember),
		orgInvitations:      make(map[string]domain.OrgInvitation),
		auditLogs:           []domain.AuditLogEntry{},
		events:              make([]domain.ExperimentEvent, 0),
		users:               make(map[string]domain.User),
		sessions:            make(map[string]domain.Session),
		changeRequests:      make(map[string]domain.ChangeRequest),
		apiKeys:             make(map[string]domain.APIKey),
		apiKeysByHash:       make(map[string]string),
		passwordResetTokens: make(map[string]domain.PasswordResetToken),
	}

	initialSnap := newFlagSnapshot([]domain.FeatureFlag{})
	store.flagsSnapshot.Store(initialSnap)

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

func (s *MemoryStore) ListUsers(ctx context.Context) ([]domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var users []domain.User
	for _, u := range s.users {
		if !seen[u.ID] {
			seen[u.ID] = true
			uCopy := u
			uCopy.PasswordHash = ""
			users = append(users, uCopy)
		}
	}
	return users, nil
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

	if flagCopy.ProjectID == "" {
		flagCopy.ProjectID = DefaultProjectID
	}

	for i, f := range newList {
		fProj := f.ProjectID
		if fProj == "" {
			fProj = DefaultProjectID
		}
		if (f.ID == flagCopy.ID || f.Key == flagCopy.Key) && (fProj == flagCopy.ProjectID) {
			flagCopy.ID = f.ID
			flagCopy.CreatedAt = f.CreatedAt
			flagCopy.UpdatedAt = now
			newList[i] = flagCopy
			found = true

			log = domain.AuditLogEntry{
				ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
				ProjectID:   flagCopy.ProjectID,
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

	newSnap := newFlagSnapshot(nil)
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
		Details:     "Clean reset of store data.",
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

func (s *MemoryStore) CreateAPIKey(ctx context.Context, key domain.APIKey) (*domain.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		key.ID = fmt.Sprintf("key_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	if key.ProjectID == "" {
		key.ProjectID = DefaultProjectID
	}
	if key.Environment == "" {
		key.Environment = "production"
	}
	now := time.Now().UTC()
	key.CreatedAt = now
	key.Revoked = false

	s.apiKeys[key.ID] = key
	if key.KeyHash != "" {
		s.apiKeysByHash[key.KeyHash] = key.ID
	}

	return &key, nil
}

func (s *MemoryStore) ListAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]domain.APIKey, 0, len(s.apiKeys))
	for _, k := range s.apiKeys {
		// Redact raw key and key_hash on list
		kCopy := k
		kCopy.Key = ""
		kCopy.KeyHash = ""
		res = append(res, kCopy)
	}
	return res, nil
}

func (s *MemoryStore) GetAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, exists := s.apiKeysByHash[hash]
	if !exists {
		return nil, fmt.Errorf("api key not found")
	}

	key, exists := s.apiKeys[id]
	if !exists || key.Revoked {
		return nil, fmt.Errorf("api key revoked or not found")
	}

	now := time.Now().UTC()
	key.LastUsedAt = &now
	s.apiKeys[id] = key

	kCopy := key
	return &kCopy, nil
}

func (s *MemoryStore) RevokeAPIKey(ctx context.Context, id string, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, exists := s.apiKeys[id]
	if !exists {
		return fmt.Errorf("api key not found: %s", id)
	}

	key.Revoked = true
	s.apiKeys[id] = key
	delete(s.apiKeysByHash, key.KeyHash)

	audit := domain.AuditLogEntry{
		ID:          fmt.Sprintf("audit_%d", time.Now().UnixNano()),
		FlagKey:     "api-keys",
		Action:      "API_KEY_REVOKED",
		Environment: "all",
		Actor:       actor,
		Timestamp:   time.Now().UTC(),
		Details:     fmt.Sprintf("Revoked API Key '%s' (%s)", key.Name, key.ID),
	}
	s.auditLogs = append([]domain.AuditLogEntry{audit}, s.auditLogs...)

	return nil
}

func (s *MemoryStore) CreatePasswordResetToken(ctx context.Context, email string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[email]
	if !exists {
		return "", fmt.Errorf("user not found with email: %s", email)
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := fmt.Sprintf("flg_rst_%s", hex.EncodeToString(b))

	now := time.Now().UTC()
	s.passwordResetTokens[token] = domain.PasswordResetToken{
		Token:     token,
		Email:     user.Email,
		ExpiresAt: now.Add(ttl),
		Used:      false,
		CreatedAt: now,
	}

	return token, nil
}

func (s *MemoryStore) GetPasswordResetToken(ctx context.Context, token string) (*domain.PasswordResetToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.passwordResetTokens[token]
	if !exists {
		return nil, fmt.Errorf("invalid or expired password reset token")
	}

	tCopy := t
	return &tCopy, nil
}

func (s *MemoryStore) ResetPasswordWithToken(ctx context.Context, token string, newPasswordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.passwordResetTokens[token]
	if !exists {
		return fmt.Errorf("invalid or expired reset token")
	}
	if t.Used {
		return fmt.Errorf("reset token has already been used")
	}
	if t.IsExpired() {
		return fmt.Errorf("reset token has expired")
	}

	user, exists := s.users[t.Email]
	if !exists {
		return fmt.Errorf("user associated with token not found")
	}

	user.PasswordHash = newPasswordHash
	user.UpdatedAt = time.Now().UTC()
	s.users[user.Email] = user
	s.users[user.ID] = user

	// Mark token as used
	t.Used = true
	s.passwordResetTokens[token] = t

	// Invalidate existing sessions for this user for security
	for tokenKey, sess := range s.sessions {
		if sess.UserID == user.ID {
			delete(s.sessions, tokenKey)
		}
	}

	return nil
}

func (s *MemoryStore) CreateOrganization(ctx context.Context, org domain.Organization) (*domain.Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if org.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		org.ID = fmt.Sprintf("org_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	if org.Slug == "" {
		org.Slug = org.ID
	}
	if _, exists := s.orgs[org.ID]; exists {
		return nil, fmt.Errorf("organization already exists with ID: %s", org.ID)
	}
	if _, exists := s.orgs[org.Slug]; exists {
		return nil, fmt.Errorf("organization already exists with slug: %s", org.Slug)
	}
	now := time.Now().UTC()
	org.CreatedAt = now
	org.UpdatedAt = now

	s.orgs[org.ID] = org
	s.orgs[org.Slug] = org
	return &org, nil
}

func (s *MemoryStore) GetOrganization(ctx context.Context, idOrSlug string) (*domain.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	org, exists := s.orgs[idOrSlug]
	if !exists {
		return nil, fmt.Errorf("organization not found: %s", idOrSlug)
	}
	return &org, nil
}

func (s *MemoryStore) ListOrganizations(ctx context.Context) ([]domain.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var res []domain.Organization
	for _, org := range s.orgs {
		if !seen[org.ID] {
			seen[org.ID] = true
			res = append(res, org)
		}
	}
	return res, nil
}

func (s *MemoryStore) CreateProject(ctx context.Context, project domain.Project) (*domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if project.OrganizationID == "" {
		project.OrganizationID = DefaultOrgID
	}
	if project.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		project.ID = fmt.Sprintf("proj_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	if project.Slug == "" {
		project.Slug = project.ID
	}
	key := project.OrganizationID + ":" + project.Slug
	if _, exists := s.projects[project.ID]; exists {
		return nil, fmt.Errorf("project already exists with ID: %s", project.ID)
	}
	if _, exists := s.projects[key]; exists {
		return nil, fmt.Errorf("project already exists with slug '%s' in org '%s'", project.Slug, project.OrganizationID)
	}

	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now

	s.projects[project.ID] = project
	s.projects[key] = project
	s.projects[project.Slug] = project
	return &project, nil
}

func (s *MemoryStore) GetProject(ctx context.Context, idOrSlug string) (*domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	proj, exists := s.projects[idOrSlug]
	if !exists {
		return nil, fmt.Errorf("project not found: %s", idOrSlug)
	}
	return &proj, nil
}

func (s *MemoryStore) ListProjects(ctx context.Context, organizationID string) ([]domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var res []domain.Project
	for _, proj := range s.projects {
		if !seen[proj.ID] {
			if organizationID == "" || proj.OrganizationID == organizationID {
				seen[proj.ID] = true
				res = append(res, proj)
			}
		}
	}
	return res, nil
}

func (s *MemoryStore) ListFlagsByProject(ctx context.Context, projectID string) ([]domain.FeatureFlag, error) {
	if projectID == "" {
		projectID = DefaultProjectID
	}
	snap := s.flagsSnapshot.Load()
	if snap == nil {
		return nil, nil
	}
	var res []domain.FeatureFlag
	for _, f := range snap.flagsList {
		if f.ProjectID == projectID || (projectID == DefaultProjectID && f.ProjectID == "") {
			res = append(res, f.DeepCopy())
		}
	}
	return res, nil
}

func (s *MemoryStore) GetFlagByProject(ctx context.Context, projectID, keyOrID string) (*domain.FeatureFlag, error) {
	if projectID == "" {
		projectID = DefaultProjectID
	}
	snap := s.flagsSnapshot.Load()
	if snap == nil {
		return nil, fmt.Errorf("flag not found: %s", keyOrID)
	}
	if f, ok := snap.flagsMap[projectID+":"+keyOrID]; ok {
		clone := f.DeepCopy()
		return &clone, nil
	}
	if projectID == DefaultProjectID {
		if f, ok := snap.flagsMap[keyOrID]; ok {
			clone := f.DeepCopy()
			return &clone, nil
		}
	}
	return nil, fmt.Errorf("flag not found: %s", keyOrID)
}

func (s *MemoryStore) ListAuditLogsByProject(ctx context.Context, projectID string, limit int) ([]domain.AuditLogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if projectID == "" {
		projectID = DefaultProjectID
	}

	var res []domain.AuditLogEntry
	for _, entry := range s.auditLogs {
		if entry.ProjectID == projectID || (projectID == DefaultProjectID && entry.ProjectID == "") {
			res = append(res, entry)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (s *MemoryStore) ListChangeRequestsByProject(ctx context.Context, projectID string, status domain.ChangeRequestStatus) ([]domain.ChangeRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if projectID == "" {
		projectID = DefaultProjectID
	}

	var res []domain.ChangeRequest
	for _, cr := range s.changeRequests {
		if cr.ProjectID == projectID || (projectID == DefaultProjectID && cr.ProjectID == "") {
			if status == "" || cr.Status == status {
				res = append(res, cr)
			}
		}
	}
	return res, nil
}

func (s *MemoryStore) ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if projectID == "" {
		projectID = DefaultProjectID
	}

	var res []domain.APIKey
	for _, k := range s.apiKeys {
		if k.ProjectID == projectID || (projectID == DefaultProjectID && k.ProjectID == "") {
			kCopy := k
			kCopy.Key = ""
			kCopy.KeyHash = ""
			res = append(res, kCopy)
		}
	}
	return res, nil
}

func (s *MemoryStore) CreateOrgMember(ctx context.Context, member domain.OrgMember) (*domain.OrgMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if member.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		member.ID = fmt.Sprintf("mem_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	if member.CreatedAt.IsZero() {
		member.CreatedAt = time.Now().UTC()
	}
	if member.Role == "" {
		member.Role = "developer"
	}

	s.orgMembers[member.ID] = member
	s.orgMembers[member.OrganizationID+":"+member.UserID] = member
	return &member, nil
}

func (s *MemoryStore) ListOrgMembers(ctx context.Context, organizationID string) ([]domain.OrgMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var res []domain.OrgMember
	for _, m := range s.orgMembers {
		if m.OrganizationID == organizationID && !seen[m.ID] {
			seen[m.ID] = true
			res = append(res, m)
		}
	}
	return res, nil
}

func (s *MemoryStore) ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userOrgIDs := make(map[string]bool)
	for _, m := range s.orgMembers {
		if m.UserID == userID {
			userOrgIDs[m.OrganizationID] = true
		}
	}

	user, userExists := s.users[userID]
	if userExists && user.Role == domain.RoleAdmin {
		var allOrgs []domain.Organization
		seen := make(map[string]bool)
		for _, o := range s.orgs {
			if !seen[o.ID] {
				seen[o.ID] = true
				allOrgs = append(allOrgs, o)
			}
		}
		if len(allOrgs) > 0 {
			return allOrgs, nil
		}
	}

	seen := make(map[string]bool)
	var res []domain.Organization
	for _, o := range s.orgs {
		if (userOrgIDs[o.ID] || (userExists && strings.Contains(o.Slug, domain.Slugify(user.Name)))) && !seen[o.ID] {
			seen[o.ID] = true
			res = append(res, o)
		}
	}
	return res, nil
}

func (s *MemoryStore) CreateOrgInvitation(ctx context.Context, inv domain.OrgInvitation) (*domain.OrgInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if inv.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		inv.ID = fmt.Sprintf("inv_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	if inv.Token == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		inv.Token = "inv_" + hex.EncodeToString(b)
	}
	now := time.Now().UTC()
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}
	if inv.ExpiresAt.IsZero() {
		inv.ExpiresAt = now.Add(7 * 24 * time.Hour)
	}
	if inv.Role == "" {
		inv.Role = "developer"
	}

	s.orgInvitations[inv.Token] = inv
	s.orgInvitations[inv.ID] = inv
	return &inv, nil
}

func (s *MemoryStore) GetOrgInvitation(ctx context.Context, token string) (*domain.OrgInvitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, exists := s.orgInvitations[token]
	if !exists {
		return nil, fmt.Errorf("invitation not found")
	}
	if inv.IsExpired() {
		return nil, fmt.Errorf("invitation has expired")
	}
	invCopy := inv
	return &invCopy, nil
}

func (s *MemoryStore) AcceptOrgInvitation(ctx context.Context, token, userID string) (*domain.OrgMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inv, exists := s.orgInvitations[token]
	if !exists {
		return nil, fmt.Errorf("invitation not found")
	}
	if inv.IsExpired() {
		return nil, fmt.Errorf("invitation has expired")
	}
	if inv.IsAccepted() {
		return nil, fmt.Errorf("invitation already accepted")
	}

	now := time.Now().UTC()
	inv.AcceptedAt = &now
	s.orgInvitations[token] = inv
	s.orgInvitations[inv.ID] = inv

	b := make([]byte, 4)
	_, _ = rand.Read(b)
	member := domain.OrgMember{
		ID:             fmt.Sprintf("mem_%d_%s", now.UnixNano(), hex.EncodeToString(b)),
		OrganizationID: inv.OrganizationID,
		UserID:         userID,
		Role:           inv.Role,
		CreatedAt:      now,
	}

	s.orgMembers[member.ID] = member
	s.orgMembers[member.OrganizationID+":"+member.UserID] = member
	return &member, nil
}

func (s *MemoryStore) ListOrgInvitations(ctx context.Context, organizationID string) ([]domain.OrgInvitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var res []domain.OrgInvitation
	for _, inv := range s.orgInvitations {
		if inv.OrganizationID == organizationID && !seen[inv.ID] {
			seen[inv.ID] = true
			res = append(res, inv)
		}
	}
	return res, nil
}

