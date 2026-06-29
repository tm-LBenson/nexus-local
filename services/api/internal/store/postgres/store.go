package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return New(pool), nil
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) SaveTenant(ctx context.Context, tenant domain.Tenant) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenants (id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    updated_at = EXCLUDED.updated_at
	`, tenant.ID, tenant.Name, tenant.CreatedAt, tenant.UpdatedAt)
	return err
}

func (s *Store) GetTenant(ctx context.Context, id domain.TenantID) (domain.Tenant, error) {
	var tenant domain.Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, created_at, updated_at
		FROM tenants
		WHERE id = $1
	`, id).Scan(&tenant.ID, &tenant.Name, &tenant.CreatedAt, &tenant.UpdatedAt)
	if err != nil {
		return domain.Tenant{}, translateErr(err)
	}
	return tenant, nil
}

func (s *Store) SaveUser(ctx context.Context, user domain.User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, email, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE
		SET email = EXCLUDED.email,
		    name = EXCLUDED.name,
		    updated_at = EXCLUDED.updated_at
	`, user.ID, user.Email, user.Name, user.CreatedAt, user.UpdatedAt)
	return err
}

func (s *Store) GetUser(ctx context.Context, id domain.UserID) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return domain.User{}, translateErr(err)
	}
	return user, nil
}

func (s *Store) SaveMembership(ctx context.Context, membership domain.Membership) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO memberships (tenant_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO UPDATE
		SET role = EXCLUDED.role
	`, membership.TenantID, membership.UserID, membership.Role)
	return err
}

func (s *Store) ListMembershipsForUser(ctx context.Context, userID domain.UserID) ([]domain.Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, user_id, role
		FROM memberships
		WHERE user_id = $1
		ORDER BY tenant_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]domain.Membership, 0)
	for rows.Next() {
		var membership domain.Membership
		if err := rows.Scan(&membership.TenantID, &membership.UserID, &membership.Role); err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	return memberships, rows.Err()
}

func (s *Store) ListMembershipsForTenant(ctx context.Context, tenantID domain.TenantID) ([]domain.Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, user_id, role
		FROM memberships
		WHERE tenant_id = $1
		ORDER BY user_id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]domain.Membership, 0)
	for rows.Next() {
		var membership domain.Membership
		if err := rows.Scan(&membership.TenantID, &membership.UserID, &membership.Role); err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	return memberships, rows.Err()
}

func (s *Store) DeleteMembership(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM memberships
		WHERE tenant_id = $1 AND user_id = $2
	`, tenantID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) SaveDocument(ctx context.Context, document domain.Document) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO documents (
			tenant_id, id, owner_id, name, storage_key, size_bytes, status, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    name = EXCLUDED.name,
		    storage_key = EXCLUDED.storage_key,
		    size_bytes = EXCLUDED.size_bytes,
		    status = EXCLUDED.status,
		    updated_at = EXCLUDED.updated_at
	`, document.TenantID, document.ID, document.OwnerID, document.Name, document.StorageKey, document.SizeBytes, document.Status, document.CreatedAt, document.UpdatedAt)
	return err
}

func (s *Store) GetDocument(ctx context.Context, tenantID domain.TenantID, id domain.DocumentID) (domain.Document, error) {
	var document domain.Document
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, owner_id, name, storage_key, size_bytes, status, created_at, updated_at
		FROM documents
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&document.ID,
		&document.TenantID,
		&document.OwnerID,
		&document.Name,
		&document.StorageKey,
		&document.SizeBytes,
		&document.Status,
		&document.CreatedAt,
		&document.UpdatedAt,
	)
	if err != nil {
		return domain.Document{}, translateErr(err)
	}
	return document, nil
}

