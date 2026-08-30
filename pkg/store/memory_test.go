package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestMemoryStore_DeepCopyIsolation(t *testing.T) {
	memStore := store.NewMemoryStore()
	ctx := context.Background()

	// 1. Get initial flag
	flag1, err := memStore.GetFlag(ctx, "ai-smart-search")
	if err != nil {
		t.Fatalf("Failed to get flag: %v", err)
	}

	// 2. Mutate map directly on the returned struct copy WITHOUT calling SaveFlag
	prodConfig := flag1.Environments[domain.EnvProduction]
	prodConfig.Percentage = 99.99
	flag1.Environments[domain.EnvProduction] = prodConfig

	// 3. Fetch fresh copy from store
	flag2, err := memStore.GetFlag(ctx, "ai-smart-search")
	if err != nil {
		t.Fatalf("Failed to get flag second time: %v", err)
	}

	// 4. Assert that store state was NOT mutated by caller's map modification
	if flag2.Environments[domain.EnvProduction].Percentage == 99.99 {
		t.Fatalf("DATA INTEGRITY BUG: MemoryStore leaked mutable map reference! Percentage was modified without SaveFlag.")
	}

	// 5. Test ListFlags isolation
	flagsList, err := memStore.ListFlags(ctx)
	if err != nil {
		t.Fatalf("Failed to list flags: %v", err)
	}
	for _, f := range flagsList {
		if f.Key == "ai-smart-search" {
			f.Environments[domain.EnvProduction] = domain.EnvironmentConfig{Percentage: 1.23}
		}
	}

	flag3, _ := memStore.GetFlag(ctx, "ai-smart-search")
	if flag3.Environments[domain.EnvProduction].Percentage == 1.23 {
		t.Fatalf("DATA INTEGRITY BUG: ListFlags leaked mutable map reference!")
	}
}

func TestMemoryStore_ConcurrentReadWriteRace(t *testing.T) {
	memStore := store.NewMemoryStore()
	ctx := context.Background()

	const numGoroutines = 50
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Readers: GetFlag & ListFlags
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = memStore.ListFlags(ctx)
				flag, err := memStore.GetFlag(ctx, "ai-smart-search")
				if err == nil && flag != nil {
					// Read from map to ensure no race
					_ = flag.Environments[domain.EnvProduction].Percentage
				}
			}
		}(g)
	}

	// Writers: ToggleFlag & UpdateRollout
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				pct := float64((id*10 + i) % 100)
				_, _, _ = memStore.UpdateRollout(ctx, "ai-smart-search", domain.EnvProduction, pct, "concurrency-tester")
				enabled := (i % 2) == 0
				_, _, _ = memStore.ToggleFlag(ctx, "ai-smart-search", domain.EnvProduction, &enabled, "concurrency-tester")
			}
		}(g)
	}

	// Writers: SaveFlag
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				flag, err := memStore.GetFlag(ctx, "ai-smart-search")
				if err == nil && flag != nil {
					flag.Name = fmt.Sprintf("AI Smart Search (v%d.%d)", id, i)
					_, _ = memStore.SaveFlag(ctx, *flag, "concurrency-writer")
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestMemoryStore_AuditLogRetention(t *testing.T) {
	memStore := store.NewMemoryStore()
	ctx := context.Background()

	initialLogs, err := memStore.ListAuditLogs(ctx, 100)
	if err != nil {
		t.Fatalf("Failed to list audit logs: %v", err)
	}

	initialCount := len(initialLogs)

	// Perform operations that generate audit logs
	_, _, _ = memStore.UpdateRollout(ctx, "ai-smart-search", domain.EnvProduction, 80, "audit-tester")
	_, _, _ = memStore.ToggleFlag(ctx, "ai-smart-search", domain.EnvProduction, nil, "audit-tester")

	updatedLogs, err := memStore.ListAuditLogs(ctx, 100)
	if err != nil {
		t.Fatalf("Failed to list audit logs: %v", err)
	}

	if len(updatedLogs) != initialCount+2 {
		t.Fatalf("Expected %d audit logs after mutations, got %d", initialCount+2, len(updatedLogs))
	}
}

func BenchmarkMemoryStore_GetFlag_Atomic(b *testing.B) {
	memStore := store.NewMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = memStore.GetFlag(ctx, "ai-smart-search")
	}
}

func BenchmarkMemoryStore_ListFlags_Atomic(b *testing.B) {
	memStore := store.NewMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = memStore.ListFlags(ctx)
	}
}

func TestMemoryStore_OrgAndProjectHierarchy(t *testing.T) {
	memStore := store.NewMemoryStore()
	ctx := context.Background()

	// 1. Verify default org and default project are seeded
	orgs, err := memStore.ListOrganizations(ctx)
	if err != nil || len(orgs) == 0 {
		t.Fatalf("Expected seeded default organization, got err=%v, count=%d", err, len(orgs))
	}
	projects, err := memStore.ListProjects(ctx, store.DefaultOrgID)
	if err != nil || len(projects) == 0 {
		t.Fatalf("Expected seeded default project, got err=%v, count=%d", err, len(projects))
	}

	// 2. Create custom organization
	customOrg, err := memStore.CreateOrganization(ctx, domain.Organization{
		Name: "Acme Enterprise",
		Slug: "acme-enterprise",
	})
	if err != nil {
		t.Fatalf("Failed to create organization: %v", err)
	}

	// 3. Create custom project under Acme Enterprise
	customProj, err := memStore.CreateProject(ctx, domain.Project{
		OrganizationID: customOrg.ID,
		Name:           "Mobile Checkout v2",
		Slug:           "mobile-checkout-v2",
	})
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// 4. Add flag to custom project
	flagKey := "enable-apple-pay"
	_, err = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ProjectID:   customProj.ID,
		Key:         flagKey,
		Name:        "Enable Apple Pay",
		Type:        "boolean",
		Description: "One-touch Apple Pay in checkout",
	}, "test-author")
	if err != nil {
		t.Fatalf("Failed to save flag in custom project: %v", err)
	}

	// 5. Verify flag is listed in custom project
	projFlags, err := memStore.ListFlagsByProject(ctx, customProj.ID)
	if err != nil || len(projFlags) != 1 || projFlags[0].Key != flagKey {
		t.Fatalf("Expected 1 flag in custom project, got %d flags (%v)", len(projFlags), projFlags)
	}

	// 6. Verify flag is isolated and NOT in default project
	defaultFlags, err := memStore.ListFlagsByProject(ctx, store.DefaultProjectID)
	if err != nil {
		t.Fatalf("Failed to list default project flags: %v", err)
	}
	for _, f := range defaultFlags {
		if f.Key == flagKey {
			t.Fatalf("ISOLATION BREACH: Flag %s from project %s leaked into default project!", flagKey, customProj.ID)
		}
	}

	// 7. Verify GetFlagByProject returns the flag for custom project
	foundFlag, err := memStore.GetFlagByProject(ctx, customProj.ID, flagKey)
	if err != nil || foundFlag == nil {
		t.Fatalf("Expected GetFlagByProject to find %s in %s, got: %v", flagKey, customProj.ID, err)
	}
}
