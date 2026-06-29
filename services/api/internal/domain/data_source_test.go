package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewDataSourceDefaultsToActiveFolder(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)

	source, err := NewDataSource(DataSourceCreate{
		ID:       DataSourceID("src_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Name:     "Support Docs",
		RootPath: "C:\\Docs",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("new data source: %v", err)
	}
	if source.Type != DataSourceTypeFolder {
		t.Fatalf("type = %q, want folder", source.Type)
	}
	if source.Status != DataSourceStatusActive {
		t.Fatalf("status = %q, want active", source.Status)
	}
	if source.CreatedAt != now || source.UpdatedAt != now {
		t.Fatalf("timestamps = %s/%s, want %s", source.CreatedAt, source.UpdatedAt, now)
	}
	if len(source.IncludePatterns) != 0 || len(source.ExcludePatterns) != 0 {
		t.Fatalf("patterns = %#v/%#v, want empty defaults", source.IncludePatterns, source.ExcludePatterns)
	}
}

func TestNewDataSourceNormalizesPatterns(t *testing.T) {
	source, err := NewDataSource(DataSourceCreate{
		ID:              DataSourceID("src_1"),
		TenantID:        TenantID("tenant_1"),
		OwnerID:         UserID("user_1"),
		Name:            "Support Docs",
		RootPath:        "C:\\Docs",
		IncludePatterns: []string{" **/*.md ", "", "# comment", "\\cases\\**", "**/*.md"},
		ExcludePatterns: []string{"/archive/**", "tmp/**"},
	})
	if err != nil {
		t.Fatalf("new data source: %v", err)
	}
	if len(source.IncludePatterns) != 2 ||
		source.IncludePatterns[0] != "**/*.md" ||
		source.IncludePatterns[1] != "cases/**" {
		t.Fatalf("include patterns = %#v", source.IncludePatterns)
	}
	if len(source.ExcludePatterns) != 2 ||
		source.ExcludePatterns[0] != "archive/**" ||
		source.ExcludePatterns[1] != "tmp/**" {
		t.Fatalf("exclude patterns = %#v", source.ExcludePatterns)
	}
}

func TestNewDataSourceCanScheduleScans(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	source, err := NewDataSource(DataSourceCreate{
		ID:                  DataSourceID("src_1"),
		TenantID:            TenantID("tenant_1"),
		OwnerID:             UserID("user_1"),
		Name:                "Support Docs",
		RootPath:            "C:\\Docs",
		ScanIntervalMinutes: 60,
		Now:                 now,
	})
	if err != nil {
		t.Fatalf("new data source: %v", err)
	}
	if source.ScanIntervalMinutes != 60 || source.NextScanAt == nil || !source.NextScanAt.Equal(now) {
		t.Fatalf("schedule = %d/%v, want 60/%s", source.ScanIntervalMinutes, source.NextScanAt, now)
	}

	if err := source.Transition(DataSourceStatusScanning, now); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if err := source.CompleteScan(1, 0, 0, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("complete scan: %v", err)
	}
	wantNext := now.Add(70 * time.Minute)
	if source.NextScanAt == nil || !source.NextScanAt.Equal(wantNext) {
		t.Fatalf("next scan = %v, want %s", source.NextScanAt, wantNext)
	}
}

func TestNewDataSourceRejectsInvalidSchedule(t *testing.T) {
	_, err := NewDataSource(DataSourceCreate{
		ID:                  DataSourceID("src_1"),
		TenantID:            TenantID("tenant_1"),
		OwnerID:             UserID("user_1"),
		Name:                "Support Docs",
		RootPath:            "C:\\Docs",
		ScanIntervalMinutes: 1,
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want invalid entity", err)
	}
}

func TestNewDataSourceRejectsUnsafePattern(t *testing.T) {
	_, err := NewDataSource(DataSourceCreate{
		ID:              DataSourceID("src_1"),
		TenantID:        TenantID("tenant_1"),
		OwnerID:         UserID("user_1"),
		Name:            "Support Docs",
		RootPath:        "C:\\Docs",
		IncludePatterns: []string{"../secrets/**"},
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want invalid entity", err)
	}
}

func TestNewDataSourceRejectsInvalidInput(t *testing.T) {
	_, err := NewDataSource(DataSourceCreate{
		ID:       DataSourceID("src_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Type:     DataSourceType("bad"),
		Name:     "Docs",
		RootPath: "C:\\Docs",
	})
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want invalid entity", err)
	}
}

func TestDataSourceUpdateAndArchive(t *testing.T) {
	source, err := NewDataSource(DataSourceCreate{
		ID:       DataSourceID("src_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Name:     "Docs",
		RootPath: "C:\\Docs",
	})
	if err != nil {
		t.Fatalf("new data source: %v", err)
	}
	updatedAt := time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC)
	if err := source.Update("Runbooks", DataSourceTypeNetworkShare, "\\\\nas\\runbooks", []string{"**/*.md"}, []string{"archive/**"}, 1440, updatedAt); err != nil {
		t.Fatalf("update: %v", err)
	}
	if source.Name != "Runbooks" || source.Type != DataSourceTypeNetworkShare || source.RootPath != "\\\\nas\\runbooks" {
		t.Fatalf("source after update = %#v", source)
	}
	if len(source.IncludePatterns) != 1 || source.IncludePatterns[0] != "**/*.md" ||
		len(source.ExcludePatterns) != 1 || source.ExcludePatterns[0] != "archive/**" {
		t.Fatalf("patterns after update = %#v/%#v", source.IncludePatterns, source.ExcludePatterns)
	}
	if source.ScanIntervalMinutes != 1440 || source.NextScanAt == nil {
		t.Fatalf("schedule after update = %d/%v", source.ScanIntervalMinutes, source.NextScanAt)
	}
	if err := source.Transition(DataSourceStatusArchived, updatedAt.Add(time.Hour)); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if source.NextScanAt != nil {
		t.Fatalf("next scan after archive = %v, want nil", source.NextScanAt)
	}
	if err := source.Update("Again", DataSourceTypeFolder, "C:\\Again", nil, nil, 0, updatedAt.Add(2*time.Hour)); !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want invalid entity", err)
	}
}

func TestDataSourceScanResults(t *testing.T) {
	source, err := NewDataSource(DataSourceCreate{
		ID:       DataSourceID("src_1"),
		TenantID: TenantID("tenant_1"),
		OwnerID:  UserID("user_1"),
		Name:     "Docs",
		RootPath: "C:\\Docs",
	})
	if err != nil {
		t.Fatalf("new data source: %v", err)
	}
	now := time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC)
	if err := source.Transition(DataSourceStatusScanning, now); err != nil {
		t.Fatalf("start scan: %v", err)
	}
	if err := source.CompleteScan(3, 2, 0, now.Add(time.Minute)); err != nil {
		t.Fatalf("complete scan: %v", err)
	}
	if source.Status != DataSourceStatusActive {
		t.Fatalf("status = %q, want active", source.Status)
	}
	if source.LastScanAt == nil {
		t.Fatal("last scan at is nil")
	}
	if source.LastScanImported != 3 || source.LastScanSkipped != 2 || source.LastScanFailed != 0 {
		t.Fatalf("scan counts = %d/%d/%d, want 3/2/0", source.LastScanImported, source.LastScanSkipped, source.LastScanFailed)
	}

	if err := source.Transition(DataSourceStatusScanning, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("restart scan: %v", err)
	}
	if err := source.FailScan(1, 4, 2, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("fail scan: %v", err)
	}
	if source.Status != DataSourceStatusFailed {
		t.Fatalf("status = %q, want failed", source.Status)
	}
	if source.LastScanImported != 1 || source.LastScanSkipped != 4 || source.LastScanFailed != 2 {
		t.Fatalf("failed scan counts = %d/%d/%d, want 1/4/2", source.LastScanImported, source.LastScanSkipped, source.LastScanFailed)
	}

	if err := source.FailScan(-1, 0, 0, now); !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("err = %v, want invalid entity", err)
	}
}

