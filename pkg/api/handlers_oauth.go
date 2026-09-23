package api

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"golang.org/x/crypto/bcrypt"
)

// oauthHTTPClient is used for OAuth token exchange and user profile queries.
var oauthHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

type oauthEndpoints struct {
	authURL     string
	tokenURL    string
	userInfoURL string
	emailsURL   string
}

// #nosec G101 -- public OAuth 2.0 provider authorization and token endpoint URLs
var googleEndpoints = oauthEndpoints{
	authURL:     "https://accounts.google.com/o/oauth2/v2/auth",
	tokenURL:    "https://oauth2.googleapis.com/token",
	userInfoURL: "https://www.googleapis.com/oauth2/v3/userinfo",
}

// #nosec G101 -- public OAuth 2.0 provider authorization and token endpoint URLs
var githubEndpoints = oauthEndpoints{
	authURL:     "https://github.com/login/oauth/authorize",
	tokenURL:    "https://github.com/login/oauth/access_token",
	userInfoURL: "https://api.github.com/user",
	emailsURL:   "https://api.github.com/user/emails",
}

// OAuthProvidersResponse represents the public status of social login providers.
type OAuthProvidersResponse struct {
	Google bool `json:"google"`
	GitHub bool `json:"github"`
}

// OAuthStateClaims encapsulates CSRF state claims stored in the temporary state cookie.
type OAuthStateClaims struct {
	Provider  string    `json:"provider"`
	State     string    `json:"state"`
	Nonce     string    `json:"nonce"`
	CreatedAt time.Time `json:"created_at"`
}

// getOAuthRedirectURI computes the absolute callback URL for a given provider.
func (s *Server) getOAuthRedirectURI(r *http.Request, provider string) string {
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
	return fmt.Sprintf("%s://%s/api/v1/auth/oauth/%s/callback", scheme, host, provider)
}

func (s *Server) setOAuthStateCookie(w http.ResponseWriter, r *http.Request, claims OAuthStateClaims) error {
	data, err := json.Marshal(claims)
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	isSecure := isCookieSecure(r)

	// #nosec G124 -- dynamic secure flag based on TLS/environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieOAuthStateName,
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

func (s *Server) clearOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := isCookieSecure(r)
	// #nosec G124 -- dynamic secure flag based on TLS/environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieOAuthStateName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) getAndVerifyOAuthStateCookie(r *http.Request, provider, incomingState string) (*OAuthStateClaims, error) {
	cookie, err := r.Cookie(domain.CookieOAuthStateName)
	if err != nil || cookie.Value == "" {
		return nil, fmt.Errorf("missing or expired oauth state cookie")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid oauth state cookie encoding")
	}

	var claims OAuthStateClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("invalid oauth state cookie payload")
	}

	if claims.Provider != provider {
		return nil, fmt.Errorf("oauth provider mismatch")
	}

	if time.Since(claims.CreatedAt) > 10*time.Minute {
		return nil, fmt.Errorf("oauth state expired")
	}

	if subtle.ConstantTimeCompare([]byte(incomingState), []byte(claims.State)) != 1 {
		return nil, fmt.Errorf("invalid oauth state token")
	}

	return &claims, nil
}

// handleOAuthProviders returns whether Google and GitHub social login are enabled on the platform.
// GET /api/v1/auth/oauth/providers
func (s *Server) handleOAuthProviders(w http.ResponseWriter, r *http.Request) {
	resp := OAuthProvidersResponse{
		Google: s.cfg.IsGoogleOAuthEnabled(),
		GitHub: s.cfg.IsGitHubOAuthEnabled(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleOAuthGoogleLogin initiates the Google OpenID Connect / OAuth flow.
// GET /api/v1/auth/oauth/google/login
func (s *Server) handleOAuthGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsGoogleOAuthEnabled() {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, "Google authentication is not configured on this server", http.StatusNotFound, nil))
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

	claims := OAuthStateClaims{
		Provider:  "google",
		State:     state,
		Nonce:     nonce,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.setOAuthStateCookie(w, r, claims); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to set security state cookie", http.StatusInternalServerError, err))
		return
	}

	authURL, err := url.Parse(googleEndpoints.authURL)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to parse Google auth URL", http.StatusInternalServerError, err))
		return
	}

	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", s.cfg.GoogleClientID)
	q.Set("redirect_uri", s.getOAuthRedirectURI(r, "google"))
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("access_type", "online")
	authURL.RawQuery = q.Encode()

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

// handleOAuthGitHubLogin initiates the GitHub OAuth authorization flow.
// GET /api/v1/auth/oauth/github/login
func (s *Server) handleOAuthGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsGitHubOAuthEnabled() {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, "GitHub authentication is not configured on this server", http.StatusNotFound, nil))
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

	claims := OAuthStateClaims{
		Provider:  "github",
		State:     state,
		Nonce:     nonce,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.setOAuthStateCookie(w, r, claims); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to set security state cookie", http.StatusInternalServerError, err))
		return
	}

	authURL, err := url.Parse(githubEndpoints.authURL)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to parse GitHub auth URL", http.StatusInternalServerError, err))
		return
	}

	q := authURL.Query()
	q.Set("client_id", s.cfg.GitHubClientID)
	q.Set("redirect_uri", s.getOAuthRedirectURI(r, "github"))
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	authURL.RawQuery = q.Encode()

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

