package domain

import (
	"testing"
)

func TestUser_EffectiveRole_And_IsPrivileged(t *testing.T) {
	tests := []struct {
		name               string
		user               *User
		expectedRole       string
		expectedPrivileged bool
	}{
		{
			name:               "nil user",
			user:               nil,
			expectedRole:       "",
			expectedPrivileged: false,
		},
		{
			name: "platform staff admin bypass",
			user: &User{
				ID:   "u_admin",
				Role: RoleAdmin,
				ActiveMembership: &OrgMember{
					Role: "developer",
				},
			},
			expectedRole:       "admin",
			expectedPrivileged: true,
		},
		{
			name: "org owner",
			user: &User{
				ID:   "u_owner",
				Role: RoleDeveloper,
				ActiveMembership: &OrgMember{
					Role: "owner",
				},
			},
			expectedRole:       "owner",
			expectedPrivileged: true,
		},
		{
			name: "org admin",
			user: &User{
				ID:   "u_org_admin",
				Role: RoleDeveloper,
				ActiveMembership: &OrgMember{
					Role: "admin",
				},
			},
			expectedRole:       "admin",
			expectedPrivileged: true,
		},
		{
			name: "org developer",
			user: &User{
				ID:   "u_dev",
				Role: RoleDeveloper,
				ActiveMembership: &OrgMember{
					Role: "developer",
				},
			},
			expectedRole:       "developer",
			expectedPrivileged: false,
		},
		{
			name: "org viewer",
			user: &User{
				ID:   "u_viewer",
				Role: RoleDeveloper,
				ActiveMembership: &OrgMember{
					Role: "viewer",
				},
			},
			expectedRole:       "viewer",
			expectedPrivileged: false,
		},
		{
			name: "no active membership falls back to base role",
			user: &User{
				ID:   "u_fallback",
				Role: RoleDeveloper,
			},
			expectedRole:       "developer",
			expectedPrivileged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := tt.user.EffectiveRole()
			if role != tt.expectedRole {
				t.Errorf("EffectiveRole() = %q, want %q", role, tt.expectedRole)
			}
			privileged := tt.user.IsPrivileged()
			if privileged != tt.expectedPrivileged {
				t.Errorf("IsPrivileged() = %v, want %v", privileged, tt.expectedPrivileged)
			}
		})
	}
}