func TestDataSourceCancelScanReturnsToActiveWithoutCompleting(t *testing.T) {
	source, err := NewDataSource(DataSourceCreate{
		ID:                  DataSourceID("src_1"),
		TenantID:            TenantID("tenant_1"),
		OwnerID:             UserID("user_1"),
		Name:                "Docs",
		RootPath:            "C:\\Docs",
		ScanIntervalMinutes: 60,
	})
	if err != nil {
		t.Fatalf("new data source: %v", err)
	}
	now := time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC)
	if err := source.Transition(DataSourceStatusScanning, now); err != nil {
		t.Fatalf("start scan: %v", err)
	}
	if err := source.CancelScan(now.Add(time.Minute)); err != nil {
		t.Fatalf("cancel scan: %v", err)
	}
	if source.Status != DataSourceStatusActive {
		t.Fatalf("status = %q, want active", source.Status)
	}
	if source.LastScanAt != nil ||
		source.LastScanImported != 0 ||
		source.LastScanSkipped != 0 ||
		source.LastScanFailed != 0 {
		t.Fatalf("last scan = %v/%d/%d/%d, want untouched", source.LastScanAt, source.LastScanImported, source.LastScanSkipped, source.LastScanFailed)
	}
	wantNext := now.Add(61 * time.Minute)
	if source.NextScanAt == nil || !source.NextScanAt.Equal(wantNext) {
		t.Fatalf("next scan = %v, want %s", source.NextScanAt, wantNext)
	}
}
