package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/email"
	"github.com/dhawalhost/flagura/pkg/store"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthFlow(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 1. Test Sign Up with valid credentials
	signUpPayload := domain.SignUpRequest{
		Name:     "Test User",
		Email:    "test.user@company.com",
		Password: "FlaguraPass123!",
		Role:     domain.RoleDeveloper,
	}
	body, _ := json.Marshal(signUpPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected status 201 Created on sign up, got: %d (%s)", w.Code, w.Body.String())
	}

	var authResp domain.AuthResponse
	if err := json.NewDecoder(w.Body).Decode(&authResp); err != nil {
		t.Fatalf("Failed to decode auth response: %v", err)
	}
	if authResp.User == nil || authResp.User.Email != "test.user@company.com" {
		t.Fatalf("Expected created user email 'test.user@company.com', got: %+v", authResp.User)
	}
	if authResp.Token == "" {
		t.Fatalf("Expected non-empty session token")
	}

	// Check cookie
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("Expected %s cookie in response", SessionCookieName)
	}

	// 2. Test Sign Up with duplicate email (should fail with 409 Conflict)
	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(body))
	reqDup.Header.Set("Content-Type", "application/json")
	wDup := httptest.NewRecorder()
	server.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusConflict {
		t.Fatalf("Expected status 409 Conflict for duplicate email, got: %d", wDup.Code)
	}

	// 3. Test Login with valid credentials
	loginPayload := domain.LoginRequest{
		Email:    "test.user@company.com",
		Password: "FlaguraPass123!",
	}
	loginBody, _ := json.Marshal(loginPayload)
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	server.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK on login, got: %d (%s)", wLogin.Code, wLogin.Body.String())
	}

	// 4. Test Login with invalid password (should fail with 401 Unauthorized)
	badLoginPayload := domain.LoginRequest{
		Email:    "test.user@company.com",
		Password: fmt.Sprintf("WrongPass_%d", time.Now().UnixNano()),
	}
	badLoginBody, _ := json.Marshal(badLoginPayload)
	reqBadLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(badLoginBody))
	reqBadLogin.Header.Set("Content-Type", "application/json")
	wBadLogin := httptest.NewRecorder()
	server.ServeHTTP(wBadLogin, reqBadLogin)
	if wBadLogin.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized for bad password, got: %d", wBadLogin.Code)
	}

	// 5. Test Authenticated /api/v1/auth/me endpoint
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.AddCookie(sessionCookie)
	wMe := httptest.NewRecorder()
	server.ServeHTTP(wMe, reqMe)

	if wMe.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK on /me, got: %d (%s)", wMe.Code, wMe.Body.String())
	}

	var meUser domain.User
	if err := json.NewDecoder(wMe.Body).Decode(&meUser); err != nil {
		t.Fatalf("Failed to decode /me response: %v", err)
	}
	if meUser.Email != "test.user@company.com" {
		t.Fatalf("Expected me email 'test.user@company.com', got: %s", meUser.Email)
	}

	// 6. Test Logout
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	reqLogout.AddCookie(sessionCookie)
	wLogout := httptest.NewRecorder()
	server.ServeHTTP(wLogout, reqLogout)
	if wLogout.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK on logout, got: %d", wLogout.Code)
	}

	// Verify session revoked
	reqMeAfter := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMeAfter.AddCookie(sessionCookie)
	wMeAfter := httptest.NewRecorder()
	server.ServeHTTP(wMeAfter, reqMeAfter)
	if wMeAfter.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized on /me after logout, got: %d", wMeAfter.Code)
	}
}

func TestProtectedDashboard(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	// Unauthenticated dashboard access should redirect to /auth
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("Expected status 303 See Other redirect to /auth, got: %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/auth" {
		t.Fatalf("Expected redirect Location '/auth', got: %s", loc)
	}

	// Create user and session
	user, _ := memStore.CreateUser(context.Background(), domain.User{
		Email: "admin@flagura.dev",
		Name:  "Dhawal",
		Role:  domain.RoleAdmin,
	})
	token := "valid_test_token"
	_ = memStore.CreateSession(context.Background(), domain.Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// Authenticated dashboard access should return 200 OK
	reqAuth := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	reqAuth.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: token,
	})
	wAuth := httptest.NewRecorder()
	server.ServeHTTP(wAuth, reqAuth)

	if wAuth.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for authenticated dashboard, got: %d", wAuth.Code)
	}
}

