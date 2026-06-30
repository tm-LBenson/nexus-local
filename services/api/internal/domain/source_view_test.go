package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewSourceViewNormalizesFilters(t *testing.T) {
	view, err := NewSourceView(SourceViewCreate{
		ID:       SourceViewID("view_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Name:     "  Needs Review  ",
		Filters: SourceViewFilters{
			Health:   " Review ",
			Query:    "  one drive  ",
			Schedule: " Overdue ",
			Type:     DataSourceTypeSyncedFolder,
		},
		Now: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("new source view: %v", err)
	}
	if view.Name != "Needs Review" ||
		view.Filters.Health != "review" ||
		view.Filters.Query != "one drive" ||
		view.Filters.Schedule != "overdue" ||
		view.Filters.Type != DataSourceTypeSyncedFolder {
		t.Fatalf("view = %#v", view)
	}
}

func TestNewSourceViewRejectsInvalidFilters(t *testing.T) {
	_, err := NewSourceView(SourceViewCreate{
		ID:       SourceViewID("view_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Name:     "Bad View",
		Filters:  SourceViewFilters{Health: "wat"},
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want invalid entity", err)
	}
}
