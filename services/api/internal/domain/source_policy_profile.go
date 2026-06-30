package domain

import (
	"fmt"
	"strings"
	"time"
)

type SourcePolicyProfile struct {
	ID                  SourcePolicyProfileID
	TenantID            TenantID
	OwnerID             UserID
	Name                string
	Detail              string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SourcePolicyProfileCreate struct {
	ID                  SourcePolicyProfileID
	TenantID            TenantID
	OwnerID             UserID
	Name                string
	Detail              string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
	Now                 time.Time
}

const (
	maxSourcePolicyProfileNameLength   = 80
	maxSourcePolicyProfileDetailLength = 180
)

func NewSourcePolicyProfile(input SourcePolicyProfileCreate) (SourcePolicyProfile, error) {
	includePatterns, err := NormalizeDataSourcePatterns(input.IncludePatterns)
	if err != nil {
		return SourcePolicyProfile{}, fmt.Errorf("source policy profile include patterns: %w", err)
	}
	excludePatterns, err := NormalizeDataSourcePatterns(input.ExcludePatterns)
	if err != nil {
		return SourcePolicyProfile{}, fmt.Errorf("source policy profile exclude patterns: %w", err)
	}
	scanIntervalMinutes, err := NormalizeScanIntervalMinutes(input.ScanIntervalMinutes)
	if err != nil {
		return SourcePolicyProfile{}, fmt.Errorf("source policy profile scan interval: %w", err)
	}
	name := strings.TrimSpace(input.Name)
	detail := strings.TrimSpace(input.Detail)
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.OwnerID)) ||
		name == "" ||
		len(name) > maxSourcePolicyProfileNameLength ||
		len(detail) > maxSourcePolicyProfileDetailLength {
		return SourcePolicyProfile{}, fmt.Errorf("source policy profile: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return SourcePolicyProfile{
		ID:                  input.ID,
		TenantID:            input.TenantID,
		OwnerID:             input.OwnerID,
		Name:                name,
		Detail:              detail,
		IncludePatterns:     includePatterns,
		ExcludePatterns:     excludePatterns,
		ScanIntervalMinutes: scanIntervalMinutes,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

func (p *SourcePolicyProfile) Update(name string, detail string, includePatterns []string, excludePatterns []string, scanIntervalMinutes int, now time.Time) error {
	normalizedInclude, err := NormalizeDataSourcePatterns(includePatterns)
	if err != nil {
		return fmt.Errorf("source policy profile update include patterns: %w", err)
	}
	normalizedExclude, err := NormalizeDataSourcePatterns(excludePatterns)
	if err != nil {
		return fmt.Errorf("source policy profile update exclude patterns: %w", err)
	}
	normalizedInterval, err := NormalizeScanIntervalMinutes(scanIntervalMinutes)
	if err != nil {
		return fmt.Errorf("source policy profile update scan interval: %w", err)
	}
	name = strings.TrimSpace(name)
	detail = strings.TrimSpace(detail)
	if name == "" ||
		len(name) > maxSourcePolicyProfileNameLength ||
		len(detail) > maxSourcePolicyProfileDetailLength {
		return fmt.Errorf("source policy profile update: %w", ErrInvalidEntity)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.Name = name
	p.Detail = detail
	p.IncludePatterns = normalizedInclude
	p.ExcludePatterns = normalizedExclude
	p.ScanIntervalMinutes = normalizedInterval
	p.UpdatedAt = now
	return nil
}
