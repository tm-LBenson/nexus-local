package domain

import "strings"

type TenantID string
type UserID string
type ConversationID string
type MessageID string
type DocumentID string
type DataSourceID string
type SourceViewID string
type SourcePolicyProfileID string
type JobID string
type AuditEventID string

func emptyID(value string) bool {
	return strings.TrimSpace(value) == ""
}