func TestSignupCannotSelfAssignAdminRole(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	// Attacker tries to register as admin
	payload := map[string]string{
		"email":    "attacker@evil.com",
		"password": "Hunter2Password!",
		"role":     "admin",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d", w.Code)
	}

	createdUser, err := memStore.GetUserByEmail(context.Background(), "attacker@evil.com")
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}

	if createdUser.Role == domain.RoleAdmin {
		t.Fatalf("SECURITY VULNERABILITY: User was able to self-assign RoleAdmin at signup!")
	}
	if createdUser.Role != domain.RoleDeveloper {
		t.Fatalf("Expected default RoleDeveloper, got %s", createdUser.Role)
	}
}

func TestPasswordComplexityValidation(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	testCases := []struct {
		name     string
		password string
	}{
		{"TooShort", "Aa1!"},
		{"NoUppercase", "lowercase123!"},
		{"NoLowercase", "UPPERCASE123!"},
		{"NoNumber", "NoNumbersHere!"},
		{"NoSpecialChar", "NoSpecialChars123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := domain.SignUpRequest{
				Name:     "Tester",
				Email:    "test_" + tc.name + "@company.com",
				Password: tc.password,
				Role:     domain.RoleDeveloper,
			}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("Expected 400 Bad Request for weak password '%s', got: %d", tc.password, w.Code)
			}
		})
	}
}

func TestForgotPasswordAndResetFlow(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	initialRawPassword := fmt.Sprintf("InitPass_%d!", time.Now().UnixNano())
	newRawPassword := fmt.Sprintf("NewStrongPass_%d_Safe!", time.Now().UnixNano())

	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(initialRawPassword), bcrypt.DefaultCost)
	u := domain.NewUser("dhawal@flagura.dev", "Dhawal", string(hashedBytes), domain.RoleDeveloper)
	_, _ = memStore.CreateUser(context.Background(), u)

	forgotPayload := domain.ForgotPasswordRequest{
		Email: "dhawal@flagura.dev",
	}
	body, _ := json.Marshal(forgotPayload)

	// 1. When SMTP is not configured, email is disabled by default -> 400 Bad Request
	reqDisabled := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	reqDisabled.Header.Set("Content-Type", "application/json")
	wDisabled := httptest.NewRecorder()
	server.ServeHTTP(wDisabled, reqDisabled)

	if wDisabled.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request when email service is disabled, got %d: %s", wDisabled.Code, wDisabled.Body.String())
	}

	// 2. Enable mailer (e.g. SMTP configured or ConsoleMailer enabled)
	server.SetMailer(email.NewConsoleMailer())

	reqForgot := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	reqForgot.Header.Set("Content-Type", "application/json")
	wForgot := httptest.NewRecorder()
	server.ServeHTTP(wForgot, reqForgot)

	if wForgot.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on forgot password when mailer is enabled, got %d: %s", wForgot.Code, wForgot.Body.String())
	}

	// 2. Generate a reset token directly from store to simulate receiving link via email
	resetToken, err := memStore.CreatePasswordResetToken(context.Background(), "dhawal@flagura.dev", 15*time.Minute)
	if err != nil {
		t.Fatalf("Failed to create reset token: %v", err)
	}

	// 3. Reset password using valid token
	resetPayload := domain.ResetPasswordRequest{
		Token:       resetToken,
		NewPassword: newRawPassword,
	}
	resetBody, _ := json.Marshal(resetPayload)
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(resetBody))
	reqReset.Header.Set("Content-Type", "application/json")
	wReset := httptest.NewRecorder()
	server.ServeHTTP(wReset, reqReset)

	if wReset.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on password reset, got %d: %s", wReset.Code, wReset.Body.String())
	}

	// 4. Old password must fail
	oldLogin := domain.LoginRequest{
		Email:    "dhawal@flagura.dev",
		Password: initialRawPassword,
	}
	oldBody, _ := json.Marshal(oldLogin)
	reqOld := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(oldBody))
	reqOld.Header.Set("Content-Type", "application/json")
	wOld := httptest.NewRecorder()
	server.ServeHTTP(wOld, reqOld)

	if wOld.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for old password, got %d", wOld.Code)
	}

	// 5. New password must succeed
	newLogin := domain.LoginRequest{
		Email:    "dhawal@flagura.dev",
		Password: newRawPassword,
	}
	newBody, _ := json.Marshal(newLogin)
	reqNew := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(newBody))
	reqNew.Header.Set("Content-Type", "application/json")
	wNew := httptest.NewRecorder()
	server.ServeHTTP(wNew, reqNew)

	if wNew.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for new password, got %d", wNew.Code)
	}

	// 6. Token reuse must fail (single-use token)
	reqReuse := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(resetBody))
	reqReuse.Header.Set("Content-Type", "application/json")
	wReuse := httptest.NewRecorder()
	server.ServeHTTP(wReuse, reqReuse)

	if wReuse.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for reused token, got %d", wReuse.Code)
	}
}