// handleOAuthGoogleCallback handles the callback redirect from Google.
// GET /api/v1/auth/oauth/google/callback
func (s *Server) handleOAuthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		s.clearOAuthStateCookie(w, r)
		http.Redirect(w, r, fmt.Sprintf("/auth?error=%s", url.QueryEscape("Google login cancelled: "+errParam)), http.StatusFound)
		return
	}

	stateParam := r.URL.Query().Get("state")
	codeParam := r.URL.Query().Get("code")
	if stateParam == "" || codeParam == "" {
		s.clearOAuthStateCookie(w, r)
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Missing code or state from Google"), http.StatusFound)
		return
	}

	if _, err := s.getAndVerifyOAuthStateCookie(r, "google", stateParam); err != nil {
		s.clearOAuthStateCookie(w, r)
		http.Redirect(w, r, fmt.Sprintf("/auth?error=%s", url.QueryEscape("Authentication security check failed: "+err.Error())), http.StatusFound)
		return
	}
	s.clearOAuthStateCookie(w, r)

	// Exchange authorization code for token
	tokenValues := url.Values{
		"code":          {codeParam},
		"client_id":     {s.cfg.GoogleClientID},
		"client_secret": {s.cfg.GoogleClientSecret},
		"redirect_uri":  {s.getOAuthRedirectURI(r, "google")},
		"grant_type":    {"authorization_code"},
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, googleEndpoints.tokenURL, strings.NewReader(tokenValues.Encode()))
	if err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Internal error preparing token exchange"), http.StatusFound)
		return
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := oauthHTTPClient.Do(tokenReq)
	if err != nil || tokenResp.StatusCode != http.StatusOK {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to exchange authorization code with Google"), http.StatusFound)
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Invalid token response from Google"), http.StatusFound)
		return
	}

	// Fetch user profile from Google UserInfo endpoint
	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, googleEndpoints.userInfoURL, nil)
	if err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to fetch Google profile"), http.StatusFound)
		return
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := oauthHTTPClient.Do(userReq)
	if err != nil || userResp.StatusCode != http.StatusOK {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to retrieve profile from Google"), http.StatusFound)
		return
	}
	defer userResp.Body.Close()

	var profile struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&profile); err != nil || strings.TrimSpace(profile.Email) == "" {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Google profile did not return a valid email address"), http.StatusFound)
		return
	}

	s.completeOAuthLogin(w, r, strings.ToLower(strings.TrimSpace(profile.Email)), profile.Name, profile.Picture)
}

// handleOAuthGitHubCallback handles the callback redirect from GitHub.
// GET /api/v1/auth/oauth/github/callback
func (s *Server) handleOAuthGitHubCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		s.clearOAuthStateCookie(w, r)
		http.Redirect(w, r, fmt.Sprintf("/auth?error=%s", url.QueryEscape("GitHub login cancelled: "+errParam)), http.StatusFound)
		return
	}

	stateParam := r.URL.Query().Get("state")
	codeParam := r.URL.Query().Get("code")
	if stateParam == "" || codeParam == "" {
		s.clearOAuthStateCookie(w, r)
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Missing code or state from GitHub"), http.StatusFound)
		return
	}

	if _, err := s.getAndVerifyOAuthStateCookie(r, "github", stateParam); err != nil {
		s.clearOAuthStateCookie(w, r)
		http.Redirect(w, r, fmt.Sprintf("/auth?error=%s", url.QueryEscape("Authentication security check failed: "+err.Error())), http.StatusFound)
		return
	}
	s.clearOAuthStateCookie(w, r)

	// Exchange code for token
	tokenBody, _ := json.Marshal(map[string]string{
		"client_id":     s.cfg.GitHubClientID,
		"client_secret": s.cfg.GitHubClientSecret,
		"code":          codeParam,
		"redirect_uri":  s.getOAuthRedirectURI(r, "github"),
	})

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, githubEndpoints.tokenURL, bytes.NewReader(tokenBody))
	if err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Internal error preparing token exchange"), http.StatusFound)
		return
	}
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := oauthHTTPClient.Do(tokenReq)
	if err != nil || tokenResp.StatusCode != http.StatusOK {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to exchange authorization code with GitHub"), http.StatusFound)
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Invalid token response from GitHub"), http.StatusFound)
		return
	}

	// Fetch user profile from GitHub API
	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, githubEndpoints.userInfoURL, nil)
	if err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to fetch GitHub profile"), http.StatusFound)
		return
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := oauthHTTPClient.Do(userReq)
	if err != nil || userResp.StatusCode != http.StatusOK {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to retrieve profile from GitHub"), http.StatusFound)
		return
	}
	defer userResp.Body.Close()

	var ghUser struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	_ = json.NewDecoder(userResp.Body).Decode(&ghUser)

	// Query GitHub verified emails endpoint
	emailsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, githubEndpoints.emailsURL, nil)
	if err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to fetch GitHub emails"), http.StatusFound)
		return
	}
	emailsReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	emailsReq.Header.Set("Accept", "application/json")

	emailsResp, err := oauthHTTPClient.Do(emailsReq)
	if err != nil || emailsResp.StatusCode != http.StatusOK {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to query verified emails from GitHub"), http.StatusFound)
		return
	}
	defer emailsResp.Body.Close()

	var emailsList []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(emailsResp.Body).Decode(&emailsList); err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to parse GitHub emails"), http.StatusFound)
		return
	}

	var verifiedEmail string
	for _, entry := range emailsList {
		if entry.Primary && entry.Verified {
			verifiedEmail = entry.Email
			break
		}
	}
	// Fallback to any verified email if primary is not explicitly tagged
	if verifiedEmail == "" {
		for _, entry := range emailsList {
			if entry.Verified {
				verifiedEmail = entry.Email
				break
			}
		}
	}

	if verifiedEmail == "" {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Your GitHub account does not have a verified email address"), http.StatusFound)
		return
	}

	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}
	if displayName == "" {
		displayName = strings.Split(verifiedEmail, "@")[0]
	}

	s.completeOAuthLogin(w, r, strings.ToLower(strings.TrimSpace(verifiedEmail)), displayName, ghUser.AvatarURL)
}

