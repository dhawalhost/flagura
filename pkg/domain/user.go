package domain

import (
	"time"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         UserRole  `json:"role"`
	AvatarURL    string    `json:"avatarUrl,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Session struct {
	Token     string    `json:"token"`
	UserID    string    `json:"userId"`
	User      *User     `json:"user,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

type SignUpRequest struct {
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	Role        UserRole `json:"role"`
	InviteToken string   `json:"inviteToken,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	User    *User  `json:"user"`
	Token   string `json:"token"`
	Message string `json:"message"`
}

type APIKey struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId,omitempty"`
	Environment string     `json:"environment,omitempty"` // "production", "staging", "development", or "all"
	Key         string     `json:"key,omitempty"`         // Raw token, only returned on initial creation
	KeyPrefix   string     `json:"key_prefix"`            // Display prefix (e.g. "flg_live_8f7b...****")
	KeyHash     string     `json:"key_hash,omitempty"`    // SHA-256 hash for secure storage
	Name        string     `json:"name"`                  // Descriptive name (e.g. "Prod K8s Cluster")
	Role        UserRole   `json:"role"`                  // RoleDeveloper or RoleAdmin
	CreatedBy   string     `json:"created_by"`            // Creator user email
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	Revoked     bool       `json:"revoked"`
}

// AllowsEnvironment checks whether the API Key is authorized to access the given environment.
func (k *APIKey) AllowsEnvironment(env Environment) bool {
	if k == nil {
		return true
	}
	if k.Environment == "" || k.Environment == "all" || k.Environment == "*" {
		return true
	}
	return k.Environment == string(env)
}

type PasswordResetToken struct {
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"createdAt"`
}

func (t *PasswordResetToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

type UpdateProfileRequest struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}
