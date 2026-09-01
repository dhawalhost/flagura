package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dhawalhost/flagura/pkg/domain"
)

const SessionCookieName = domain.CookieSessionName

func generateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *Server) setProjectCookie(w http.ResponseWriter, r *http.Request, projectID string, expiresAt time.Time) {
	isSecure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		isSecure = true
	}
	if os.Getenv("ENVIRONMENT") == string(domain.EnvProduction) || os.Getenv("SECURE_COOKIE") == "true" {
		isSecure = true
	}

	// #nosec G124 -- active project selection cookie configured with SameSite and dynamic TLS
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieProjectName,
		Value:    projectID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	isSecure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		isSecure = true
	}
	if os.Getenv("ENVIRONMENT") == string(domain.EnvProduction) || os.Getenv("SECURE_COOKIE") == "true" {
		isSecure = true
	}

	// #nosec G124 -- dynamic secure flag based on TLS and environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieSessionName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		isSecure = true
	}
	if os.Getenv("ENVIRONMENT") == string(domain.EnvProduction) || os.Getenv("SECURE_COOKIE") == "true" {
		isSecure = true
	}

	// #nosec G124 -- dynamic secure flag based on TLS and environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieSessionName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) getUserFromRequest(r *http.Request) (*domain.User, error) {
	var token string

	// 1. Check HTTP Cookie
	if cookie, err := r.Cookie(domain.CookieSessionName); err == nil && cookie.Value != "" {
		token = cookie.Value
	}

	// 2. Fallback to Authorization Header (Bearer token)
	if token == "" {
		authHeader := r.Header.Get(domain.HeaderAuthorization)
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	// 3. Fallback to X-API-Key Header
	if token == "" {
		token = r.Header.Get(domain.HeaderAPIKey)
	}

	if token == "" {
		return nil, domain.ErrUnauthorized
	}

	// Check master API key if configured
	apiKeyEnv := os.Getenv("FLAGURA_API_KEY")
	if apiKeyEnv != "" && subtle.ConstantTimeCompare([]byte(token), []byte(apiKeyEnv)) == 1 {
		return &domain.User{
			ID:    "usr_api_key_service",
			Email: "api-service@flagura.dev",
			Name:  "API Service Account",
			Role:  domain.RoleAdmin,
		}, nil
	}

	// Check stored dynamic API Keys via SHA-256 hash lookup
	h := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(h[:])
	if apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash); err == nil && apiKey != nil && !apiKey.Revoked {
		return &domain.User{
			ID:    apiKey.ID,
			Email: apiKey.CreatedBy,
			Name:  apiKey.Name,
			Role:  apiKey.Role,
		}, nil
	}

	sess, err := s.store.GetSession(r.Context(), token)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if sess.User == nil {
		return nil, domain.ErrUserNotFound
	}

	return sess.User, nil
}

func (s *Server) getAPIKeyFromRequest(r *http.Request) *domain.APIKey {
	var token string
	authHeader := r.Header.Get(domain.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = r.Header.Get(domain.HeaderAPIKey)
	}
	if token == "" {
		return nil
	}

	h := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(h[:])
	if apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash); err == nil && apiKey != nil && !apiKey.Revoked {
		return apiKey
	}
	return nil
}

func validatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>/?~", ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter (A-Z)")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter (a-z)")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one number (0-9)")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special symbol (!@#$%%^&*)")
	}

	return nil
}

