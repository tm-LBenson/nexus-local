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
	ID                  DataSourceID
	TenantID            TenantID
	OwnerID             UserID
	Type                DataSourceType
	Name                string
	RootPath            string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
	NextScanAt          *time.Time
	Status              DataSourceStatus
	LastScanAt          *time.Time
	LastScanImported    int
	LastScanSkipped     int
	LastScanFailed      int
	CreatedAt           time.Time
	UpdatedAt           time.Time
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
	ID                  DataSourceID
	TenantID            TenantID
	OwnerID             UserID
	Type                DataSourceType
	Name                string
	RootPath            string
	IncludePatterns     []string
	ExcludePatterns     []string
	ScanIntervalMinutes int
	Now                 time.Time
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
	includePatterns, err := NormalizeDataSourcePatterns(input.IncludePatterns)
	if err != nil {
		return DataSource{}, fmt.Errorf("data source include patterns: %w", err)
	}
	excludePatterns, err := NormalizeDataSourcePatterns(input.ExcludePatterns)
	if err != nil {
		return DataSource{}, fmt.Errorf("data source exclude patterns: %w", err)
	}
	scanIntervalMinutes, err := NormalizeScanIntervalMinutes(input.ScanIntervalMinutes)
	if err != nil {
		return DataSource{}, fmt.Errorf("data source scan interval: %w", err)
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
	var nextScanAt *time.Time
	if scanIntervalMinutes > 0 {
		next := now
		nextScanAt = &next
	}

	return DataSource{
		ID:                  input.ID,
		TenantID:            input.TenantID,
		OwnerID:             input.OwnerID,
		Type:                sourceType,
		Name:                strings.TrimSpace(input.Name),
		RootPath:            strings.TrimSpace(input.RootPath),
		IncludePatterns:     includePatterns,
		ExcludePatterns:     excludePatterns,
		ScanIntervalMinutes: scanIntervalMinutes,
		NextScanAt:          nextScanAt,
		Status:              DataSourceStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
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

func (s *DataSource) Update(name string, sourceType DataSourceType, rootPath string, includePatterns []string, excludePatterns []string, scanIntervalMinutes int, now time.Time) error {
	if sourceType == "" {
		sourceType = s.Type
	}
	normalizedInclude, err := NormalizeDataSourcePatterns(includePatterns)
	if err != nil {
		return fmt.Errorf("data source update include patterns: %w", err)
	}
	normalizedExclude, err := NormalizeDataSourcePatterns(excludePatterns)
	if err != nil {
		return fmt.Errorf("data source update exclude patterns: %w", err)
	}
	normalizedInterval, err := NormalizeScanIntervalMinutes(scanIntervalMinutes)
	if err != nil {
		return fmt.Errorf("data source update scan interval: %w", err)
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
	s.IncludePatterns = normalizedInclude
	s.ExcludePatterns = normalizedExclude
	intervalChanged := s.ScanIntervalMinutes != normalizedInterval
	s.ScanIntervalMinutes = normalizedInterval
	if normalizedInterval == 0 {
		s.NextScanAt = nil
	} else if intervalChanged || s.NextScanAt == nil {
		s.NextScanAt = nextScanAtForInterval(normalizedInterval, s.LastScanAt, now)
	}
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
	if next == DataSourceStatusArchived {
		s.NextScanAt = nil
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
	s.ScheduleNextScan(now)
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
	s.ScheduleNextScan(now)
	s.UpdatedAt = now
	return nil
}

func (s *DataSource) ScheduleNextScan(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if s.ScanIntervalMinutes <= 0 || s.Status == DataSourceStatusArchived {
		s.NextScanAt = nil
		return
	}
	next := now.Add(time.Duration(s.ScanIntervalMinutes) * time.Minute)
	s.NextScanAt = &next
	s.UpdatedAt = now
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

const minScanIntervalMinutes = 5
const maxScanIntervalMinutes = 43200

func NormalizeScanIntervalMinutes(minutes int) (int, error) {
	if minutes < 0 {
		return 0, fmt.Errorf("scan interval is negative: %w", ErrInvalidEntity)
	}
	if minutes == 0 {
		return 0, nil
	}
	if minutes < minScanIntervalMinutes || minutes > maxScanIntervalMinutes {
		return 0, fmt.Errorf("scan interval must be 0 or between %d and %d minutes: %w", minScanIntervalMinutes, maxScanIntervalMinutes, ErrInvalidEntity)
	}
	return minutes, nil
}

func nextScanAtForInterval(minutes int, lastScanAt *time.Time, now time.Time) *time.Time {
	if minutes <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if lastScanAt == nil {
		next := now
		return &next
	}
	next := lastScanAt.Add(time.Duration(minutes) * time.Minute)
	if next.Before(now) {
		next = now
	}
	return &next
}

const maxDataSourcePatterns = 100
const maxDataSourcePatternLength = 240

func NormalizeDataSourcePatterns(patterns []string) ([]string, error) {
	normalized := make([]string, 0, len(patterns))
	seen := map[string]bool{}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
		pattern = strings.TrimPrefix(pattern, "/")
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		if len(pattern) > maxDataSourcePatternLength {
			return nil, fmt.Errorf("pattern %q is too long: %w", pattern, ErrInvalidEntity)
		}
		if strings.Contains(pattern, "\x00") || hasParentPathSegment(pattern) {
			return nil, fmt.Errorf("pattern %q is not allowed: %w", pattern, ErrInvalidEntity)
		}
		if seen[pattern] {
			continue
		}
		seen[pattern] = true
		normalized = append(normalized, pattern)
		if len(normalized) > maxDataSourcePatterns {
			return nil, fmt.Errorf("too many patterns: %w", ErrInvalidEntity)
		}
	}
	return normalized, nil
}

func hasParentPathSegment(pattern string) bool {
	for _, segment := range strings.Split(pattern, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
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
