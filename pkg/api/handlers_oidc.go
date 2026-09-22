package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"golang.org/x/crypto/bcrypt"
)

// oidcHTTPClient is used for IdP token and userinfo requests.
var oidcHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// generateSecureToken creates a cryptographically secure URL-safe random string.
func generateSecureToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// getOIDCRedirectURI computes the absolute callback URL for the current request.
func (s *Server) getOIDCRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	if host == "" {
		host = "localhost:3000"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/oidc/callback", scheme, host)
}

func (s *Server) setOIDCStateCookie(w http.ResponseWriter, r *http.Request, stateClaims domain.OIDCStateClaims) error {
	data, err := json.Marshal(stateClaims)
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	isSecure := isCookieSecure(r)

	// #nosec G124 -- dynamic secure flag based on TLS/environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieOIDCStateName,
		Value:    encoded,
		Path:     "/",
		Expires:  time.Now().Add(10 * time.Minute),
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
	return nil
}

func (s *Server) getAndVerifyOIDCStateCookie(r *http.Request, stateParam string) (*domain.OIDCStateClaims, error) {
	cookie, err := r.Cookie(domain.CookieOIDCStateName)
	if err != nil || cookie.Value == "" {
		return nil, errors.New("missing or expired OIDC state cookie")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, errors.New("malformed OIDC state cookie")
	}

	var claims domain.OIDCStateClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, errors.New("invalid OIDC state payload")
	}

	// Verify constant-time match
	if subtle.ConstantTimeCompare([]byte(claims.State), []byte(stateParam)) != 1 {
		return nil, errors.New("OIDC state mismatch (possible CSRF)")
	}

	// Verify TTL (10 minutes)
	if time.Since(claims.CreatedAt) > 10*time.Minute {
		return nil, errors.New("OIDC state has expired")
	}

	return &claims, nil
}

func (s *Server) clearOIDCStateCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := isCookieSecure(r)
	// #nosec G124 -- dynamic secure flag based on TLS/environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieOIDCStateName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

// handleOIDCLogin initiates the OpenID Connect authorization code flow.
// GET /api/v1/auth/oidc/login?org_id=... or ?domain=...
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	orgID := strings.TrimSpace(r.URL.Query().Get("org_id"))
	domainQuery := strings.TrimSpace(r.URL.Query().Get("domain"))

	var cfg *domain.OIDCConfig
	var err error

	if orgID != "" {
		cfg, err = s.store.GetOIDCConfig(ctx, orgID)
	} else if domainQuery != "" {
		cfg, err = s.store.GetOIDCConfigByDomain(ctx, domainQuery)
	} else {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "Either 'org_id' or 'domain' query parameter is required to initiate SSO", http.StatusBadRequest, nil))
		return
	}

	if err != nil || cfg == nil || !cfg.Enabled {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, "SSO via OIDC is not configured or is currently disabled for this organization", http.StatusNotFound, err))
		return
	}

	state, err := generateSecureToken(32)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to generate security state token", http.StatusInternalServerError, err))
		return
	}

	nonce, err := generateSecureToken(32)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to generate security nonce", http.StatusInternalServerError, err))
		return
	}

	stateClaims := domain.OIDCStateClaims{
		State:          state,
		Nonce:          nonce,
		OrganizationID: cfg.OrganizationID,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.setOIDCStateCookie(w, r, stateClaims); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to set security state cookie", http.StatusInternalServerError, err))
		return
	}

	// Parse IdP authorization endpoint
	authURL, err := url.Parse(strings.TrimRight(cfg.IssuerURL, "/") + "/authorize")
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Invalid OIDC issuer URL configuration", http.StatusInternalServerError, err))
		return
	}

	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", s.getOIDCRedirectURI(r))
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	q.Set("nonce", nonce)
	authURL.RawQuery = q.Encode()

	// #nosec G710 -- redirect target is constructed from validated and configured organization OIDC issuer
	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

// OIDCTokenResponse represents the JSON response from an IdP token endpoint.
type OIDCTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// OIDCIDTokenClaims represents standard OIDC identity claims.
type OIDCIDTokenClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Nonce   string `json:"nonce"`
}

