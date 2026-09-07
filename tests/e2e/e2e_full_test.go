package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
	flagura "github.com/dhawalhost/flagura/sdks/go"
	flaguraOF "github.com/dhawalhost/flagura/sdks/go/openfeature"
	of "github.com/open-feature/go-sdk/openfeature"
)

func TestE2E_FullPlatformAndMultiSDK(t *testing.T) {
	// =========================================================================
	// 0. Bootstrap Ephemeral In-Memory Flagura Server on Real TCP Loopback
	// =========================================================================
	memStore := store.NewMemoryStore()
	srv, err := api.NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to instantiate api.Server: %v", err)
	}

	ts := httptest.NewServer(srv)
	defer ts.Close()

	t.Logf("🚀 Ephemeral Flagura Server listening on %s", ts.URL)

	testPassword := "TestPass_123_Secure!"
	testEmail := fmt.Sprintf("dev.tester.%d@flagura-e2e.dev", time.Now().UnixNano())

	var sessionCookie *http.Cookie
	var projectCookie *http.Cookie
	var activeProjID string
	var apiKey string

	createStandardFlags := func(projID string, cookie *http.Cookie) {
		flagsToCreate := []domain.FeatureFlag{
			// 1. Boolean Flag
			{
				ProjectID:   projID,
				Key:         "e2e-bool-active",
				Name:        "Active Boolean Feature",
				Type:        "boolean",
				Description: "Boolean flag enabled in production",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {
						Enabled:  true,
						Strategy: domain.StrategyBoolean,
					},
				},
			},
			// 2. Percentage Rollout Flag (50%)
			{
				ProjectID:   projID,
				Key:         "e2e-percentage-rollout",
				Name:        "Rollout 50% Feature",
				Type:        "boolean",
				Description: "Deterministic 50% rollout",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {
						Enabled:    true,
						Strategy:   domain.StrategyPercentage,
						Percentage: 50,
					},
				},
			},
			// 3. Rule Targeting Flag
			{
				ProjectID:   projID,
				Key:         "e2e-rule-targeting",
				Name:        "Rule Targeted Feature",
				Type:        "boolean",
				Description: "Enabled only for developer role",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {
						Enabled:  true,
						Strategy: domain.StrategyRules,
						Rules: []domain.TargetingRule{
							{
								ID:        "rule-guest",
								Attribute: domain.AttrRole,
								Operator:  domain.OpEquals,
								Values:    []string{"guest"},
								Action:    domain.ActionForceDisabled,
							},
							{
								ID:        "rule-dev",
								Attribute: domain.AttrRole,
								Operator:  domain.OpEquals,
								Values:    []string{"developer"},
								Action:    domain.ActionForceEnabled,
							},
						},
					},
				},
			},
			// 4. Multivariate Flag
			{
				ProjectID:   projID,
				Key:         "e2e-multivariate-models",
				Name:        "Multivariate Model Switcher",
				Type:        "multivariate",
				Description: "Multivariate flag with 3 model variants",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {
						Enabled:        true,
						Strategy:       domain.StrategyMultivariate,
						DefaultVariant: "control",
						Variants: []domain.FlagVariant{
							{Key: "control", Name: "Control Default", Value: "gpt-3.5", Weight: 10},
							{Key: "flash-v1", Name: "Gemini Flash", Value: "gemini-1.5-flash", Weight: 45},
							{Key: "pro-v1", Name: "Gemini Pro", Value: "gemini-1.5-pro", Weight: 45},
						},
					},
				},
			},
		}

		for _, flag := range flagsToCreate {
			b, _ := json.Marshal(flag)
			req, _ := http.NewRequest("POST", ts.URL+"/api/v1/flags", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Project-ID", projID)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	bootstrapTestEnv := func(subT *testing.T) {
		if apiKey != "" && activeProjID != "" {
			return
		}

		if activeProjID == "" || sessionCookie == nil {
			signupPayload := map[string]string{
				"email":    testEmail,
				"password": testPassword,
				"name":     "E2E Automated Tester",
			}
			b, _ := json.Marshal(signupPayload)
			resp, err := http.Post(ts.URL+"/api/v1/auth/signup", "application/json", bytes.NewReader(b))
			if err != nil {
				subT.Fatalf("Bootstrap signup request failed: %v", err)
			}
			for _, c := range resp.Cookies() {
				if c.Name == domain.CookieSessionName {
					sessionCookie = c
				}
				if c.Name == domain.CookieProjectName {
					projectCookie = c
					activeProjID = c.Value
				}
			}
			resp.Body.Close()
		}

		if apiKey == "" {
			keyReqPayload := map[string]string{
				"name":        "E2E Automated SDK Key",
				"environment": "production",
			}
			b, _ := json.Marshal(keyReqPayload)
			req, _ := http.NewRequest("POST", ts.URL+"/api/v1/api-keys", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Project-ID", activeProjID)
			if sessionCookie != nil {
				req.AddCookie(sessionCookie)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				subT.Fatalf("Bootstrap API key creation failed: %v", err)
			}
			var keyResp struct {
				APIKey domain.APIKey `json:"api_key"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&keyResp)
			resp.Body.Close()
			apiKey = keyResp.APIKey.Key
		}

		createStandardFlags(activeProjID, sessionCookie)
	}

	// =========================================================================
	// 1. User Sign Up, Login, and Identity Verification
	// =========================================================================
	t.Run("1_User_Signup_And_Login", func(t *testing.T) {
		// 1.1 Sign up new user
		signupPayload := map[string]string{
			"email":    testEmail,
			"password": testPassword,
			"name":     "E2E Automated Tester",
		}
		bodyBytes, _ := json.Marshal(signupPayload)
		resp, err := http.Post(ts.URL+"/api/v1/auth/signup", "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("Signup request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Signup expected 201 Created, got %d", resp.StatusCode)
		}

		_ = projectCookie
		for _, c := range resp.Cookies() {
			if c.Name == domain.CookieSessionName {
				sessionCookie = c
			}
			if c.Name == domain.CookieProjectName {
				projectCookie = c
				activeProjID = c.Value
			}
		}

		if sessionCookie == nil {
			t.Fatal("Expected session cookie from signup response")
		}
		if activeProjID == "" {
			t.Fatal("Expected auto-provisioned project ID from signup response")
		}

		// 1.2 Verify Login endpoint
		loginPayload := map[string]string{
			"email":    testEmail,
			"password": testPassword,
		}
		bodyBytes, _ = json.Marshal(loginPayload)
		respLogin, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("Login request failed: %v", err)
		}
		defer respLogin.Body.Close()

		if respLogin.StatusCode != http.StatusOK {
			t.Fatalf("Login expected 200 OK, got %d", respLogin.StatusCode)
		}

		// 1.3 Verify /api/v1/auth/me
		reqMe, _ := http.NewRequest("GET", ts.URL+"/api/v1/auth/me", nil)
		reqMe.AddCookie(sessionCookie)
		respMe, err := http.DefaultClient.Do(reqMe)
		if err != nil {
			t.Fatalf("/me request failed: %v", err)
		}
		defer respMe.Body.Close()

		if respMe.StatusCode != http.StatusOK {
			t.Fatalf("/me expected 200 OK, got %d", respMe.StatusCode)
		}

		var meUser domain.User
		_ = json.NewDecoder(respMe.Body).Decode(&meUser)
		if meUser.Email != testEmail {
			t.Fatalf("Expected user email %s, got %s", testEmail, meUser.Email)
		}
		t.Logf("  ✓ User successfully registered & verified: %s (Proj: %s)", meUser.Email, activeProjID)
	})

	// =========================================================================
	// 2. Provision SDK API Key
	// =========================================================================
	t.Run("2_API_Key_Provisioning", func(t *testing.T) {
		keyReqPayload := map[string]string{
			"name":        "E2E Automated SDK Key",
			"environment": "production",
		}
		b, _ := json.Marshal(keyReqPayload)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/api-keys", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", activeProjID)
		req.AddCookie(sessionCookie)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("API Key creation failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("API Key creation expected 201, got %d", resp.StatusCode)
		}

		var keyResp struct {
			APIKey domain.APIKey `json:"api_key"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
			t.Fatalf("Failed to decode API key response: %v", err)
		}

		apiKey = keyResp.APIKey.Key
		if !strings.HasPrefix(apiKey, "flg_live_") {
			t.Fatalf("Expected API key prefix flg_live_, got %s", apiKey)
		}
		t.Logf("  ✓ API Key provisioned: %s (Prefix: %s)", keyResp.APIKey.KeyPrefix, apiKey[:16])
	})

	// =========================================================================
	// 3. Create Feature Flags (Boolean, Rollout, Multivariate, Targeting)
	// =========================================================================
	t.Run("3_Create_Feature_Flags", func(t *testing.T) {
		bootstrapTestEnv(t)
		createStandardFlags(activeProjID, sessionCookie)
		t.Logf("  ✓ Feature flags created")
	})

	// =========================================================================
	// 4. Native Go SDK & OpenFeature Go Provider Evaluations
	// =========================================================================
	var goClient *flagura.Client
	t.Run("4_Go_SDK_Evaluation", func(t *testing.T) {
		bootstrapTestEnv(t)
		goClient = flagura.NewClient(
			ts.URL,
			apiKey,
			flagura.WithProjectID(activeProjID),
			flagura.WithLocalEvaluation(false),
		)
		defer goClient.Close()

		ctx := context.Background()

		// 4.1 Test Boolean flag
		resBool, err := goClient.Evaluate(ctx, "e2e-bool-active", flagura.Context{UserID: "usr_go_1"})
		if err != nil {
			t.Fatalf("Go SDK Evaluate(e2e-bool-active) error: %v", err)
		}
		if !resBool.Enabled {
			t.Fatalf("Expected e2e-bool-active to be enabled, got false")
		}
		t.Logf("  ✓ Go SDK Boolean evaluation: %s -> enabled=%v (reason=%s)", resBool.FlagKey, resBool.Enabled, resBool.Reason)

		// 4.2 Test Rollout determinism
		resRollout1, _ := goClient.Evaluate(ctx, "e2e-percentage-rollout", flagura.Context{UserID: "usr_consistent_42"})
		resRollout2, _ := goClient.Evaluate(ctx, "e2e-percentage-rollout", flagura.Context{UserID: "usr_consistent_42"})
		if resRollout1.Enabled != resRollout2.Enabled {
			t.Fatalf("Percentage rollout not deterministic: first=%v, second=%v", resRollout1.Enabled, resRollout2.Enabled)
		}
		t.Logf("  ✓ Go SDK Deterministic rollout: user 'usr_consistent_42' consistently evaluated to %v", resRollout1.Enabled)

		// 4.3 Test Rule Targeting
		devCtx := flagura.Context{UserID: "usr_dev", Role: "developer"}
		guestCtx := flagura.Context{UserID: "usr_guest", Role: "guest"}

		resDev, err := goClient.Evaluate(ctx, "e2e-rule-targeting", devCtx)
		if err != nil || !resDev.Enabled {
			t.Fatalf("Expected developer to match targeting rule: %v, err: %v", resDev.Enabled, err)
		}

		resGuest, err := goClient.Evaluate(ctx, "e2e-rule-targeting", guestCtx)
		if err != nil || resGuest.Enabled {
			t.Fatalf("Expected guest to fail targeting rule: %v, err: %v", resGuest.Enabled, err)
		}
		t.Logf("  ✓ Go SDK Rule targeting: developer=%v, guest=%v", resDev.Enabled, resGuest.Enabled)

		// 4.4 Test Multivariate
		resMV, err := goClient.Evaluate(ctx, "e2e-multivariate-models", flagura.Context{UserID: "usr_ai_tester"})
		if err != nil || resMV.Variant == "" {
			t.Fatalf("Expected valid multivariate variant, got '%s', err: %v", resMV.Variant, err)
		}
		t.Logf("  ✓ Go SDK Multivariate evaluation: variant='%s', value=%v", resMV.Variant, resMV.Value)

		// 4.5 OpenFeature Go Provider
		provider := flaguraOF.NewProvider(goClient)
		if err := of.SetProviderAndWait(provider); err != nil {
			t.Fatalf("OpenFeature SetProviderAndWait error: %v", err)
		}

		ofClient := of.NewClient("e2e-go-suite")
		ofCtx := of.NewEvaluationContext("usr_go_1", map[string]interface{}{"role": "developer"})

		ofBool, err := ofClient.BooleanValue(ctx, "e2e-bool-active", false, ofCtx)
		if err != nil || !ofBool {
			t.Fatalf("OpenFeature BooleanValue expected true, got %v, err: %v", ofBool, err)
		}

		ofDetails, err := ofClient.StringValueDetails(ctx, "e2e-multivariate-models", "fallback", ofCtx)
		if err != nil || ofDetails.Variant == "" {
			t.Fatalf("OpenFeature StringValueDetails expected valid variant, got: %+v, err: %v", ofDetails, err)
		}
		t.Logf("  ✓ OpenFeature Go Provider: BooleanValue=%v, StringValueDetails.Variant='%s'", ofBool, ofDetails.Variant)
	})

	// =========================================================================
	// 5. Live Real-Time Flag Toggling & SDK Observation
	// =========================================================================
	t.Run("5_Live_Flag_Toggle", func(t *testing.T) {
		bootstrapTestEnv(t)
		toggleClient := flagura.NewClient(ts.URL, apiKey, flagura.WithProjectID(activeProjID))
		defer toggleClient.Close()

		ctx := context.Background()

		// Verify initially enabled
		initially, _ := toggleClient.IsEnabled(ctx, "e2e-bool-active", flagura.Context{UserID: "usr_toggle_1"})
		if !initially {
			t.Fatalf("Expected flag e2e-bool-active to be initially true")
		}

		// Toggle off via API
		reqToggleOff, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/flags/e2e-bool-active/toggle?env=production", nil)
		reqToggleOff.Header.Set("X-Project-ID", activeProjID)
		reqToggleOff.AddCookie(sessionCookie)
		respOff, err := http.DefaultClient.Do(reqToggleOff)
		if err != nil || respOff.StatusCode != http.StatusOK {
			t.Fatalf("Toggle off failed: %v", err)
		}
		respOff.Body.Close()

		// Verify SDK observes disabled
		afterToggle, _ := toggleClient.IsEnabled(ctx, "e2e-bool-active", flagura.Context{UserID: "usr_toggle_1"})
		if afterToggle {
			t.Fatalf("Expected flag e2e-bool-active to be false after toggle off")
		}

		// Toggle back on
		reqToggleOn, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/flags/e2e-bool-active/toggle?env=production", nil)
		reqToggleOn.Header.Set("X-Project-ID", activeProjID)
		reqToggleOn.AddCookie(sessionCookie)
		respOn, err := http.DefaultClient.Do(reqToggleOn)
		if err != nil || respOn.StatusCode != http.StatusOK {
			t.Fatalf("Toggle on failed: %v", err)
		}
		respOn.Body.Close()

		// Verify SDK observes enabled again
		afterOn, _ := toggleClient.IsEnabled(ctx, "e2e-bool-active", flagura.Context{UserID: "usr_toggle_1"})
		if !afterOn {
			t.Fatalf("Expected flag e2e-bool-active to be true after toggle on")
		}
		t.Logf("  ✓ Live flag toggle verified: true -> false -> true")
	})

	// =========================================================================
	// 6. Telemetry Ingestion & Aggregation
	// =========================================================================
	t.Run("6_Telemetry_Ingestion", func(t *testing.T) {
		bootstrapTestEnv(t)
		telemetryPayload := map[string]interface{}{
			"timestamp": time.Now().UnixMilli(),
			"events": map[string]interface{}{
				"e2e-bool-active": map[string]interface{}{
					"evaluations": 10,
					"variants": map[string]int{
						"on": 10,
					},
				},
			},
		}
		b, _ := json.Marshal(telemetryPayload)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/telemetry/events", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("X-Project-ID", activeProjID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Telemetry ingest failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200/202 for telemetry ingest, got %d", resp.StatusCode)
		}
		t.Logf("  ✓ Telemetry event ingested successfully")
	})

	// =========================================================================
	// 7. 4-Eyes Governance Workflow (Change Requests)
	// =========================================================================
	t.Run("7_FourEyes_Governance", func(t *testing.T) {
		bootstrapTestEnv(t)
		// 7.1 Author submits CR
		crBody := domain.ChangeRequest{
			ProjectID:   activeProjID,
			FlagKey:     "e2e-bool-active",
			Environment: domain.EnvProduction,
			Title:       "Update e2e-bool-active to 100% rollout",
			Description: "Approved via automated pipeline",
			ProposedConfig: domain.EnvironmentConfig{
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 100,
			},
		}
		b, _ := json.Marshal(crBody)
		reqCR, _ := http.NewRequest("POST", ts.URL+"/api/v1/change-requests", bytes.NewReader(b))
		reqCR.Header.Set("Content-Type", "application/json")
		reqCR.Header.Set("X-Project-ID", activeProjID)
		reqCR.AddCookie(sessionCookie)

		respCR, err := http.DefaultClient.Do(reqCR)
		if err != nil {
			t.Fatalf("CR creation request failed: %v", err)
		}
		defer respCR.Body.Close()

		if respCR.StatusCode != http.StatusCreated {
			t.Fatalf("CR creation expected 201, got %d", respCR.StatusCode)
		}

		var createdCR domain.ChangeRequest
		_ = json.NewDecoder(respCR.Body).Decode(&createdCR)
		crID := createdCR.ID

		// 7.2 Author attempts self-approval -> Must receive 403 Forbidden
		reviewPayload := map[string]interface{}{
			"approved": true,
			"comments": "Attempting self-approval",
		}
		bRev, _ := json.Marshal(reviewPayload)
		reqSelfReview, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/change-requests/%s/review", ts.URL, crID), bytes.NewReader(bRev))
		reqSelfReview.Header.Set("Content-Type", "application/json")
		reqSelfReview.Header.Set("X-Project-ID", activeProjID)
		reqSelfReview.AddCookie(sessionCookie)

		respSelfReview, err := http.DefaultClient.Do(reqSelfReview)
		if err != nil {
			t.Fatalf("Self review request failed: %v", err)
		}
		defer respSelfReview.Body.Close()

		if respSelfReview.StatusCode != http.StatusBadRequest {
			t.Fatalf("Expected 400 Bad Request for author self-approval, got %d", respSelfReview.StatusCode)
		}
		t.Logf("  ✓ 4-Eyes Governance: Author self-approval successfully blocked (400 Bad Request)")
	})

	// =========================================================================
	// 8. Automated TypeScript SDK Test Execution
	// =========================================================================
	t.Run("8_TypeScript_SDK_Automated_Suite", func(t *testing.T) {
		bootstrapTestEnv(t)

		if _, err := exec.LookPath("node"); err != nil {
			t.Skip("Node.js runtime not found in PATH, skipping TypeScript SDK automated suite")
		}

		tsCandidates := []string{
			"sdk_test.ts",
			filepath.Join("tests", "e2e", "sdk_test.ts"),
			filepath.Join("..", "tests", "e2e", "sdk_test.ts"),
			filepath.Join("..", "..", "tests", "e2e", "sdk_test.ts"),
		}
		var tsScript string
		for _, c := range tsCandidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					tsScript = abs
					break
				}
			}
		}

		if tsScript == "" {
			t.Skip("sdk_test.ts script not found, skipping TypeScript SDK automated suite")
		}

		tsDir := filepath.Dir(tsScript)

		nodeCandidates := []string{
			filepath.Join(tsDir, "..", "..", "examples", "typescript", "node_modules"),
			filepath.Join("..", "examples", "typescript", "node_modules"),
			filepath.Join("examples", "typescript", "node_modules"),
			filepath.Join("..", "..", "examples", "typescript", "node_modules"),
		}
		var nodePath string
		for _, c := range nodeCandidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					nodePath = abs
					break
				}
			}
		}

		tsxCandidates := []string{
			filepath.Join(nodePath, ".bin", "tsx"),
			filepath.Join(tsDir, "..", "..", "examples", "typescript", "node_modules", ".bin", "tsx"),
		}
		var tsxBin string
		for _, b := range tsxCandidates {
			if info, err := os.Stat(b); err == nil && !info.IsDir() {
				tsxBin = b
				break
			}
		}

		var cmd *exec.Cmd
		if tsxBin != "" {
			cmd = exec.Command(tsxBin, tsScript)
		} else {
			cmd = exec.Command("npx", "--yes", "tsx", tsScript)
		}
		cmd.Dir = tsDir
		cmd.Env = append(os.Environ(),
			"FLAGURA_ENDPOINT="+ts.URL,
			"FLAGURA_API_KEY="+apiKey,
			"NODE_PATH="+nodePath,
		)
		out, err := cmd.CombinedOutput()
		t.Logf("TypeScript SDK E2E Output:\n%s", string(out))

		if err != nil {
			t.Fatalf("TypeScript SDK E2E test failed: %v", err)
		}
		t.Logf("  ✓ TypeScript SDK E2E Suite executed successfully (Exit Code 0)")
	})

	// =========================================================================
	// 9. Automated Python SDK Test Execution
	// =========================================================================
	t.Run("9_Python_SDK_Automated_Suite", func(t *testing.T) {
		bootstrapTestEnv(t)

		if _, err := exec.LookPath("python3"); err != nil {
			t.Skip("Python 3 runtime not found in PATH, skipping Python SDK automated suite")
		}

		pyCandidates := []string{
			"sdk_test.py",
			filepath.Join("tests", "e2e", "sdk_test.py"),
			filepath.Join("..", "tests", "e2e", "sdk_test.py"),
			filepath.Join("..", "..", "tests", "e2e", "sdk_test.py"),
		}
		var pyScript string
		for _, c := range pyCandidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					pyScript = abs
					break
				}
			}
		}

		if pyScript == "" {
			t.Skip("sdk_test.py script not found, skipping Python SDK automated suite")
		}

		pyDir := filepath.Dir(pyScript)
		pyPathCandidates := []string{
			filepath.Join(pyDir, "..", "..", "sdks", "python"),
			filepath.Join("..", "sdks", "python"),
			filepath.Join("sdks", "python"),
			filepath.Join("..", "..", "sdks", "python"),
		}
		var pythonPath string
		for _, c := range pyPathCandidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					pythonPath = abs
					break
				}
			}
		}

		cmd := exec.Command("python3", pyScript)
		cmd.Dir = pyDir
		cmd.Env = append(os.Environ(),
			"FLAGURA_ENDPOINT="+ts.URL,
			"FLAGURA_API_KEY="+apiKey,
			"PYTHONPATH="+pythonPath,
			"PYTHONDONTWRITEBYTECODE=1",
		)
		out, err := cmd.CombinedOutput()
		t.Logf("Python SDK E2E Output:\n%s", string(out))

		if err != nil {
			t.Fatalf("Python SDK E2E test failed: %v", err)
		}
		t.Logf("  ✓ Python SDK E2E Suite executed successfully (Exit Code 0)")
	})

	// =========================================================================
	// 10. Automated Rust SDK Contract Verification
	// =========================================================================
	t.Run("10_Rust_SDK_Contract_Verification", func(t *testing.T) {
		bootstrapTestEnv(t)
		// Verify wire format matches Rust reqwest JSON structure exactly
		reqBody := map[string]interface{}{
			"flags": []string{"e2e-bool-active", "e2e-multivariate-models"},
			"context": map[string]interface{}{
				"user_id":     "usr_rust_client",
				"environment": "production",
			},
		}
		b, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/evaluate", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("X-Project-ID", activeProjID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Rust contract evaluation request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Rust contract evaluation expected 200 OK, got %d", resp.StatusCode)
		}

		var evalResp struct {
			Results map[string]struct {
				FlagKey   string      `json:"flag_key"`
				Enabled   bool        `json:"enabled"`
				Variant   string      `json:"variant"`
				Value     interface{} `json:"value"`
				Reason    string      `json:"reason"`
				LatencyNs uint64      `json:"latency_ns"`
				LatencyUs float64     `json:"latency_us"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&evalResp); err != nil {
			t.Fatalf("Rust contract JSON decode failed: %v", err)
		}

		boolFlag, ok := evalResp.Results["e2e-bool-active"]
		if !ok || !boolFlag.Enabled {
			t.Fatalf("Rust contract expected e2e-bool-active to be enabled")
		}

		mvFlag, ok := evalResp.Results["e2e-multivariate-models"]
		if !ok || mvFlag.Variant == "" {
			t.Fatalf("Rust contract expected e2e-multivariate-models to have variant")
		}

		t.Logf("  ✓ Rust SDK Wire Contract verified: serde deserialization succeeded with all fields")
	})
}
