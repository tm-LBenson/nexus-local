package domain

import (
	"fmt"
	"strings"
	"time"
)

type DataSourceType string

const (
	DataSourceTypeFolder       DataSourceType = "folder"
	DataSourceTypeSyncedFolder DataSourceType = "synced_folder"
	DataSourceTypeNetworkShare DataSourceType = "network_share"
	DataSourceTypeExport       DataSourceType = "export"
	DataSourceTypeConnector    DataSourceType = "connector"
)

type DataSourceStatus string

const (
	DataSourceStatusActive   DataSourceStatus = "active"
	DataSourceStatusScanning DataSourceStatus = "scanning"
	DataSourceStatusFailed   DataSourceStatus = "failed"
	DataSourceStatusArchived DataSourceStatus = "archived"
)

type DataSourceScanOutcome string

const (
	DataSourceScanOutcomeImported DataSourceScanOutcome = "imported"
	DataSourceScanOutcomeSkipped  DataSourceScanOutcome = "skipped"
	DataSourceScanOutcomeFailed   DataSourceScanOutcome = "failed"
	DataSourceScanOutcomeDeleted  DataSourceScanOutcome = "deleted"
)

type DataSource struct {
	ID               DataSourceID
	TenantID         TenantID
	OwnerID          UserID
	Type             DataSourceType
	Name             string
	RootPath         string
	Status           DataSourceStatus
	LastScanAt       *time.Time
	LastScanImported int
	LastScanSkipped  int
	LastScanFailed   int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type DataSourceScanEntry struct {
	TenantID    TenantID
	JobID       JobID
	SourceID    DataSourceID
	Path        string
	Outcome     DataSourceScanOutcome
	Reason      string
	Message     string
	DocumentID  DocumentID
	SizeBytes   int64
	ContentHash string
	CreatedAt   time.Time
}

type DataSourceCreate struct {
	ID       DataSourceID
	TenantID TenantID
	OwnerID  UserID
	Type     DataSourceType
	Name     string
	RootPath string
	Now      time.Time
}

type DataSourceScanEntryCreate struct {
	TenantID    TenantID
	JobID       JobID
	SourceID    DataSourceID
	Path        string
	Outcome     DataSourceScanOutcome
	Reason      string
	Message     string
	DocumentID  DocumentID
	SizeBytes   int64
	ContentHash string
	Now         time.Time
}

func NewDataSource(input DataSourceCreate) (DataSource, error) {
	sourceType := input.Type
	if sourceType == "" {
		sourceType = DataSourceTypeFolder
	}
	if emptyID(string(input.ID)) ||
		emptyID(string(input.TenantID)) ||
		emptyID(string(input.OwnerID)) ||
		!sourceType.Valid() ||
		strings.TrimSpace(input.Name) == "" ||
		strings.TrimSpace(input.RootPath) == "" {
		return DataSource{}, fmt.Errorf("data source: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return DataSource{
		ID:        input.ID,
		TenantID:  input.TenantID,
		OwnerID:   input.OwnerID,
		Type:      sourceType,
		Name:      strings.TrimSpace(input.Name),
		RootPath:  strings.TrimSpace(input.RootPath),
		Status:    DataSourceStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func NewDataSourceScanEntry(input DataSourceScanEntryCreate) (DataSourceScanEntry, error) {
	if emptyID(string(input.TenantID)) ||
		emptyID(string(input.JobID)) ||
		emptyID(string(input.SourceID)) ||
		strings.TrimSpace(input.Path) == "" ||
		!input.Outcome.Valid() ||
		input.SizeBytes < 0 {
		return DataSourceScanEntry{}, fmt.Errorf("data source scan entry: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return DataSourceScanEntry{
		TenantID:    input.TenantID,
		JobID:       input.JobID,
		SourceID:    input.SourceID,
		Path:        strings.TrimSpace(input.Path),
		Outcome:     input.Outcome,
		Reason:      strings.TrimSpace(input.Reason),
		Message:     strings.TrimSpace(input.Message),
		DocumentID:  input.DocumentID,
		SizeBytes:   input.SizeBytes,
		ContentHash: strings.TrimSpace(input.ContentHash),
		CreatedAt:   now,
	}, nil
}

func (s *DataSource) Update(name string, sourceType DataSourceType, rootPath string, now time.Time) error {
	if sourceType == "" {
		sourceType = s.Type
	}
	if !sourceType.Valid() ||
		strings.TrimSpace(name) == "" ||
		strings.TrimSpace(rootPath) == "" ||
		s.Status == DataSourceStatusArchived {
		return fmt.Errorf("data source update: %w", ErrInvalidEntity)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.Name = strings.TrimSpace(name)
	s.Type = sourceType
	s.RootPath = strings.TrimSpace(rootPath)
	s.UpdatedAt = now
	return nil
}

func (s *DataSource) Transition(next DataSourceStatus, now time.Time) error {
	if !s.Status.CanTransition(next) {
		return fmt.Errorf("%s -> %s: %w", s.Status, next, ErrInvalidStateTransition)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.Status = next
	if next == DataSourceStatusActive {
		scannedAt := now
		s.LastScanAt = &scannedAt
	}
	s.UpdatedAt = now
	return nil
}

func (s *DataSource) CompleteScan(imported int, skipped int, failed int, now time.Time) error {
	if err := validateScanCounts(imported, skipped, failed); err != nil {
		return err
	}
	if err := s.Transition(DataSourceStatusActive, now); err != nil {
		return err
	}
	s.LastScanImported = imported
	s.LastScanSkipped = skipped
	s.LastScanFailed = failed
	return nil
}

func (s *DataSource) FailScan(imported int, skipped int, failed int, now time.Time) error {
	if err := validateScanCounts(imported, skipped, failed); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if s.Status != DataSourceStatusFailed {
		if err := s.Transition(DataSourceStatusFailed, now); err != nil {
			return err
		}
	}
	scannedAt := now
	s.LastScanAt = &scannedAt
	s.LastScanImported = imported
	s.LastScanSkipped = skipped
	s.LastScanFailed = failed
	s.UpdatedAt = now
	return nil
}

func (t DataSourceType) Valid() bool {
	switch t {
	case DataSourceTypeFolder,
		DataSourceTypeSyncedFolder,
		DataSourceTypeNetworkShare,
		DataSourceTypeExport,
		DataSourceTypeConnector:
		return true
	default:
		return false
	}
}

func (o DataSourceScanOutcome) Valid() bool {
	switch o {
	case DataSourceScanOutcomeImported,
		DataSourceScanOutcomeSkipped,
		DataSourceScanOutcomeFailed,
		DataSourceScanOutcomeDeleted:
		return true
	default:
		return false
	}
}

func validateScanCounts(imported int, skipped int, failed int) error {
	if imported < 0 || skipped < 0 || failed < 0 {
		return fmt.Errorf("data source scan counts: %w", ErrInvalidEntity)
	}
	return nil
}

func (s DataSourceStatus) CanTransition(next DataSourceStatus) bool {
	switch s {
	case DataSourceStatusActive:
		return next == DataSourceStatusScanning ||
			next == DataSourceStatusFailed ||
			next == DataSourceStatusArchived
	case DataSourceStatusScanning:
		return next == DataSourceStatusActive ||
			next == DataSourceStatusFailed ||
			next == DataSourceStatusArchived
	case DataSourceStatusFailed:
		return next == DataSourceStatusScanning ||
			next == DataSourceStatusActive ||
			next == DataSourceStatusArchived
	default:
		return false
	}
}
