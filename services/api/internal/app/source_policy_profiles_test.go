package app

import (
	"context"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestSourcePolicyProfileServiceCreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	service := NewSourcePolicyProfileService(memory.New(), sourcePolicyProfileIDs{}, sourcePolicyProfileClock{})

	created, err := service.Create(ctx, CreateSourcePolicyProfileInput{
		TenantID:            domain.TenantID("tenant_1"),
		OwnerID:             domain.UserID("user_1"),
		Name:                "Support",
		Detail:              "Cases and logs",
		IncludePatterns:     []string{"**/*.json"},
		ExcludePatterns:     []string{"**/tmp/**"},
		ScanIntervalMinutes: 1440,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Profile.ID != domain.SourcePolicyProfileID("policy_fixed") ||
		created.Profile.IncludePatterns[0] != "**/*.json" {
		t.Fatalf("unexpected profile: %#v", created.Profile)
	}

	updated, err := service.Update(ctx, UpdateSourcePolicyProfileInput{
		TenantID:              domain.TenantID("tenant_1"),
		SourcePolicyProfileID: created.Profile.ID,
		Name:                  "Support logs",
		Detail:                "Refined",
		IncludePatterns:       []string{"**/*.csv"},
		ExcludePatterns:       []string{},
		ScanIntervalMinutes:   0,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Profile.Name != "Support logs" || updated.Profile.IncludePatterns[0] != "**/*.csv" {
		t.Fatalf("unexpected updated profile: %#v", updated.Profile)
	}

	listed, err := service.List(ctx, ListSourcePolicyProfilesInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed.Profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(listed.Profiles))
	}

	if err := service.Delete(ctx, DeleteSourcePolicyProfileInput{
		TenantID:              domain.TenantID("tenant_1"),
		SourcePolicyProfileID: created.Profile.ID,
	}); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	listed, err = service.List(ctx, ListSourcePolicyProfilesInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("List after delete returned error: %v", err)
	}
	if len(listed.Profiles) != 0 {
		t.Fatalf("expected zero profiles, got %d", len(listed.Profiles))
	}
}

type sourcePolicyProfileIDs struct{}

func (sourcePolicyProfileIDs) NewSourcePolicyProfileID() domain.SourcePolicyProfileID {
	return domain.SourcePolicyProfileID("policy_fixed")
}

type sourcePolicyProfileClock struct{}

func (sourcePolicyProfileClock) Now() time.Time {
	return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
}
