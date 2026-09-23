package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/canary"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/email"
	"github.com/dhawalhost/flagura/pkg/store"
	"github.com/dhawalhost/flagura/web"
)

type Server struct {
	store       store.Store
	mux         *http.ServeMux
	handler     http.Handler
	startTime   time.Time
	evalCount   uint64
	authLimiter *IPRateLimiter
	apiLimiter  *IPRateLimiter
	streamHub   *StreamHub
	telemetry   *TelemetryAggregator
	canary      *canary.CanaryScheduler
	mailer      email.Mailer
}

func NewServer(st store.Store) (*Server, error) {
	hub := NewStreamHub()
	go hub.Run()

	canarySched := canary.NewCanaryScheduler(st, hub)
	canarySched.StartBackgroundLoop(15 * time.Second)

	s := &Server{
		store:       st,
		mux:         http.NewServeMux(),
		startTime:   time.Now().UTC(),
		authLimiter: NewIPRateLimiter(5, 10, 1*time.Minute),
		apiLimiter:  NewIPRateLimiter(200, 400, 1*time.Minute).EnableTiers(true),
		streamHub:   hub,
		telemetry:   NewTelemetryAggregator(),
		canary:      canarySched,
		mailer:      email.NewMailerFromEnv(),
	}
	s.routes()
	s.handler = s.PanicRecoveryMiddleware(
		TracingMiddleware(
			RequestIDMiddleware(
				StructuredLoggerMiddleware(
					SecurityHeadersMiddleware(
						MaxBytesMiddleware(1<<20, s.mux),
					),
				),
			),
		),
	)
	return s, nil
}

func (s *Server) SetMailer(m email.Mailer) {
	s.mailer = m
}

