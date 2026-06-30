package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type Store struct {
	mu sync.RWMutex

	tenants       map[domain.TenantID]domain.Tenant
	users         map[domain.UserID]domain.User
	memberships   map[membershipKey]domain.Membership
	documents     map[tenantDocumentKey]domain.Document
	dataSources   map[tenantDataSourceKey]domain.DataSource
	scanEntries   map[tenantScanEntryKey]domain.DataSourceScanEntry
	jobs          map[tenantJobKey]domain.Job
	conversations map[tenantConversationKey]domain.Conversation
	messages      map[tenantMessageKey]domain.Message
	auditEvents   map[tenantAuditEventKey]domain.AuditEvent
}

type membershipKey struct {
	tenantID domain.TenantID
	userID   domain.UserID
}

type tenantDocumentKey struct {
	tenantID   domain.TenantID
	documentID domain.DocumentID
}

type tenantDataSourceKey struct {
	tenantID     domain.TenantID
	dataSourceID domain.DataSourceID
}

type tenantScanEntryKey struct {
	tenantID domain.TenantID
	jobID    domain.JobID
	path     string
}

type tenantJobKey struct {
	tenantID domain.TenantID
	jobID    domain.JobID
}

type tenantConversationKey struct {
	tenantID       domain.TenantID
	conversationID domain.ConversationID
}

type tenantMessageKey struct {
	tenantID       domain.TenantID
	conversationID domain.ConversationID
	messageID      domain.MessageID
}

type tenantAuditEventKey struct {
	tenantID domain.TenantID
	eventID  domain.AuditEventID
}

func New() *Store {
	return &Store{
		tenants:       map[domain.TenantID]domain.Tenant{},
		users:         map[domain.UserID]domain.User{},
		memberships:   map[membershipKey]domain.Membership{},
		documents:     map[tenantDocumentKey]domain.Document{},
		dataSources:   map[tenantDataSourceKey]domain.DataSource{},
		scanEntries:   map[tenantScanEntryKey]domain.DataSourceScanEntry{},
		jobs:          map[tenantJobKey]domain.Job{},
		conversations: map[tenantConversationKey]domain.Conversation{},
		messages:      map[tenantMessageKey]domain.Message{},
		auditEvents:   map[tenantAuditEventKey]domain.AuditEvent{},
	}
}

func (s *Store) SaveTenant(ctx context.Context, tenant domain.Tenant) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[tenant.ID] = tenant
	return nil
}

func (s *Store) GetTenant(ctx context.Context, id domain.TenantID) (domain.Tenant, error) {
	if err := ctx.Err(); err != nil {
		return domain.Tenant{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tenant, ok := s.tenants[id]
	if !ok {
		return domain.Tenant{}, store.ErrNotFound
	}
	return tenant, nil
}

func (s *Store) DeleteTenant(ctx context.Context, id domain.TenantID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tenants[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.tenants, id)
	for key := range s.memberships {
		if key.tenantID == id {
			delete(s.memberships, key)
		}
	}
	for key := range s.documents {
		if key.tenantID == id {
			delete(s.documents, key)
		}
	}
	for key := range s.dataSources {
		if key.tenantID == id {
			delete(s.dataSources, key)
		}
	}
	for key := range s.scanEntries {
		if key.tenantID == id {
			delete(s.scanEntries, key)
		}
	}
	for key := range s.jobs {
		if key.tenantID == id {
			delete(s.jobs, key)
		}
	}
	for key := range s.conversations {
		if key.tenantID == id {
			delete(s.conversations, key)
		}
	}
	for key := range s.messages {
		if key.tenantID == id {
			delete(s.messages, key)
		}
	}
	for key := range s.auditEvents {
		if key.tenantID == id {
			delete(s.auditEvents, key)
		}
	}
	return nil
}

func (s *Store) SaveUser(ctx context.Context, user domain.User) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[user.ID] = user
	return nil
}

func (s *Store) GetUser(ctx context.Context, id domain.UserID) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return domain.User{}, store.ErrNotFound
	}
	return user, nil
}

func (s *Store) SaveMembership(ctx context.Context, membership domain.Membership) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memberships[membershipKey{
		tenantID: membership.TenantID,
		userID:   membership.UserID,
	}] = membership
	return nil
}

