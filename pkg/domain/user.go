package domain

import (
	"time"
)

type UserRole string

const (
	RoleAdmin     UserRole = "admin"
	RoleDeveloper UserRole = "developer"
	RoleQA        UserRole = "qa"
	RoleMember    UserRole = "member"
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
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Role     UserRole `json:"role"`
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
	ID         string     `json:"id"`
	Key        string     `json:"key,omitempty"`      // Raw token, only returned on initial creation
	KeyPrefix  string     `json:"key_prefix"`        // Display prefix (e.g. "flg_live_8f7b...****")
	KeyHash    string     `json:"key_hash,omitempty"` // SHA-256 hash for secure storage
	Name       string     `json:"name"`              // Descriptive name (e.g. "Prod K8s Cluster")
	Role       UserRole   `json:"role"`              // RoleDeveloper or RoleAdmin
	CreatedBy  string     `json:"created_by"`        // Creator user email
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Revoked    bool       `json:"revoked"`
}

