package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dhawalhost/flagura/pkg/domain"
)

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