func (s *Server) handleSignUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest, err))
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Email == "" || !strings.Contains(req.Email, "@") {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "A valid email address is required", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	if err := validatePasswordComplexity(req.Password); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodePasswordTooWeak, err.Error(), http.StatusBadRequest, err))
		return
	}
	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}

	// Security: All new registrations default strictly to RoleDeveloper.
	userRole := domain.RoleDeveloper

	// Check if user exists
	if _, err := s.store.GetUserByEmail(r.Context(), req.Email); err == nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeEmailAlreadyExists, "An account with this email address already exists", http.StatusConflict, domain.ErrEmailAlreadyExists))
		return
	}

	// Hash password with bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to hash password", http.StatusInternalServerError, err))
		return
	}

	user := domain.NewUser(req.Email, req.Name, string(hashedBytes), userRole)

	createdUser, err := s.store.CreateUser(r.Context(), user)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to create user: "+err.Error(), http.StatusInternalServerError, err))
		return
	}

	var activeProjID string
	if req.InviteToken != "" {
		if inv, err := s.store.GetOrgInvitation(r.Context(), req.InviteToken); err == nil && inv != nil {
			_, _ = s.store.AcceptOrgInvitation(r.Context(), req.InviteToken, createdUser.ID)
			if projs, err := s.store.ListProjects(r.Context(), inv.OrganizationID); err == nil && len(projs) > 0 {
				activeProjID = projs[0].ID
			}
		}
	}

	if activeProjID == "" {
		// Provision dedicated multi-tenant Organization for the new user
		orgName := fmt.Sprintf("%s's Workspace", createdUser.Name)
		orgSlug := fmt.Sprintf("%s-%s", domain.Slugify(createdUser.Name), createdUser.ID[len(createdUser.ID)-4:])
		userOrg := domain.NewOrganization(orgName, orgSlug, "Dedicated workspace for "+createdUser.Name)
		createdOrg, _ := s.store.CreateOrganization(r.Context(), userOrg)

		orgID := "org_" + createdUser.ID
		if createdOrg != nil && createdOrg.ID != "" {
			orgID = createdOrg.ID
		}
		// Register user as workspace owner
		_, _ = s.store.CreateOrgMember(r.Context(), domain.OrgMember{
			OrganizationID: orgID,
			UserID:         createdUser.ID,
			Role:           "owner",
		})

		// Provision default Project within the newly created Organization
		userProjSlug := fmt.Sprintf("prod-%s", createdUser.ID[len(createdUser.ID)-4:])
		userProj := domain.NewProject(orgID, "Production Flags", userProjSlug, "Production feature flags scope for "+createdUser.Name)
		createdProj, _ := s.store.CreateProject(r.Context(), userProj)
		if createdProj != nil && createdProj.ID != "" {
			activeProjID = createdProj.ID
		}
	}

	// Create session token (expires in 7 days)
	token, err := generateSessionToken()
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to generate session token", http.StatusInternalServerError, err))
		return
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	sess := domain.Session{
		Token:     token,
		UserID:    createdUser.ID,
		ExpiresAt: expiresAt,
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to persist session", http.StatusInternalServerError, err))
		return
	}

	s.setSessionCookie(w, r, token, expiresAt)

	// Set active project cookie to user's freshly provisioned unique project
	if activeProjID != "" {
		s.setProjectCookie(w, r, activeProjID, expiresAt)
	}

	// Send welcome email in background if email service is active
	if s.mailer != nil && s.mailer.IsEnabled() {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			host = "localhost:3000"
		}
		dashboardURL := fmt.Sprintf("%s://%s/dashboard", scheme, host)
		go func(email, name, dashURL string) {
			_ = s.mailer.SendWelcomeEmail(email, name, dashURL)
		}(createdUser.Email, createdUser.Name, dashboardURL)
	}

	s.writeJSON(w, http.StatusCreated, domain.AuthResponse{
		User:    createdUser,
		Token:   token,
		Message: "Account created successfully",
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest, err))
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInvalidCredentials, "Invalid email or password", http.StatusUnauthorized, domain.ErrUnauthorized))
		return
	}

	// Verify bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInvalidCredentials, "Invalid email or password", http.StatusUnauthorized, domain.ErrUnauthorized))
		return
	}

	// Create session token
	token, err := generateSessionToken()
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to generate session token", http.StatusInternalServerError, err))
		return
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	sess := domain.Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to persist session", http.StatusInternalServerError, err))
		return
	}

	s.setSessionCookie(w, r, token, expiresAt)

	// Set active project cookie if user has a scoped organization/project
	if orgs, err := s.store.ListUserOrganizations(r.Context(), user.ID); err == nil && len(orgs) > 0 {
		for _, org := range orgs {
			if projs, err := s.store.ListProjects(r.Context(), org.ID); err == nil && len(projs) > 0 {
				s.setProjectCookie(w, r, projs[0].ID, expiresAt)
				break
			}
		}
	}

	s.writeJSON(w, http.StatusOK, domain.AuthResponse{
		User:    user,
		Token:   token,
		Message: "Login successful",
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}

	s.clearSessionCookie(w, r)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.getUserFromRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.mailer == nil || !s.mailer.IsEnabled() {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Email Service Disabled",
			"message": "Email delivery is disabled on this instance (SMTP is not configured). Please contact your workspace administrator to reset your credentials.",
		})
		return
	}

	var req domain.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Create 15-minute token (if user exists)
	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err == nil && user != nil {
		token, err := s.store.CreatePasswordResetToken(r.Context(), email, 15*time.Minute)
		if err == nil {
			scheme := "http"
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			host := r.Host
			if host == "" {
				host = "localhost:3000"
			}
			resetURL := fmt.Sprintf("%s://%s/auth?mode=reset&token=%s", scheme, host, token)
			_ = s.mailer.SendPasswordReset(user.Email, user.Name, resetURL)
		}
	}

	// Always return 200 OK to prevent user enumeration attacks
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "If an account with that email exists, password reset instructions have been dispatched.",
	})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(req.Token)
	newPassword := req.NewPassword

	if token == "" {
		http.Error(w, "Reset token is required", http.StatusBadRequest)
		return
	}
	if err := validatePasswordComplexity(newPassword); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Weak Password",
			"message": err.Error(),
		})
		return
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	if err := s.store.ResetPasswordWithToken(r.Context(), token, string(hash)); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Invalid or Expired Token",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password has been successfully updated. You can now sign in with your new credentials.",
	})
}

