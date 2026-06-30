package app

import (
	"context"
	"testing"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store/memory"
)

func TestSourceViewServiceCreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	service := NewSourceViewService(memory.New(), sourceViewIDs{}, sourceViewClock{})

	created, err := service.Create(ctx, CreateSourceViewInput{
		TenantID: domain.TenantID("tenant_1"),
		OwnerID:  domain.UserID("user_1"),
		Name:     "Blocked",
		Filters: SourceViewFiltersInput{
			Health: "blocked",
			Query:  "cases",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.View.ID != domain.SourceViewID("view_fixed") ||
		created.View.Filters.Health != "blocked" {
		t.Fatalf("created = %#v", created.View)
	}

	updated, err := service.Update(ctx, UpdateSourceViewInput{
		TenantID:     domain.TenantID("tenant_1"),
		SourceViewID: created.View.ID,
		Name:         "Needs Review",
		Filters: SourceViewFiltersInput{
			Health:   "review",
			Schedule: "overdue",
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.View.Name != "Needs Review" ||
		updated.View.Filters.Health != "review" ||
		updated.View.Filters.Schedule != "overdue" {
		t.Fatalf("updated = %#v", updated.View)
	}

	listed, err := service.List(ctx, ListSourceViewsInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed.Views) != 1 || listed.Views[0].Name != "Needs Review" {
		t.Fatalf("listed = %#v", listed.Views)
	}

	if err := service.Delete(ctx, DeleteSourceViewInput{
		TenantID:     domain.TenantID("tenant_1"),
		SourceViewID: created.View.ID,
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	listed, err = service.List(ctx, ListSourceViewsInput{TenantID: domain.TenantID("tenant_1")})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(listed.Views) != 0 {
		t.Fatalf("listed after delete = %#v", listed.Views)
	}
}

type sourceViewIDs struct{}

func (sourceViewIDs) NewSourceViewID() domain.SourceViewID {
	return domain.SourceViewID("view_fixed")
}

type sourceViewClock struct{}

func (sourceViewClock) Now() time.Time {
	return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
}
