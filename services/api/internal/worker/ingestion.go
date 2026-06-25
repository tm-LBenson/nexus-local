package worker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/ingest"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type Clock interface {
	Now() time.Time
}

type DocumentIngestionWorker struct {
	repos     store.RepositorySet
	clock     Clock
	objects   providers.ObjectStore
	embedder  providers.Embedder
	vectors   providers.VectorIndex
	extractor ingest.Extractor
	chunker   ingest.Chunker
}

type ProcessResult struct {
	JobID      domain.JobID
	DocumentID domain.DocumentID
	ChunkCount int
}

func NewDocumentIngestionWorker(repos store.RepositorySet, clock Clock) DocumentIngestionWorker {
	return DocumentIngestionWorker{
		repos:     repos,
		clock:     clock,
		extractor: ingest.NewExtractor(10 << 20),
		chunker:   ingest.NewChunker(1200, 150),
	}
}

func (w DocumentIngestionWorker) WithPipeline(objects providers.ObjectStore, embedder providers.Embedder, vectors providers.VectorIndex) DocumentIngestionWorker {
	w.objects = objects
	w.embedder = embedder
	w.vectors = vectors
	return w
}

func (w DocumentIngestionWorker) WithTextPipeline(extractor ingest.Extractor, chunker ingest.Chunker) DocumentIngestionWorker {
	w.extractor = extractor
	w.chunker = chunker
	return w
}

func (w DocumentIngestionWorker) ProcessNext(ctx context.Context) (ProcessResult, error) {
	job, err := w.repos.ClaimNextQueuedJob(ctx, w.clock.Now())
	if err != nil {
		return ProcessResult{}, err
	}

	result, err := w.processClaimedJob(ctx, job)
	if err != nil {
		if transitionErr := job.Fail(err, w.clock.Now()); transitionErr == nil {
			_ = w.repos.SaveJob(ctx, job)
		}
		return ProcessResult{}, err
	}

	if err := job.Transition(domain.JobStateSucceeded, w.clock.Now()); err != nil {
		return ProcessResult{}, err
	}
	if err := w.repos.SaveJob(ctx, job); err != nil {
		return ProcessResult{}, err
	}

	return result, nil
}

func (w DocumentIngestionWorker) processClaimedJob(ctx context.Context, job domain.Job) (ProcessResult, error) {
	if job.Type != domain.JobTypeDocumentIngestion || job.ResourceType != "document" || job.ResourceID == "" {
		return ProcessResult{}, fmt.Errorf("unsupported ingestion job %s/%s/%s", job.Type, job.ResourceType, job.ResourceID)
	}

	documentID := domain.DocumentID(job.ResourceID)
	document, err := w.repos.GetDocument(ctx, job.TenantID, documentID)
	if err != nil {
		return ProcessResult{}, err
	}

	if err := document.Transition(domain.DocumentStatusProcessing, w.clock.Now()); err != nil {
		return ProcessResult{}, err
	}
	if err := w.repos.SaveDocument(ctx, document); err != nil {
		return ProcessResult{}, err
	}

	chunkCount, err := w.ingestDocument(ctx, document)
	if err != nil {
		if transitionErr := document.Transition(domain.DocumentStatusFailed, w.clock.Now()); transitionErr == nil {
			_ = w.repos.SaveDocument(ctx, document)
		}
		return ProcessResult{}, err
	}

	if err := document.Transition(domain.DocumentStatusReady, w.clock.Now()); err != nil {
		return ProcessResult{}, err
	}
	if err := w.repos.SaveDocument(ctx, document); err != nil {
		return ProcessResult{}, err
	}

	return ProcessResult{
		JobID:      job.ID,
		DocumentID: document.ID,
		ChunkCount: chunkCount,
	}, nil
}

func (w DocumentIngestionWorker) ingestDocument(ctx context.Context, document domain.Document) (int, error) {
	if w.objects == nil || w.embedder == nil || w.vectors == nil {
		return 0, fmt.Errorf("document ingestion pipeline is not configured")
	}

	body, info, err := w.objects.GetObject(ctx, document.TenantID, document.StorageKey)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	text, err := w.extractor.Extract(body, document.Name, info.ContentType)
	if err != nil {
		if err == io.EOF {
			return 0, nil
		}
		return 0, err
	}
	chunks := w.chunker.Chunk(text)
	if len(chunks) == 0 {
		return 0, fmt.Errorf("document produced no text chunks")
	}

	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	embedded, err := w.embedder.Embed(ctx, providers.EmbeddingRequest{Texts: texts})
	if err != nil {
		return 0, err
	}
	if len(embedded.Vectors) != len(chunks) {
		return 0, fmt.Errorf("embedding count %d does not match chunk count %d", len(embedded.Vectors), len(chunks))
	}

	vectors := make([]providers.Vector, 0, len(chunks))
	for i, chunk := range chunks {
		vectors = append(vectors, providers.Vector{
			TenantID:   document.TenantID,
			DocumentID: document.ID,
			ChunkID:    chunk.ID,
			Values:     embedded.Vectors[i],
			Text:       chunk.Text,
			Metadata: map[string]string{
				"document_name": document.Name,
				"chunk_index":   fmt.Sprintf("%d", chunk.Index),
				"storage_key":   document.StorageKey,
			},
		})
	}

	if err := w.vectors.DeleteDocument(ctx, document.TenantID, document.ID); err != nil {
		return 0, err
	}
	if err := w.vectors.Upsert(ctx, vectors); err != nil {
		return 0, err
	}
	return len(vectors), nil
}
