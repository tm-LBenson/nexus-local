package memory

import (
	"context"
	"sort"
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
	jobs          map[tenantJobKey]domain.Job
	conversations map[tenantConversationKey]domain.Conversation
	messages      map[tenantMessageKey]domain.Message
}

type membershipKey struct {
	tenantID domain.TenantID
	userID   domain.UserID
}

type tenantDocumentKey struct {
	tenantID   domain.TenantID
	documentID domain.DocumentID
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

func New() *Store {
	return &Store{
		tenants:       map[domain.TenantID]domain.Tenant{},
		users:         map[domain.UserID]domain.User{},
		memberships:   map[membershipKey]domain.Membership{},
		documents:     map[tenantDocumentKey]domain.Document{},
		jobs:          map[tenantJobKey]domain.Job{},
		conversations: map[tenantConversationKey]domain.Conversation{},
		messages:      map[tenantMessageKey]domain.Message{},
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

func (s *Store) ClaimNextQueuedJob(ctx context.Context, now time.Time) (domain.Job, error) {
	if err := ctx.Err(); err != nil {
		return domain.Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	queued := make([]domain.Job, 0)
	for _, job := range s.jobs {
		if job.State == domain.JobStateQueued {
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
