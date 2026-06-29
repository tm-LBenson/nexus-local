package worker

import (
	"context"
	"errors"
	"sync"

	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type DocumentIngestionProcessor interface {
	ProcessNext(ctx context.Context) (ProcessResult, error)
}

type DocumentIngestionBatchResult struct {
	Results   []ProcessResult
	Processed int
	Empty     int
	Failed    int
}

func ProcessDocumentIngestionBatch(ctx context.Context, processor DocumentIngestionProcessor, concurrency int) (DocumentIngestionBatchResult, error) {
	if err := ctx.Err(); err != nil {
		return DocumentIngestionBatchResult{}, err
	}
	if concurrency < 1 {
		concurrency = 1
	}

	outcomes := make(chan documentIngestionOutcome, concurrency)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := processor.ProcessNext(ctx)
			outcomes <- documentIngestionOutcome{result: result, err: err}
		}()
	}
	wg.Wait()
	close(outcomes)

	batch := DocumentIngestionBatchResult{
		Results: make([]ProcessResult, 0, concurrency),
	}
	var firstErr error
	for outcome := range outcomes {
		if outcome.err == nil {
			batch.Processed++
			batch.Results = append(batch.Results, outcome.result)
			continue
		}
		if errors.Is(outcome.err, store.ErrNotFound) {
			batch.Empty++
			continue
		}
		batch.Failed++
		if firstErr == nil {
			firstErr = outcome.err
		}
	}
	return batch, firstErr
}

type documentIngestionOutcome struct {
	result ProcessResult
	err    error
}