func (s *Store) ListMembershipsForUser(ctx context.Context, userID domain.UserID) ([]domain.Membership, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	memberships := make([]domain.Membership, 0)
	for _, membership := range s.memberships {
		if membership.UserID == userID {
			memberships = append(memberships, membership)
		}
	}
	sort.Slice(memberships, func(i, j int) bool {
		return memberships[i].TenantID < memberships[j].TenantID
	})
	return memberships, nil
}

func (s *Store) ListMembershipsForTenant(ctx context.Context, tenantID domain.TenantID) ([]domain.Membership, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	memberships := make([]domain.Membership, 0)
	for _, membership := range s.memberships {
		if membership.TenantID == tenantID {
			memberships = append(memberships, membership)
		}
	}
	sort.Slice(memberships, func(i, j int) bool {
		return memberships[i].UserID < memberships[j].UserID
	})
	return memberships, nil
}

func (s *Store) DeleteMembership(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	key := membershipKey{
		tenantID: tenantID,
		userID:   userID,
	}
	if _, ok := s.memberships[key]; !ok {
		return store.ErrNotFound
	}
	delete(s.memberships, key)
	return nil
}

func (s *Store) SaveDocument(ctx context.Context, document domain.Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[tenantDocumentKey{
		tenantID:   document.TenantID,
		documentID: document.ID,
	}] = document
	return nil
}

func (s *Store) GetDocument(ctx context.Context, tenantID domain.TenantID, id domain.DocumentID) (domain.Document, error) {
	if err := ctx.Err(); err != nil {
		return domain.Document{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	document, ok := s.documents[tenantDocumentKey{tenantID: tenantID, documentID: id}]
	if !ok {
		return domain.Document{}, store.ErrNotFound
	}
	return document, nil
}

func (s *Store) ListDocuments(ctx context.Context, tenantID domain.TenantID) ([]domain.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	documents := make([]domain.Document, 0)
	for _, document := range s.documents {
		if document.TenantID == tenantID {
			documents = append(documents, document)
		}
	}
	sort.Slice(documents, func(i, j int) bool {
		if documents[i].CreatedAt.Equal(documents[j].CreatedAt) {
			return documents[i].ID < documents[j].ID
		}
		return documents[i].CreatedAt.Before(documents[j].CreatedAt)
	})
	return documents, nil
}

func (s *Store) SaveDataSource(ctx context.Context, source domain.DataSource) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataSources[tenantDataSourceKey{
		tenantID:     source.TenantID,
		dataSourceID: source.ID,
	}] = source
	return nil
}

func (s *Store) GetDataSource(ctx context.Context, tenantID domain.TenantID, id domain.DataSourceID) (domain.DataSource, error) {
	if err := ctx.Err(); err != nil {
		return domain.DataSource{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.dataSources[tenantDataSourceKey{tenantID: tenantID, dataSourceID: id}]
	if !ok {
		return domain.DataSource{}, store.ErrNotFound
	}
	return source, nil
}

func (s *Store) ListDataSources(ctx context.Context, tenantID domain.TenantID) ([]domain.DataSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	sources := make([]domain.DataSource, 0)
	for _, source := range s.dataSources {
		if source.TenantID == tenantID {
			sources = append(sources, source)
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].UpdatedAt.Equal(sources[j].UpdatedAt) {
			return sources[i].ID < sources[j].ID
		}
		return sources[i].UpdatedAt.After(sources[j].UpdatedAt)
	})
	return sources, nil
}

func (s *Store) ListDueDataSources(ctx context.Context, now time.Time, limit int) ([]domain.DataSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	sources := make([]domain.DataSource, 0)
	for _, source := range s.dataSources {
		if source.ScanIntervalMinutes <= 0 ||
			source.NextScanAt == nil ||
			source.NextScanAt.After(now) ||
			source.Status == domain.DataSourceStatusArchived ||
			source.Status == domain.DataSourceStatusScanning {
			continue
		}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].NextScanAt.Equal(*sources[j].NextScanAt) {
			if sources[i].TenantID != sources[j].TenantID {
				return sources[i].TenantID < sources[j].TenantID
			}
			return sources[i].ID < sources[j].ID
		}
		return sources[i].NextScanAt.Before(*sources[j].NextScanAt)
	})
	if limit > 0 && len(sources) > limit {
		sources = sources[:limit]
	}
	return sources, nil
}

