package domain

import (
	"errors"
	"time"
)

// Organization represents a top-level tenant organization (e.g. Acme Corp).
type Organization struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Workspace is an alias for Organization for backward compatibility.
type Workspace = Organization

// OrgMember represents a user's membership in an Organization.
type OrgMember struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Role           string    `json:"role"` // "owner", "admin", "developer", "viewer"
	CreatedAt      time.Time `json:"created_at"`
}

// OrgInvitation represents an invite for a user to join an Organization.
type OrgInvitation struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	OrgName        string     `json:"org_name"`
	Email          string     `json:"email"`
	Token          string     `json:"token"`
	Role           string     `json:"role"` // "admin", "developer", "viewer"
	InvitedBy      string     `json:"invited_by"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (i *OrgInvitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

func (i *OrgInvitation) IsAccepted() bool {
	return i.AcceptedAt != nil
}

// Project represents an isolated project scope within an Organization.
type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ChangeRequestStatus represents the review and application status.
type ChangeRequestStatus string

const (
	ChangeRequestStatusPending  ChangeRequestStatus = "PENDING"
	ChangeRequestStatusApproved ChangeRequestStatus = "APPROVED"
	ChangeRequestStatusRejected ChangeRequestStatus = "REJECTED"
	ChangeRequestStatusApplied  ChangeRequestStatus = "APPLIED"
)

// ChangeRequest enforces 4-Eyes Principle change approval governance on production flags.
type ChangeRequest struct {
	ID             string              `json:"id"`
	ProjectID      string              `json:"project_id"`
	FlagKey        string              `json:"flag_key"`
	Environment    Environment         `json:"environment"`
	Title          string              `json:"title"`
	Description    string              `json:"description,omitempty"`
	AuthorUserID   string              `json:"author_user_id"`
	AuthorEmail    string              `json:"author_email"`
	AuthorName     string              `json:"author_name"`
	ProposedConfig EnvironmentConfig   `json:"proposed_config"`
	Status         ChangeRequestStatus `json:"status"`
	ReviewerUserID string              `json:"reviewer_user_id,omitempty"`
	ReviewerEmail  string              `json:"reviewer_email,omitempty"`
	ReviewerName   string              `json:"reviewer_name,omitempty"`
	ReviewComments string              `json:"review_comments,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	ReviewedAt     *time.Time          `json:"reviewed_at,omitempty"`
	AppliedAt      *time.Time          `json:"applied_at,omitempty"`
}

// Review checks that the reviewer is not the author (4-Eyes governance) and updates status.
func (cr *ChangeRequest) Review(reviewerID, reviewerEmail, reviewerName string, approved bool, comments string) error {
	if cr.Status != ChangeRequestStatusPending {
		return errors.New("cannot review non-pending change request")
	}
	if cr.AuthorUserID == reviewerID {
		return ErrFourEyesSelfApproval
	}

	now := time.Now().UTC()
	cr.ReviewerUserID = reviewerID
	cr.ReviewerEmail = reviewerEmail
	cr.ReviewerName = reviewerName
	cr.ReviewComments = comments
	cr.ReviewedAt = &now

	if approved {
		cr.Status = ChangeRequestStatusApproved
	} else {
		cr.Status = ChangeRequestStatusRejected
	}
	return nil
}