// parseIDTokenClaims decodes unverified payload claims from an ID token JWT.
func parseIDTokenClaims(idToken string) (*OIDCIDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return nil, errors.New("malformed id_token JWT")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Retry with standard base64 if needed
		payloadBytes, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to base64-decode JWT payload: %w", err)
		}
	}

	var claims OIDCIDTokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}
	return &claims, nil
}

// fetchUserInfo queries the IdP userinfo endpoint using the access token.
func fetchUserInfo(ctx context.Context, issuerURL, accessToken string) (*OIDCIDTokenClaims, error) {
	userinfoURL := strings.TrimRight(issuerURL, "/") + "/userinfo"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := oidcHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var claims OIDCIDTokenClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

// handleOIDCCallback handles the authorization code callback from the IdP.
// GET /api/v1/auth/oidc/callback?code=...&state=...
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Check for error sent from IdP
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		s.clearOIDCStateCookie(w, r)
		http.Redirect(w, r, fmt.Sprintf("/auth?error=%s", url.QueryEscape(errParam+": "+errDesc)), http.StatusFound)
		return
	}

	stateParam := r.URL.Query().Get("state")
	codeParam := r.URL.Query().Get("code")

	if stateParam == "" || codeParam == "" {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "Missing 'code' or 'state' parameter from IdP callback", http.StatusBadRequest, nil))
		return
	}

	// Verify state token against cookie
	stateClaims, err := s.getAndVerifyOIDCStateCookie(r, stateParam)
	if err != nil {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, err.Error(), http.StatusBadRequest, err))
		return
	}

	// Fetch OIDC configuration for the organization
	cfg, err := s.store.GetOIDCConfig(ctx, stateClaims.OrganizationID)
	if err != nil || cfg == nil || !cfg.Enabled {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, "Organization OIDC configuration not found or disabled", http.StatusNotFound, err))
		return
	}

	// Exchange authorization code for tokens
	tokenURL := strings.TrimRight(cfg.IssuerURL, "/") + "/token"
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {codeParam},
		"redirect_uri":  {s.getOIDCRedirectURI(r)},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to construct token exchange request", http.StatusInternalServerError, err))
		return
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := oidcHTTPClient.Do(tokenReq)
	if err != nil {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to connect to IdP token endpoint", http.StatusInternalServerError, err))
		return
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tokenResp.Body)
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, fmt.Sprintf("IdP token exchange failed (%d): %s", tokenResp.StatusCode, string(body)), http.StatusBadRequest, nil))
		return
	}

	var tokenData OIDCTokenResponse
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to parse token response", http.StatusInternalServerError, err))
		return
	}

	// Extract identity claims from ID Token or Userinfo
	var claims *OIDCIDTokenClaims
	if tokenData.IDToken != "" {
		claims, _ = parseIDTokenClaims(tokenData.IDToken)
	}
	if (claims == nil || claims.Email == "") && tokenData.AccessToken != "" {
		claims, err = fetchUserInfo(ctx, cfg.IssuerURL, tokenData.AccessToken)
		if err != nil {
			s.clearOIDCStateCookie(w, r)
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to fetch user claims from userinfo endpoint: "+err.Error(), http.StatusInternalServerError, err))
			return
		}
	}

	if claims == nil || strings.TrimSpace(claims.Email) == "" {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "Identity provider did not return an email claim in token", http.StatusBadRequest, nil))
		return
	}

	// Verify allowed domains
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if !cfg.IsDomainAllowed(email) {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeForbidden, fmt.Sprintf("Email domain for %q is not authorized for this organization", email), http.StatusForbidden, nil))
		return
	}

	// Just-In-Time (JIT) Provisioning
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil || user == nil {
		// Create new user account with unguessable random password
		randPwd, _ := generateSecureToken(32)
		pwdHash, _ := bcrypt.GenerateFromPassword([]byte(randPwd), bcrypt.DefaultCost)
		userName := strings.TrimSpace(claims.Name)
		if userName == "" {
			parts := strings.Split(email, "@")
			userName = parts[0]
		}
		newUser := domain.NewUser(email, userName, string(pwdHash), domain.RoleDeveloper)
		user, err = s.store.CreateUser(ctx, newUser)
		if err != nil {
			s.clearOIDCStateCookie(w, r)
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to provision user: "+err.Error(), http.StatusInternalServerError, err))
			return
		}
	}

	// Ensure org membership
	memberRole := cfg.DefaultRole
	if memberRole == "" {
		memberRole = "developer"
	}
	existingMember, _ := s.store.GetOrgMember(ctx, cfg.OrganizationID, user.ID)
	if existingMember == nil {
		_, _ = s.store.CreateOrgMember(ctx, domain.OrgMember{
			OrganizationID: cfg.OrganizationID,
			UserID:         user.ID,
			Role:           memberRole,
		})
	}

	// Create session (expires in 7 days)
	sessionToken, err := generateSessionToken()
	if err != nil {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to generate session token", http.StatusInternalServerError, err))
		return
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	session := domain.Session{
		Token:     sessionToken,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		s.clearOIDCStateCookie(w, r)
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to persist session", http.StatusInternalServerError, err))
		return
	}

	// Set session cookie
	s.setSessionCookie(w, r, sessionToken, expiresAt)

	// Resolve default project for the organization and set project cookie
	if projs, err := s.store.ListProjects(ctx, cfg.OrganizationID); err == nil && len(projs) > 0 {
		s.setProjectCookie(w, r, projs[0].ID, expiresAt)
	}

	// Clean up ephemeral OIDC state cookie
	s.clearOIDCStateCookie(w, r)

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// handleOIDCConfigRoutes routes GET, PUT, and DELETE requests for organization OIDC settings.
// /api/v1/organizations/{id}/oidc
func (s *Server) handleOIDCConfigRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/organizations/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "oidc" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	orgID := parts[0]

	// Authorize caller: must be authenticated and have management privileges in org (or platform superadmin)
	user := UserFromContext(r.Context())
	if user == nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeUnauthorized, "Authentication required", http.StatusUnauthorized, nil))
		return
	}

	if user.Role != domain.RoleAdmin {
		member, err := s.store.GetOrgMember(r.Context(), orgID, user.ID)
		if err != nil || member == nil || (member.Role != "owner" && member.Role != "admin") {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeForbidden, "Only organization owners and admins can configure Single Sign-On", http.StatusForbidden, nil))
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetOIDCConfig(w, r, orgID)
	case http.MethodPut, http.MethodPost:
		s.handleSaveOIDCConfig(w, r, orgID)
	case http.MethodDelete:
		s.handleDeleteOIDCConfig(w, r, orgID)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetOIDCConfig(w http.ResponseWriter, r *http.Request, orgID string) {
	cfg, err := s.store.GetOIDCConfig(r.Context(), orgID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to retrieve OIDC config: "+err.Error(), http.StatusInternalServerError, err))
		return
	}
	if cfg == nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"organization_id": orgID,
			"enabled":         false,
			"configured":      false,
		})
		return
	}

	resp := *cfg
	if resp.ClientSecret != "" {
		resp.ClientSecret = "••••••••"
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSaveOIDCConfig(w http.ResponseWriter, r *http.Request, orgID string) {
	var req domain.OIDCConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest, err))
		return
	}

	if req.IssuerURL == "" || req.ClientID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "issuer_url and client_id are required", http.StatusBadRequest, nil))
		return
	}

	// If secret is masked or empty, preserve existing secret if present
	if strings.TrimSpace(req.ClientSecret) == "" || req.ClientSecret == "••••••••" {
		existing, err := s.store.GetOIDCConfig(r.Context(), orgID)
		if err == nil && existing != nil && existing.ClientSecret != "" {
			req.ClientSecret = existing.ClientSecret
		} else {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "client_secret is required when configuring OIDC", http.StatusBadRequest, nil))
			return
		}
	}

	req.OrganizationID = orgID
	if err := s.store.SaveOIDCConfig(r.Context(), req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to save OIDC config: "+err.Error(), http.StatusInternalServerError, err))
		return
	}

	saved, _ := s.store.GetOIDCConfig(r.Context(), orgID)
	if saved != nil {
		saved.ClientSecret = "••••••••"
		s.writeJSON(w, http.StatusOK, saved)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "OIDC configuration saved successfully"})
}

func (s *Server) handleDeleteOIDCConfig(w http.ResponseWriter, r *http.Request, orgID string) {
	if err := s.store.DeleteOIDCConfig(r.Context(), orgID); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to delete OIDC config: "+err.Error(), http.StatusInternalServerError, err))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "OIDC configuration removed"})
}