func (s *Store) SaveDataSourceScanEntry(ctx context.Context, entry domain.DataSourceScanEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanEntries[tenantScanEntryKey{
		tenantID: entry.TenantID,
		jobID:    entry.JobID,
		path:     entry.Path,
	}] = entry
	return nil
}

func (s *Store) ListDataSourceScanEntries(ctx context.Context, tenantID domain.TenantID, sourceID domain.DataSourceID, limit int) ([]domain.DataSourceScanEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]domain.DataSourceScanEntry, 0)
	for _, entry := range s.scanEntries {
		if entry.TenantID == tenantID && entry.SourceID == sourceID {
			entries = append(entries, entry)
		}
	}
	sortScanEntries(entries)
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func sortScanEntries(entries []domain.DataSourceScanEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			if entries[i].JobID != entries[j].JobID {
				return entries[i].JobID > entries[j].JobID
			}
			return entries[i].Path < entries[j].Path
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
}

func (s *Store) ListDataSourceScanEntryPage(ctx context.Context, tenantID domain.TenantID, sourceID domain.DataSourceID, filter store.DataSourceScanEntryFilter) (store.DataSourceScanEntryPage, error) {
	if err := ctx.Err(); err != nil {
		return store.DataSourceScanEntryPage{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]domain.DataSourceScanEntry, 0)
	for _, entry := range s.scanEntries {
		if entry.TenantID != tenantID || entry.SourceID != sourceID {
			continue
		}
		if filter.Outcome != "" && entry.Outcome != filter.Outcome {
			continue
		}
		entries = append(entries, entry)
	}
	sortScanEntries(entries)
	total := len(entries)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(entries) {
		entries = entries[:0]
	} else if offset > 0 {
		entries = entries[offset:]
	}
	limit := filter.Limit
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return store.DataSourceScanEntryPage{
		Entries: entries,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

func (s *Store) SaveJob(ctx context.Context, job domain.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[tenantJobKey{
		tenantID: job.TenantID,
		jobID:    job.ID,
	}] = job
	return nil
}

func (s *Store) GetJob(ctx context.Context, tenantID domain.TenantID, id domain.JobID) (domain.Job, error) {
	if err := ctx.Err(); err != nil {
		return domain.Job{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[tenantJobKey{tenantID: tenantID, jobID: id}]
	if !ok {
		return domain.Job{}, store.ErrNotFound
	}
	return job, nil
}

func (s *Store) ListJobs(ctx context.Context, tenantID domain.TenantID, limit int) ([]domain.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]domain.Job, 0)
	for _, job := range s.jobs {
		if job.TenantID == tenantID {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].UpdatedAt.Equal(jobs[j].UpdatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].UpdatedAt.After(jobs[j].UpdatedAt)
	})
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs, nil
}

func (s *Store) ClaimNextQueuedJob(ctx context.Context, now time.Time, types ...domain.JobType) (domain.Job, error) {
	if err := ctx.Err(); err != nil {
		return domain.Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	allowedTypes := map[domain.JobType]struct{}{}
	for _, jobType := range types {
		allowedTypes[jobType] = struct{}{}
	}

	queued := make([]domain.Job, 0)
	for _, job := range s.jobs {
		if job.State == domain.JobStateQueued && jobTypeAllowed(job.Type, allowedTypes) {
			queued = append(queued, job)
		}
	}
	if len(queued) == 0 {
		return domain.Job{}, store.ErrNotFound
	}

	sort.Slice(queued, func(i, j int) bool {
		if queued[i].CreatedAt.Equal(queued[j].CreatedAt) {
			return queued[i].ID < queued[j].ID
		}
		return queued[i].CreatedAt.Before(queued[j].CreatedAt)
	})

	job := queued[0]
	if err := job.Transition(domain.JobStateRunning, now); err != nil {
		return domain.Job{}, err
	}
	s.jobs[tenantJobKey{tenantID: job.TenantID, jobID: job.ID}] = job
	return job, nil
}

func jobTypeAllowed(jobType domain.JobType, allowedTypes map[domain.JobType]struct{}) bool {
	if len(allowedTypes) == 0 {
		return true
	}
	_, ok := allowedTypes[jobType]
	return ok
}

func (s *Store) SaveConversation(ctx context.Context, conversation domain.Conversation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conversations[tenantConversationKey{
		tenantID:       conversation.TenantID,
		conversationID: conversation.ID,
	}] = conversation
	return nil
}

func (s *Store) GetConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) (domain.Conversation, error) {
	if err := ctx.Err(); err != nil {
		return domain.Conversation{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	conversation, ok := s.conversations[tenantConversationKey{tenantID: tenantID, conversationID: id}]
	if !ok {
		return domain.Conversation{}, store.ErrNotFound
	}
	return conversation, nil
}

func (s *Store) ListConversations(ctx context.Context, tenantID domain.TenantID) ([]domain.Conversation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversations := make([]domain.Conversation, 0)
	for _, conversation := range s.conversations {
		if conversation.TenantID == tenantID {
			conversations = append(conversations, conversation)
		}
	}
	sort.Slice(conversations, func(i, j int) bool {
		if conversations[i].UpdatedAt.Equal(conversations[j].UpdatedAt) {
			return conversations[i].ID < conversations[j].ID
		}
		return conversations[i].UpdatedAt.After(conversations[j].UpdatedAt)
	})
	return conversations, nil
}

func (s *Store) DeleteConversation(ctx context.Context, tenantID domain.TenantID, id domain.ConversationID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	key := tenantConversationKey{tenantID: tenantID, conversationID: id}
	if _, ok := s.conversations[key]; !ok {
		return store.ErrNotFound
	}
	delete(s.conversations, key)
	for messageKey, message := range s.messages {
		if message.TenantID == tenantID && message.ConversationID == id {
			delete(s.messages, messageKey)
		}
	}
	return nil
}

func (s *Store) SaveMessage(ctx context.Context, message domain.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[tenantMessageKey{
		tenantID:       message.TenantID,
		conversationID: message.ConversationID,
		messageID:      message.ID,
	}] = message
	return nil
}

func (s *Store) ListMessages(ctx context.Context, tenantID domain.TenantID, conversationID domain.ConversationID) ([]domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]domain.Message, 0)
	for _, message := range s.messages {
		if message.TenantID == tenantID && message.ConversationID == conversationID {
			messages = append(messages, message)
		}
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
	return messages, nil
}

func (s *Store) SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditEvents[tenantAuditEventKey{
		tenantID: event.TenantID,
		eventID:  event.ID,
	}] = event
	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context, tenantID domain.TenantID, filter store.AuditEventFilter) ([]domain.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]domain.AuditEvent, 0)
	for _, event := range s.auditEvents {
		if event.TenantID == tenantID && auditEventMatches(event, filter) {
			events = append(events, event)
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].ID < events[j].ID
		}
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	if filter.Limit > 0 && len(events) > filter.Limit {
		events = events[:filter.Limit]
	}
	return events, nil
}

func auditEventMatches(event domain.AuditEvent, filter store.AuditEventFilter) bool {
	if filter.Action != "" && event.Action != filter.Action {
		return false
	}
	if filter.Outcome != "" && event.Outcome != filter.Outcome {
		return false
	}
	if filter.ActorUserID != "" && event.ActorUserID != filter.ActorUserID {
		return false
	}
	if filter.From != nil && event.CreatedAt.Before(*filter.From) {
		return false
	}
	if filter.To != nil && event.CreatedAt.After(*filter.To) {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	if query == "" {
		return true
	}
	metadata := make([]string, 0, len(event.Metadata)*2)
	for key, value := range event.Metadata {
		metadata = append(metadata, key, value)
	}
	haystack := strings.ToLower(strings.Join([]string{
		string(event.ID),
		string(event.ActorUserID),
		event.Action,
		event.ResourceType,
		event.ResourceID,
		string(event.Outcome),
		strings.Join(metadata, " "),
	}, " "))
	return strings.Contains(haystack, query)
}
