package domain

// Environment represents deployment and evaluation tiers.
type Environment string

const (
	EnvProduction  Environment = "production"
	EnvStaging     Environment = "staging"
	EnvDevelopment Environment = "development"
	EnvAll         Environment = "all"
)

// UserRole represents RBAC access authorization tiers.
type UserRole string

const (
	RoleAdmin     UserRole = "admin"
	RoleDeveloper UserRole = "developer"
	RoleQA        UserRole = "qa"
	RoleMember    UserRole = "member"
	RoleViewer    UserRole = "viewer"
)

// Standard HTTP Headers across Flagura APIs and SDKs.
const (
	HeaderProjectID     = "X-Project-ID"
	HeaderEnvironment   = "X-Environment"
	HeaderActor         = "X-Actor"
	HeaderAPIKey        = "X-API-Key"
	HeaderAuthorization = "Authorization"
	HeaderContentType   = "Content-Type"
	HeaderTrace         = "X-Trace"
	HeaderRequestID     = "X-Request-ID"
)

// Standard Cookie names.
const (
	CookieSessionName = "flagura_session"
	CookieProjectName = "flagura_project_id"
)

// Default System and Fallback Identifiers.
const (
	DefaultOrgID       = "org_default"
	DefaultOrgName     = "Default Organization"
	DefaultOrgSlug     = "default-org"
	DefaultProjectID   = "proj_default"
	DefaultProjectName = "Default Project"
	DefaultProjectSlug = "default-project"
)

// Standard Audit Actions.
const (
	ActionFlagCreated           = "FLAG_CREATED"
	ActionFlagUpdated           = "FLAG_UPDATED"
	ActionFlagDeleted           = "FLAG_DELETED"
	ActionKillSwitchToggled     = "KILL_SWITCH_TOGGLED"
	ActionRolloutUpdated        = "ROLLOUT_UPDATED"
	ActionEnvironmentPromoted   = "ENVIRONMENT_PROMOTED"
	ActionAPIKeyCreated         = "API_KEY_CREATED"
	ActionAPIKeyRevoked         = "API_KEY_REVOKED"
	ActionProjectCreated        = "PROJECT_CREATED"
	ActionOrganizationCreated   = "ORGANIZATION_CREATED"
	ActionChangeRequestCreated  = "CHANGE_REQUEST_CREATED"
	ActionChangeRequestApplied  = "CHANGE_REQUEST_APPLIED"
	ActionChangeRequestRejected = "CHANGE_REQUEST_REJECTED"
)
