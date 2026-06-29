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

type DataSource struct {
	ID         DataSourceID
	TenantID   TenantID
	OwnerID    UserID
	Type       DataSourceType
	Name       string
	RootPath   string
	Status     DataSourceStatus
	LastScanAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
