package api

// Standard HTTP Route Paths across Flagura Server APIs and UI.
const (
	// Public & Observability Routes
	RouteRoot           = "/"
	RouteHealth         = "/health"
	RouteHealthz        = "/healthz"
	RouteAPIHealth      = "/api/health"
	RouteAPIV1Health    = "/api/v1/health"
	RouteLivez          = "/livez"
	RouteReadyz         = "/readyz"
	RouteMetrics        = "/metrics"
	RouteDocs           = "/docs"
	RouteDocsPrefix     = "/docs/"
	RouteStaticPrefix   = "/static/"
	RouteInstallScript  = "/install.sh"

	// UI Dashboard Routes
	RouteUIAuth         = "/auth"
	RouteUIDashboard    = "/dashboard"
	RouteUIDashboardPrefix = "/dashboard/"

	// Authentication API Routes
	RouteAuthSignUp        = "/api/v1/auth/signup"
	RouteAuthLogin         = "/api/v1/auth/login"
	RouteAuthLogout        = "/api/v1/auth/logout"
	RouteAuthForgotPassword= "/api/v1/auth/forgot-password" // #nosec G101 -- HTTP route path, not a secret
	RouteAuthResetPassword = "/api/v1/auth/reset-password"  // #nosec G101 -- HTTP route path, not a secret
	RouteAuthMe            = "/api/v1/auth/me"
	RouteAuthProfile       = "/api/v1/auth/profile"
	RouteAuthChangePassword= "/api/v1/auth/change-password" // #nosec G101 -- HTTP route path, not a secret
	RouteAuthOIDCLogin     = "/api/v1/auth/oidc/login"
	RouteAuthOIDCCallback  = "/api/v1/auth/oidc/callback"

	// Feature Flag & Evaluation API Routes
	RouteAPIFlags          = "/api/v1/flags"
	RouteAPIFlagsPrefix    = "/api/v1/flags/"
	RouteAPIFlagsStream    = "/api/v1/flags/stream"
	RouteAPIEvaluate       = "/api/v1/evaluate"
	RouteAPIBenchmark      = "/api/v1/benchmark"

	// Telemetry, Events & Canary Webhooks
	RouteAPITelemetryEvents     = "/api/v1/telemetry/events"
	RouteAPITelemetryStats      = "/api/v1/telemetry/stats"
	RouteAPITelemetryStatsPrefix= "/api/v1/telemetry/stats/"
	RouteAPIWebhooksKillSwitch  = "/api/v1/webhooks/kill-switch/"
	RouteAPIEvents              = "/api/v1/events"
	RouteAPIExperimentsPrefix   = "/api/v1/experiments/"

	// Multi-Tenancy & Project Governance API Routes
	RouteAPIOrganizations       = "/api/v1/organizations"
	RouteAPIOrganizationsPrefix = "/api/v1/organizations/"
	RouteAPIProjects            = "/api/v1/projects"
	RouteAPIProjectsActive      = "/api/v1/projects/active"
	RouteAPIProjectsPrefix      = "/api/v1/projects/"
	RouteAPIInvitations         = "/api/v1/invitations"
	RouteAPIInvitationsAccept   = "/api/v1/invitations/accept"
	RouteAPIInvitationsPrefix   = "/api/v1/invitations/"
	RouteAPIChangeRequests      = "/api/v1/change-requests"
	RouteAPIChangeRequestsPrefix= "/api/v1/change-requests/"
	RouteAPIKeys                = "/api/v1/api-keys"  // #nosec G101 -- HTTP route path, not a secret
	RouteAPIKeysPrefix          = "/api/v1/api-keys/" // #nosec G101 -- HTTP route path, not a secret

	// Audit Logs & Disaster Recovery Backup API Routes
	RouteAPIAuditLogs           = "/api/v1/audit-logs"
	RouteAPIAuditVerify         = "/api/v1/audit/verify"
	RouteAPIAuditExport         = "/api/v1/audit/export"
	RouteAPIAuditPurge          = "/api/v1/audit/purge"
	RouteAPIBackupExport        = "/api/v1/backup/export"
	RouteAPIBackupImport        = "/api/v1/backup/import"
	RouteAPIReset               = "/api/v1/reset"
)