func (s *Store) ListDocuments(ctx context.Context, tenantID domain.TenantID) ([]domain.Document, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, owner_id, name, storage_key, size_bytes, status, created_at, updated_at
		FROM documents
		WHERE tenant_id = $1
		ORDER BY created_at, id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	documents := make([]domain.Document, 0)
	for rows.Next() {
		var document domain.Document
		if err := rows.Scan(
			&document.ID,
			&document.TenantID,
			&document.OwnerID,
			&document.Name,
			&document.StorageKey,
			&document.SizeBytes,
			&document.Status,
			&document.CreatedAt,
			&document.UpdatedAt,
		); err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

func (s *Store) SaveDataSource(ctx context.Context, source domain.DataSource) error {
	var lastScanAt any
	if source.LastScanAt != nil {
		lastScanAt = *source.LastScanAt
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO data_sources (
			tenant_id, id, owner_id, type, name, root_path, status, last_scan_at,
			last_scan_imported, last_scan_skipped, last_scan_failed,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    type = EXCLUDED.type,
		    name = EXCLUDED.name,
		    root_path = EXCLUDED.root_path,
		    status = EXCLUDED.status,
		    last_scan_at = EXCLUDED.last_scan_at,
		    last_scan_imported = EXCLUDED.last_scan_imported,
		    last_scan_skipped = EXCLUDED.last_scan_skipped,
		    last_scan_failed = EXCLUDED.last_scan_failed,
		    updated_at = EXCLUDED.updated_at
	`, source.TenantID, source.ID, source.OwnerID, source.Type, source.Name, source.RootPath, source.Status, lastScanAt, source.LastScanImported, source.LastScanSkipped, source.LastScanFailed, source.CreatedAt, source.UpdatedAt)
	return err
}

func (s *Store) GetDataSource(ctx context.Context, tenantID domain.TenantID, id domain.DataSourceID) (domain.DataSource, error) {
	var source domain.DataSource
	var lastScanAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, owner_id, type, name, root_path, status, last_scan_at,
		       last_scan_imported, last_scan_skipped, last_scan_failed,
		       created_at, updated_at
		FROM data_sources
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&source.ID,
		&source.TenantID,
		&source.OwnerID,
		&source.Type,
		&source.Name,
		&source.RootPath,
		&source.Status,
		&lastScanAt,
		&source.LastScanImported,
		&source.LastScanSkipped,
		&source.LastScanFailed,
		&source.CreatedAt,
		&source.UpdatedAt,
	)
	if err != nil {
		return domain.DataSource{}, translateErr(err)
	}
	if lastScanAt.Valid {
		source.LastScanAt = &lastScanAt.Time
	}
	return source, nil
}

func (s *Store) ListDataSources(ctx context.Context, tenantID domain.TenantID) ([]domain.DataSource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, owner_id, type, name, root_path, status, last_scan_at,
		       last_scan_imported, last_scan_skipped, last_scan_failed,
		       created_at, updated_at
		FROM data_sources
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make([]domain.DataSource, 0)
	for rows.Next() {
		var source domain.DataSource
		var lastScanAt sql.NullTime
		if err := rows.Scan(
			&source.ID,
			&source.TenantID,
			&source.OwnerID,
			&source.Type,
			&source.Name,
			&source.RootPath,
			&source.Status,
			&lastScanAt,
			&source.LastScanImported,
			&source.LastScanSkipped,
			&source.LastScanFailed,
			&source.CreatedAt,
			&source.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lastScanAt.Valid {
			source.LastScanAt = &lastScanAt.Time
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Store) SaveDataSourceScanEntry(ctx context.Context, entry domain.DataSourceScanEntry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO data_source_scan_entries (
			tenant_id, job_id, source_id, path, outcome, reason, message, document_id, size_bytes, content_hash, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (tenant_id, job_id, path) DO UPDATE
		SET source_id = EXCLUDED.source_id,
		    outcome = EXCLUDED.outcome,
		    reason = EXCLUDED.reason,
		    message = EXCLUDED.message,
		    document_id = EXCLUDED.document_id,
		    size_bytes = EXCLUDED.size_bytes,
		    content_hash = EXCLUDED.content_hash,
		    created_at = EXCLUDED.created_at
	`, entry.TenantID, entry.JobID, entry.SourceID, entry.Path, entry.Outcome, entry.Reason, entry.Message, entry.DocumentID, entry.SizeBytes, entry.ContentHash, entry.CreatedAt)
	return err
}

func (s *Store) ListDataSourceScanEntries(ctx context.Context, tenantID domain.TenantID, sourceID domain.DataSourceID, limit int) ([]domain.DataSourceScanEntry, error) {
	query := `
		SELECT tenant_id, job_id, source_id, path, outcome, reason, message, document_id, size_bytes, content_hash, created_at
		FROM data_source_scan_entries
		WHERE tenant_id = $1 AND source_id = $2
		ORDER BY created_at DESC, job_id DESC, path
	`
	args := []any{tenantID, sourceID}
	if limit > 0 {
		query += " LIMIT $3"
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]domain.DataSourceScanEntry, 0)
	for rows.Next() {
		var entry domain.DataSourceScanEntry
		if err := rows.Scan(
			&entry.TenantID,
			&entry.JobID,
			&entry.SourceID,
			&entry.Path,
			&entry.Outcome,
			&entry.Reason,
			&entry.Message,
			&entry.DocumentID,
			&entry.SizeBytes,
			&entry.ContentHash,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) SaveJob(ctx context.Context, job domain.Job) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (
			tenant_id, id, type, resource_type, resource_id, state, attempts, error_message, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET type = EXCLUDED.type,
		    resource_type = EXCLUDED.resource_type,
		    resource_id = EXCLUDED.resource_id,
		    state = EXCLUDED.state,
		    attempts = EXCLUDED.attempts,
		    error_message = EXCLUDED.error_message,
		    updated_at = EXCLUDED.updated_at
	`, job.TenantID, job.ID, job.Type, job.ResourceType, job.ResourceID, job.State, job.Attempts, job.ErrorMessage, job.CreatedAt, job.UpdatedAt)
	return err
}

func (s *Store) GetJob(ctx context.Context, tenantID domain.TenantID, id domain.JobID) (domain.Job, error) {
	var job domain.Job
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, type, resource_type, resource_id, state, attempts, error_message, created_at, updated_at
		FROM jobs
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&job.ID,
		&job.TenantID,
		&job.Type,
		&job.ResourceType,
		&job.ResourceID,
		&job.State,
		&job.Attempts,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return domain.Job{}, translateErr(err)
	}
	return job, nil
}

func (s *Store) ListJobs(ctx context.Context, tenantID domain.TenantID, limit int) ([]domain.Job, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, type, resource_type, resource_id, state, attempts, error_message, created_at, updated_at
		FROM jobs
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, id
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0)
	for rows.Next() {
		var job domain.Job
		if err := rows.Scan(
			&job.ID,
			&job.TenantID,
			&job.Type,
			&job.ResourceType,
			&job.ResourceID,
			&job.State,
			&job.Attempts,
			&job.ErrorMessage,
			&job.CreatedAt,
			&job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) ClaimNextQueuedJob(ctx context.Context, now time.Time, types ...domain.JobType) (domain.Job, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)

	args := []any{domain.JobStateQueued}
	where := "state = $1"
	if len(types) > 0 {
		placeholders := make([]string, 0, len(types))
		for _, jobType := range types {
			args = append(args, jobType)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		where += fmt.Sprintf(" AND type IN (%s)", strings.Join(placeholders, ", "))
	}

	args = append(args, domain.JobStateRunning, now)
	runningArg := len(args) - 1
	nowArg := len(args)

	var job domain.Job
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		WITH picked AS (
			SELECT tenant_id, id
			FROM jobs
			WHERE %s
			ORDER BY created_at, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs
		SET state = $%d,
		    attempts = jobs.attempts + 1,
		    error_message = '',
		    updated_at = $%d
		FROM picked
		WHERE jobs.tenant_id = picked.tenant_id
		  AND jobs.id = picked.id
		RETURNING jobs.id, jobs.tenant_id, jobs.type, jobs.resource_type, jobs.resource_id, jobs.state, jobs.attempts, jobs.error_message, jobs.created_at, jobs.updated_at
	`, where, runningArg, nowArg), args...).Scan(
		&job.ID,
		&job.TenantID,
		&job.Type,
		&job.ResourceType,
		&job.ResourceID,
		&job.State,
		&job.Attempts,
		&job.ErrorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return domain.Job{}, translateErr(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func (s *Store) SaveConversation(ctx context.Context, conversation domain.Conversation) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO conversations (
			tenant_id, id, owner_id, title, model_target, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    title = EXCLUDED.title,
		    model_target = EXCLUDED.model_target,
		    updated_at = EXCLUDED.updated_at
	`, conversation.TenantID, conversation.ID, conversation.OwnerID, conversation.Title, conversation.ModelTarget, conversation.CreatedAt, conversation.UpdatedAt)
	return err
}

func (s *Store) GetConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, owner_id, title, model_target, created_at, updated_at
		FROM conversations
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&conversation.ID,
		&conversation.TenantID,
		&conversation.OwnerID,
		&conversation.Title,
		&conversation.ModelTarget,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	if err != nil {
		return domain.Conversation{}, translateErr(err)
	}
	return conversation, nil
}

func (s *Store) ListConversations(ctx context.Context, tenantID domain.TenantID) ([]domain.Conversation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, owner_id, title, model_target, created_at, updated_at
		FROM conversations
		WHERE tenant_id = $1
		ORDER BY updated_at DESC, id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conversations := make([]domain.Conversation, 0)
	for rows.Next() {
		var conversation domain.Conversation
		if err := rows.Scan(
			&conversation.ID,
			&conversation.TenantID,
			&conversation.OwnerID,
			&conversation.Title,
			&conversation.ModelTarget,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
		); err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func (s *Store) DeleteConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM conversations
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) SaveMessage(ctx context.Context, message domain.Message) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO messages (
			tenant_id, id, conversation_id, role, content, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET conversation_id = EXCLUDED.conversation_id,
		    role = EXCLUDED.role,
		    content = EXCLUDED.content,
		    created_at = EXCLUDED.created_at
	`, message.TenantID, message.ID, message.ConversationID, message.Role, message.Content, message.CreatedAt)
	return err
}

func (s *Store) ListMessages(ctx context.Context, tenantID domain.TenantID, conversationID domain.ConversationID) ([]domain.Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, conversation_id, role, content, created_at
		FROM messages
		WHERE tenant_id = $1 AND conversation_id = $2
		ORDER BY created_at, id
	`, tenantID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]domain.Message, 0)
	for rows.Next() {
		var message domain.Message
		if err := rows.Scan(
			&message.ID,
			&message.TenantID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&message.CreatedAt,
		); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_events (
			tenant_id, id, actor_user_id, action, resource_type, resource_id, outcome, metadata, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET actor_user_id = EXCLUDED.actor_user_id,
		    action = EXCLUDED.action,
		    resource_type = EXCLUDED.resource_type,
		    resource_id = EXCLUDED.resource_id,
		    outcome = EXCLUDED.outcome,
		    metadata = EXCLUDED.metadata,
		    created_at = EXCLUDED.created_at
	`, event.TenantID, event.ID, event.ActorUserID, event.Action, event.ResourceType, event.ResourceID, event.Outcome, metadata, event.CreatedAt)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, tenantID domain.TenantID, limit int) ([]domain.AuditEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor_user_id, action, resource_type, resource_id, outcome, metadata, created_at
		FROM audit_events
		WHERE tenant_id = $1
		ORDER BY created_at DESC, id
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		var metadata []byte
		if err := rows.Scan(
			&event.ID,
			&event.TenantID,
			&event.ActorUserID,
			&event.Action,
			&event.ResourceType,
			&event.ResourceID,
			&event.Outcome,
			&metadata,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, err
			}
		}
		if event.Metadata == nil {
			event.Metadata = map[string]string{}
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func translateErr(err error) error {
	if err == pgx.ErrNoRows {
		return store.ErrNotFound
	}
	return err
}