// completeOAuthLogin finalizes user registration/linking, provisions personal workspace on new signup, and sets session cookies.
func (s *Server) completeOAuthLogin(w http.ResponseWriter, r *http.Request, email, name, avatarURL string) {
	ctx := r.Context()

	var targetUser *domain.User
	user, err := s.store.GetUserByEmail(ctx, email)
	var activeProjID string

	if err == nil && user != nil {
		targetUser = user
		if targetUser.AvatarURL == "" && avatarURL != "" {
			targetUser.AvatarURL = avatarURL
			_, _ = s.store.UpdateUser(ctx, *targetUser)
		}
	} else {
		// New User Registration via OAuth
		if name == "" {
			name = strings.Split(email, "@")[0]
		}
		randomBytes := make([]byte, 32)
		_, _ = rand.Read(randomBytes)
		dummyHash, _ := bcrypt.GenerateFromPassword(randomBytes, bcrypt.DefaultCost)

		newUser := domain.NewUser(email, name, string(dummyHash), domain.RoleDeveloper)
		newUser.AvatarURL = avatarURL

		createdUser, err := s.store.CreateUser(ctx, newUser)
		if err != nil {
			http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to create user account: "+err.Error()), http.StatusFound)
			return
		}
		targetUser = createdUser

		// Provision personal workspace organization & default project
		orgName := fmt.Sprintf("%s's Workspace", targetUser.Name)
		orgSlug := fmt.Sprintf("%s-%s", domain.Slugify(targetUser.Name), targetUser.ID[len(targetUser.ID)-4:])
		userOrg := domain.NewOrganization(orgName, orgSlug, "Dedicated workspace for "+targetUser.Name)
		createdOrg, _ := s.store.CreateOrganization(ctx, userOrg)

		orgID := "org_" + targetUser.ID
		if createdOrg != nil && createdOrg.ID != "" {
			orgID = createdOrg.ID
		}

		_, _ = s.store.CreateOrgMember(ctx, domain.OrgMember{
			OrganizationID: orgID,
			UserID:         targetUser.ID,
			Role:           "owner",
		})

		userProjSlug := fmt.Sprintf("prod-%s", targetUser.ID[len(targetUser.ID)-4:])
		userProj := domain.NewProject(orgID, "Production Flags", userProjSlug, "Production feature flags scope for "+targetUser.Name)
		createdProj, _ := s.store.CreateProject(ctx, userProj)
		if createdProj != nil && createdProj.ID != "" {
			activeProjID = createdProj.ID
		}
	}

	// Resolve active project ID if not set from new workspace creation
	if activeProjID == "" {
		if orgs, err := s.store.ListUserOrganizations(ctx, targetUser.ID); err == nil && len(orgs) > 0 {
			if projs, err := s.store.ListProjects(ctx, orgs[0].ID); err == nil && len(projs) > 0 {
				activeProjID = projs[0].ID
			}
		}
	}
	if activeProjID == "" {
		activeProjID = domain.DefaultProjectID
	}

	// Issue session
	sessionToken, err := generateSessionToken()
	if err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to generate session token"), http.StatusFound)
		return
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	session := domain.Session{
		Token:     sessionToken,
		UserID:    targetUser.ID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		http.Redirect(w, r, "/auth?error="+url.QueryEscape("Failed to persist session"), http.StatusFound)
		return
	}

	s.setSessionCookie(w, r, sessionToken, expiresAt)
	s.setProjectCookie(w, r, activeProjID, expiresAt)

	http.Redirect(w, r, "/dashboard", http.StatusFound)
}
