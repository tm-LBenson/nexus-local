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
	if err := source.Update("Runbooks", DataSourceTypeNetworkShare, "\\\\nas\\runbooks", updatedAt); err != nil {
		t.Fatalf("update: %v", err)
	}
	if source.Name != "Runbooks" || source.Type != DataSourceTypeNetworkShare || source.RootPath != "\\\\nas\\runbooks" {
		t.Fatalf("source after update = %#v", source)
	}
	if err := source.Transition(DataSourceStatusArchived, updatedAt.Add(time.Hour)); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := source.Update("Again", DataSourceTypeFolder, "C:\\Again", updatedAt.Add(2*time.Hour)); !errors.Is(err, ErrInvalidEntity) {
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
