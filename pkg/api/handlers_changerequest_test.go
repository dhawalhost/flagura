package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestFourEyesChangeRequestGovernanceFlow(t *testing.T) {
	memStore := store.NewMemoryStore()
	flagKey := "prod-database-failover"

	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:        "flag_failover",
		ProjectID: store.DefaultProjectID,
		Key:       flagKey,
		Name:      "Production Database Failover",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    false,
				Strategy:   domain.StrategyBoolean,
				Percentage: 0,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, "system")

	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Create Author (Developer A)
	authorUser, _ := memStore.CreateUser(context.Background(), domain.User{
		ID:           "usr_dev_a",
		Name:         "Alice Developer",
		Email:        "alice@flagura.dev",
		PasswordHash: "fakehash",
		Role:         domain.RoleDeveloper,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	})
	authorToken := "author_session_token"
	_ = memStore.CreateSession(context.Background(), domain.Session{
		Token:     authorToken,
		UserID:    authorUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	})
	authorCookie := &http.Cookie{Name: SessionCookieName, Value: authorToken}

	// 2. Create Reviewer (Admin B)
	reviewerUser, _ := memStore.CreateUser(context.Background(), domain.User{
		ID:           "usr_admin_b",
		Name:         "Bob Lead",
		Email:        "bob@flagura.dev",
		PasswordHash: "fakehash",
		Role:         domain.RoleAdmin,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	})
	reviewerToken := "reviewer_session_token"
	_ = memStore.CreateSession(context.Background(), domain.Session{
		Token:     reviewerToken,
		UserID:    reviewerUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	})
	reviewerCookie := &http.Cookie{Name: SessionCookieName, Value: reviewerToken}

	// 3. Alice submits ChangeRequest via POST /api/v1/change-requests
	crPayload := domain.ChangeRequest{
		FlagKey:     flagKey,
		Environment: domain.EnvProduction,
		Title:       "Enable DB failover for maintenance",
		Description: "Scheduled maintenance window failover to read replica cluster",
		ProposedConfig: domain.EnvironmentConfig{
			Enabled:    true,
			Strategy:   domain.StrategyBoolean,
			Percentage: 100,
		},
	}
	crData, _ := json.Marshal(crPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/change-requests", bytes.NewReader(crData))
	req.AddCookie(authorCookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201 Created from POST /api/v1/change-requests, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var createdCR domain.ChangeRequest
	_ = json.Unmarshal(rec.Body.Bytes(), &createdCR)
	if createdCR.ID == "" || createdCR.Status != domain.ChangeRequestStatusPending {
		t.Fatalf("expected pending change request, got: %+v", createdCR)
	}

	// 4. Test 4-Eyes Principle: Alice attempts to approve her OWN change request (MUST FAIL!)
	selfReviewPayload := map[string]interface{}{"approved": true, "comments": "Self approved"}
	selfData, _ := json.Marshal(selfReviewPayload)
	selfReq := httptest.NewRequest(http.MethodPost, "/api/v1/change-requests/"+createdCR.ID+"/review", bytes.NewReader(selfData))
	selfReq.AddCookie(authorCookie)
	selfRec := httptest.NewRecorder()

	server.ServeHTTP(selfRec, selfReq)

	if selfRec.Code == http.StatusOK {
		t.Fatalf("expected failure when author self-approves change request, got HTTP 200 OK")
	}

	// 5. Bob (Reviewer) reviews and APPROVES the change request
	reviewPayload := map[string]interface{}{"approved": true, "comments": "Approved after reviewing maintenance ticket."}
	revData, _ := json.Marshal(reviewPayload)
	revReq := httptest.NewRequest(http.MethodPost, "/api/v1/change-requests/"+createdCR.ID+"/review", bytes.NewReader(revData))
	revReq.AddCookie(reviewerCookie)
	revRec := httptest.NewRecorder()

	server.ServeHTTP(revRec, revReq)

	if revRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on reviewer approval, got %d (body: %s)", revRec.Code, revRec.Body.String())
	}

	var approvedCR domain.ChangeRequest
	_ = json.Unmarshal(revRec.Body.Bytes(), &approvedCR)
	if approvedCR.Status != domain.ChangeRequestStatusApproved {
		t.Fatalf("expected status APPROVED, got %s", approvedCR.Status)
	}

	// 6. Bob applies the approved change request to production
	applyReq := httptest.NewRequest(http.MethodPost, "/api/v1/change-requests/"+createdCR.ID+"/apply", nil)
	applyReq.AddCookie(reviewerCookie)
	applyRec := httptest.NewRecorder()

	server.ServeHTTP(applyRec, applyReq)

	if applyRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on apply, got %d (body: %s)", applyRec.Code, applyRec.Body.String())
	}

	// Verify flag state in store is now Enabled = true, Percentage = 100
	flag, _ := memStore.GetFlag(context.Background(), flagKey)
	if !flag.Environments[domain.EnvProduction].Enabled || flag.Environments[domain.EnvProduction].Percentage != 100 {
		t.Fatalf("expected flag enabled in production after apply, got: %+v", flag.Environments[domain.EnvProduction])
	}
}