func (s *Server) routes() {
	// Static asset server from embedded filesystem
	staticFS, err := fs.Sub(web.Files, "static")
	if err == nil {
		s.mux.Handle(RouteStaticPrefix, http.StripPrefix(RouteStaticPrefix, http.FileServer(http.FS(staticFS))))
	}

	// UI Web Routes
	s.handle(RouteRoot, s.handleLanding)
	s.handle(RouteInstallScript, s.handleInstallScript)
	s.handle(RouteDocs, s.handleDocs)
	s.handle(RouteDocsPrefix, s.handleDocs)
	s.handle(RouteUIAuth, s.handleAuth)
	s.handle(RouteUIDashboard, s.handleDashboard)
	s.handle(RouteUIDashboardPrefix, s.handleDashboard)

	// Top-level shortcuts to dashboard views
	for _, view := range []string{"overview", "flags", "analytics", "evaluator", "benchmark", "audit", "sdk", "profile", "settings"} {
		v := view
		s.handle("/"+v, func(w http.ResponseWriter, r *http.Request) {
			target := RouteUIDashboardPrefix + v
			if v == "overview" {
				target = RouteUIDashboard
			}
			if q := r.URL.Query().Encode(); q != "" {
				target += "?" + q
			}
			// #nosec G710 -- target path is strictly constrained to internal /dashboard route with sanitized query
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		})
	}

	// Auth API Routes (Rate limited for brute-force protection)
	s.handle(RouteAuthSignUp, s.handleSignUp, s.authLimiter.LimitHandler)
	s.handle(RouteAuthLogin, s.handleLogin, s.authLimiter.LimitHandler)
	s.handle(RouteAuthForgotPassword, s.handleForgotPassword, s.authLimiter.LimitHandler)
	s.handle(RouteAuthResetPassword, s.handleResetPassword, s.authLimiter.LimitHandler)
	s.handle(RouteAuthLogout, s.handleLogout)
	s.handle(RouteAuthMe, s.handleMe)
	s.handle(RouteAuthProfile, s.handleUpdateProfile, s.RequireAuth)
	s.handle(RouteAuthChangePassword, s.handleChangePassword, s.authLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAuthOIDCLogin, s.handleOIDCLogin, s.authLimiter.LimitHandler)
	s.handle(RouteAuthOIDCCallback, s.handleOIDCCallback, s.authLimiter.LimitHandler)

	// Public Observability & Webhook Routes
	s.handle(RouteHealth, s.handleHealth)
	s.handle(RouteHealthz, s.handleHealthz)
	s.handle(RouteAPIHealth, s.handleHealthz)
	s.handle(RouteAPIV1Health, s.handleHealth)
	s.handle(RouteLivez, s.handleLivez)
	s.handle(RouteReadyz, s.handleReadyz)
	s.handle(RouteMetrics, s.handleMetrics)
	s.handle(RouteAPIFlagsStream, s.handleFlagsStream)
	s.handle(RouteAPITelemetryEvents, s.handleIngestTelemetry, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPITelemetryStats, s.handleGetTelemetryStats, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPITelemetryStatsPrefix, s.handleGetTelemetryStats, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIWebhooksKillSwitch, s.handleWebhookKillSwitch, s.apiLimiter.LimitHandler)

	// Flag Management API Routes
	s.handleMethods(RouteAPIFlags, map[string]http.HandlerFunc{
		http.MethodGet:  s.handleGetFlags,
		http.MethodPost: s.handleCreateFlag,
	}, s.apiLimiter.LimitHandler, s.RequireAuth)

	s.handle(RouteAPIFlagsPrefix, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/canary") {
			s.handleCanaryRoutes(w, r)
			return
		}
		if strings.HasSuffix(path, "/toggle") {
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				s.RequireAuth(s.handleToggleFlag)(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/rollout") {
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				s.RequireAuth(s.handleUpdateRollout)(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/promote") {
			if r.Method == http.MethodPost {
				s.RequireAuth(s.handlePromoteEnvironment)(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/experiment") {
			if r.Method == http.MethodGet {
				s.RequireAuth(s.handleGetExperimentReport)(w, r)
				return
			}
		}

		switch r.Method {
		case http.MethodPut, http.MethodPatch, http.MethodPost:
			s.RequireAuth(s.handleUpdateFlag)(w, r)
		case http.MethodDelete:
			s.RequireAuth(s.handleDeleteFlag)(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}, s.apiLimiter.LimitHandler)

	// Organizations & Projects API Routes
	s.handleMethods(RouteAPIOrganizations, map[string]http.HandlerFunc{
		http.MethodGet:  s.handleListOrganizations,
		http.MethodPost: s.handleCreateOrganization,
	}, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIOrganizationsPrefix, s.handleOIDCConfigRoutes, s.apiLimiter.LimitHandler, s.RequireAuth)

	s.handleMethods(RouteAPIProjects, map[string]http.HandlerFunc{
		http.MethodGet:  s.handleListProjects,
		http.MethodPost: s.handleCreateProject,
	}, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIProjectsActive, s.handleSwitchActiveProject, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIProjectsPrefix, s.handleGetProject, s.apiLimiter.LimitHandler, s.RequireAuth)

	s.handle(RouteAPIInvitations, s.handleInvitations, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIInvitationsAccept, s.handleAcceptInvitation, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIInvitationsPrefix, s.handleGetInvitationByToken, s.apiLimiter.LimitHandler)

	s.handle(RouteAPIEvents, s.handleIngestEvents, s.apiLimiter.LimitHandler)
	s.handle(RouteAPIExperimentsPrefix, s.handleGetExperimentReport, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIChangeRequests, s.handleListOrCreateChangeRequests, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIChangeRequestsPrefix, s.handleChangeRequestItem, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIKeys, s.handleListOrCreateAPIKeys, s.apiLimiter.LimitHandler, s.RequireAuth)
	s.handle(RouteAPIKeysPrefix, s.handleRevokeAPIKey, s.apiLimiter.LimitHandler, s.RequireAuth)

	// Evaluation & Benchmarking
	s.handleMethods(RouteAPIEvaluate, map[string]http.HandlerFunc{
		http.MethodPost: s.handleEvaluate,
	}, s.apiLimiter.LimitHandler)

	s.handleMethods(RouteAPIBenchmark, map[string]http.HandlerFunc{
		http.MethodPost: s.handleBenchmark,
	}, s.apiLimiter.LimitHandler)

	// Audit Logs & Disaster Recovery Backup
	s.handle(RouteAPIAuditLogs, s.handleGetAuditLogs, s.RequireAuth)
	s.handle(RouteAPIAuditVerify, s.handleVerifyAuditChain, s.RequireAuth)
	s.handle(RouteAPIAuditExport, s.handleExportAuditLogs, s.RequireAuth)
	s.handle(RouteAPIAuditPurge, s.handlePurgeAuditLogs, s.RequireAuth, s.RequireAdmin)
	s.handle(RouteAPIBackupExport, s.handleExportBackup, s.RequireAuth)
	s.handle(RouteAPIBackupImport, s.handleImportBackup, s.RequireAuth)
	s.handleMethods(RouteAPIReset, map[string]http.HandlerFunc{
		http.MethodPost: s.handleReset,
	}, s.RequireAuth, s.RequireAdmin)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowedOrigin := os.Getenv("FLAGURA_ALLOWED_ORIGIN")
	if allowedOrigin != "" {
		if allowedOrigin == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			for _, ao := range strings.Split(allowedOrigin, ",") {
				ao = strings.TrimSpace(ao)
				if ao == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Vary", "Origin")
					break
				}
			}
		}
	} else if origin != "" {
		// By default only allow exact same-host origins or localhost/127.0.0.1 in development
		if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
			if parsed.Host == r.Host || parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
		}
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Webhook-Secret, X-Actor, X-Project-ID, X-Organization-ID, traceparent, tracestate")
	w.Header().Set("Access-Control-Expose-Headers", "X-Trace-ID, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.handler.ServeHTTP(w, r)
}

// writeError outputs a structured application error response adhering to Google/Stripe API standards.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	appErr := domain.MapSentinelToAppError(err)
	reqID := RequestIDFromContext(r.Context())

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(appErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":       appErr.Code,
			"type":       appErr.Type,
			"layer":      appErr.Layer,
			"message":    appErr.Message,
			"status":     appErr.HTTPStatus,
			"request_id": reqID,
		},
	})
}

// writeJSON serializes data to JSON with standard Content-Type header.
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
