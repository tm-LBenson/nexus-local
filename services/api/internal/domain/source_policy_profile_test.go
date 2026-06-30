package domain

import (
	"testing"
	"time"
)

func TestNewSourcePolicyProfileNormalizesValues(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	profile, err := NewSourcePolicyProfile(SourcePolicyProfileCreate{
		ID:                  SourcePolicyProfileID("policy_1"),
		TenantID:            TenantID("tenant_1"),
		OwnerID:             UserID("user_1"),
		Name:                "  Support exports  ",
		Detail:              "  Cases and logs  ",
		IncludePatterns:     []string{" **/*.json ", "", "**/*.csv"},
		ExcludePatterns:     []string{" **/tmp/** "},
		ScanIntervalMinutes: 1440,
		Now:                 now,
	})
	if err != nil {
		t.Fatalf("NewSourcePolicyProfile returned error: %v", err)
	}
	if profile.Name != "Support exports" || profile.Detail != "Cases and logs" {
		t.Fatalf("profile text not normalized: %#v", profile)
	}
	if len(profile.IncludePatterns) != 2 || profile.IncludePatterns[0] != "**/*.json" || profile.ExcludePatterns[0] != "**/tmp/**" {
		t.Fatalf("patterns not normalized: %#v %#v", profile.IncludePatterns, profile.ExcludePatterns)
	}
	if profile.ScanIntervalMinutes != 1440 || !profile.CreatedAt.Equal(now) || !profile.UpdatedAt.Equal(now) {
		t.Fatalf("timing/schedule not set: %#v", profile)
	}
}

func TestNewSourcePolicyProfileRejectsInvalidSchedule(t *testing.T) {
	_, err := NewSourcePolicyProfile(SourcePolicyProfileCreate{
		ID:                  SourcePolicyProfileID("policy_1"),
		TenantID:            TenantID("tenant_1"),
		OwnerID:             UserID("user_1"),
		Name:                "Bad schedule",
		ScanIntervalMinutes: -1,
	})
	if err == nil {
		t.Fatal("expected invalid schedule error")
	}
}
