package views

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestTemplComponentsRender(t *testing.T) {
	ctx := context.Background()

	// Mock sample data
	u := &domain.User{
		ID:        "usr_mock_1",
		Email:     "dev@flagura.dev",
		Name:      "Lead Engineer",
		Role:      domain.RoleAdmin,
		CreatedAt: time.Now(),
	}

	flags := []domain.FeatureFlag{
		{
			ID:          "flg_1",
			ProjectID:   "proj_default",
			Key:         "ai-smart-search",
			Name:        "AI Smart Search",
			Type:        "boolean",
			Description: "Next-gen vector neural search",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:    true,
					Strategy:   domain.StrategyPercentage,
					Percentage: 50,
				},
			},
		},
	}

	auditLogs := []domain.AuditLogEntry{
		{
			ID:          "log_1",
			ProjectID:   "proj_default",
			FlagKey:     "ai-smart-search",
			Action:      "TOGGLE",
			Environment: domain.EnvProduction,
			Actor:       "dev@flagura.dev",
			Details:     "Enabled flag in production",
			Timestamp:   time.Now(),
		},
	}

	changeRequests := []domain.ChangeRequest{
		{
			ID:           "cr_1",
			ProjectID:    "proj_default",
			FlagKey:      "ai-smart-search",
			Environment:  domain.EnvProduction,
			Title:        "Ramp up AI search to 100%",
			AuthorUserID: "usr_mock_1",
			AuthorEmail:  "dev@flagura.dev",
			AuthorName:   "Lead Engineer",
			Status:       domain.ChangeRequestStatusPending,
		},
	}

	orgs := []domain.Organization{
		{ID: "org_1", Name: "Acme Corp", Slug: "acme-corp"},
	}

	projects := []domain.Project{
		{ID: "proj_1", OrganizationID: "org_1", Name: "Payments API", Slug: "payments-api"},
	}

	tests := []struct {
		name      string
		component templ.Component
	}{
		{name: "LandingPage", component: LandingPage()},
		{name: "AuthPage", component: AuthPage()},
		{name: "Dashboard", component: Dashboard(u, flags, auditLogs, changeRequests, "In-Memory Edge Store", orgs, projects, "proj_1")},
		{name: "BentoOverview", component: BentoOverview(flags, auditLogs, "In-Memory Edge Store")},
		{name: "FlagMatrix", component: FlagMatrix(flags)},
		{name: "AnalyticsDashboard", component: AnalyticsDashboard(flags)},
		{name: "LiveEvaluator", component: LiveEvaluator(flags)},
		{name: "GovernanceModal", component: GovernanceModal(u, changeRequests)},
		{name: "ProjectModal", component: ProjectModal(orgs, projects, "proj_1")},
		{name: "SDKModal", component: SDKModal()},
		{name: "BenchmarkModal", component: BenchmarkModal()},
		{name: "AuditModal", component: AuditModal(auditLogs)},
		{name: "ExperimentModal", component: ExperimentModal()},
		{name: "FlagEditorModal", component: FlagEditorModal()},
		{name: "HeaderBar", component: HeaderBar(u, flags, "production")},
		{name: "Sidebar", component: Sidebar(u, flags, auditLogs, orgs, projects, "proj_1")},
		{name: "ProfileSettings", component: ProfileSettings(u, orgs, projects)},
		{name: "CustomCursor", component: CustomCursor()},
		{name: "ThreeCanvas", component: ThreeCanvas()},
		{name: "Layout", component: Layout("Flagura Control Plane")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tt.component.Render(ctx, &buf); err != nil {
				t.Fatalf("%s render failed: %v", tt.name, err)
			}
			if buf.Len() == 0 {
				t.Errorf("%s rendered empty output", tt.name)
			}
		})
	}
}
