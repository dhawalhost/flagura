package store

import (
	"context"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

const (
	DefaultOrgID       = domain.DefaultOrgID
	DefaultOrgName     = domain.DefaultOrgName
	DefaultOrgSlug     = domain.DefaultOrgSlug
	DefaultProjectID   = domain.DefaultProjectID
	DefaultProjectName = domain.DefaultProjectName
	DefaultProjectSlug = domain.DefaultProjectSlug
)

type Store interface {
	// Organizations & Projects
	CreateOrganization(ctx context.Context, org domain.Organization) (*domain.Organization, error)
	GetOrganization(ctx context.Context, idOrSlug string) (*domain.Organization, error)
	ListOrganizations(ctx context.Context) ([]domain.Organization, error)
	ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error)

	CreateOrgMember(ctx context.Context, member domain.OrgMember) (*domain.OrgMember, error)
	ListOrgMembers(ctx context.Context, organizationID string) ([]domain.OrgMember, error)

	CreateOrgInvitation(ctx context.Context, inv domain.OrgInvitation) (*domain.OrgInvitation, error)
	GetOrgInvitation(ctx context.Context, token string) (*domain.OrgInvitation, error)
	AcceptOrgInvitation(ctx context.Context, token, userID string) (*domain.OrgMember, error)
	ListOrgInvitations(ctx context.Context, organizationID string) ([]domain.OrgInvitation, error)

	CreateProject(ctx context.Context, project domain.Project) (*domain.Project, error)
	GetProject(ctx context.Context, idOrSlug string) (*domain.Project, error)
	ListProjects(ctx context.Context, organizationID string) ([]domain.Project, error)

	// Flags & Audit (Project-Scoped)
	ListFlagsByProject(ctx context.Context, projectID string) ([]domain.FeatureFlag, error)
	GetFlagByProject(ctx context.Context, projectID, keyOrID string) (*domain.FeatureFlag, error)
	ListAuditLogsByProject(ctx context.Context, projectID string, limit int) ([]domain.AuditLogEntry, error)

	// Flags & Audit (Default Project fallback for backward compatibility)
	ListFlags(ctx context.Context) ([]domain.FeatureFlag, error)
	GetFlag(ctx context.Context, keyOrID string) (*domain.FeatureFlag, error)
	SaveFlag(ctx context.Context, flag domain.FeatureFlag, actor string) (*domain.AuditLogEntry, error)
	DeleteFlag(ctx context.Context, keyOrID string, actor string) (*domain.AuditLogEntry, error)
	ToggleFlag(ctx context.Context, keyOrID string, env domain.Environment, enabled *bool, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error)
	UpdateRollout(ctx context.Context, keyOrID string, env domain.Environment, pct float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error)
	ListAuditLogs(ctx context.Context, limit int) ([]domain.AuditLogEntry, error)
	Reset(ctx context.Context) error
	Ping(ctx context.Context) error
	DriverName() string

	// Users & Authentication
	CreateUser(ctx context.Context, user domain.User) (*domain.User, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	CreateSession(ctx context.Context, session domain.Session) error
	GetSession(ctx context.Context, token string) (*domain.Session, error)
	DeleteSession(ctx context.Context, token string) error
	CreatePasswordResetToken(ctx context.Context, email string, ttl time.Duration) (string, error)
	GetPasswordResetToken(ctx context.Context, token string) (*domain.PasswordResetToken, error)
	ResetPasswordWithToken(ctx context.Context, token string, newPasswordHash string) error

	// Experiments & A/B Testing
	RecordExperimentEvents(ctx context.Context, events []domain.ExperimentEvent) error
	GetExperimentEvents(ctx context.Context, flagKey string, limit int) ([]domain.ExperimentEvent, error)

	// Governance & 4-Eyes Change Approvals
	CreateChangeRequest(ctx context.Context, cr domain.ChangeRequest) (*domain.ChangeRequest, error)
	GetChangeRequest(ctx context.Context, id string) (*domain.ChangeRequest, error)
	ListChangeRequests(ctx context.Context, status domain.ChangeRequestStatus) ([]domain.ChangeRequest, error)
	ListChangeRequestsByProject(ctx context.Context, projectID string, status domain.ChangeRequestStatus) ([]domain.ChangeRequest, error)
	ReviewChangeRequest(ctx context.Context, id, reviewerID, reviewerEmail, reviewerName string, approved bool, comments string) (*domain.ChangeRequest, error)
	ApplyChangeRequest(ctx context.Context, id string, actor string) (*domain.FeatureFlag, *domain.ChangeRequest, *domain.AuditLogEntry, error)

	// API Keys & Service Accounts
	CreateAPIKey(ctx context.Context, key domain.APIKey) (*domain.APIKey, error)
	ListAPIKeys(ctx context.Context) ([]domain.APIKey, error)
	ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error)
	GetAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string, actor string) error
}
