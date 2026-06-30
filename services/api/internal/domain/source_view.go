package domain

import (
	"fmt"
	"strings"
	"time"
)

type SourceViewFilters struct {
	Health   string
	Query    string
	Schedule string
	Type     DataSourceType
}

type SourceView struct {
	ID        SourceViewID
	TenantID  TenantID
	OwnerID   UserID
	Name      string
	Filters   SourceViewFilters
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SourceViewCreate struct {
	ID       SourceViewID
	TenantID TenantID
	OwnerID  UserID
	Name     string
	Filters  SourceViewFilters
	Now      time.Time
}

const (
	maxSourceViewNameLength  = 80
	maxSourceViewQueryLength = 160
)

func NewSourceView(input SourceViewCreate) (SourceView, error) {
	filters, err := NormalizeSourceViewFilters(input.Filters)
	if err != nil {
		return SourceView{}, err
	}
	name := strings.TrimSpace(input.Name)
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.OwnerID)) ||
		name == "" ||
		len(name) > maxSourceViewNameLength {
		return SourceView{}, fmt.Errorf("source view: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return SourceView{
		ID:        input.ID,
		TenantID:  input.TenantID,
		OwnerID:   input.OwnerID,
		Name:      name,
		Filters:   filters,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (v *SourceView) Update(name string, filters SourceViewFilters, now time.Time) error {
	normalized, err := NormalizeSourceViewFilters(filters)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxSourceViewNameLength {
		return fmt.Errorf("source view update: %w", ErrInvalidEntity)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	v.Name = name
	v.Filters = normalized
	v.UpdatedAt = now
	return nil
}

func NormalizeSourceViewFilters(filters SourceViewFilters) (SourceViewFilters, error) {
	health := strings.ToLower(strings.TrimSpace(filters.Health))
	if !validSourceViewHealth(health) {
		return SourceViewFilters{}, fmt.Errorf("source view health filter: %w", ErrInvalidEntity)
	}
	schedule := strings.ToLower(strings.TrimSpace(filters.Schedule))
	if !validSourceViewSchedule(schedule) {
		return SourceViewFilters{}, fmt.Errorf("source view schedule filter: %w", ErrInvalidEntity)
	}
	query := strings.TrimSpace(filters.Query)
	if len(query) > maxSourceViewQueryLength {
		return SourceViewFilters{}, fmt.Errorf("source view query filter: %w", ErrInvalidEntity)
	}
	sourceType := DataSourceType(strings.TrimSpace(string(filters.Type)))
	if sourceType != "" && !sourceType.Valid() {
		return SourceViewFilters{}, fmt.Errorf("source view type filter: %w", ErrInvalidEntity)
	}
	return SourceViewFilters{
		Health:   health,
		Query:    query,
		Schedule: schedule,
		Type:     sourceType,
	}, nil
}

func validSourceViewHealth(value string) bool {
	switch value {
	case "", "active", "blocked", "review", "healthy", "archived":
		return true
	default:
		return false
	}
}

func validSourceViewSchedule(value string) bool {
	switch value {
	case "", "manual", "scheduled", "overdue":
		return true
	default:
		return false
	}
}
