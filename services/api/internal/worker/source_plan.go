package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/ingest"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourcePlanWorker struct {
	repos  store.RepositorySet
	clock  Clock
	policy SourceScanPolicy
}

type SourcePlanResult struct {
	JobID    domain.JobID
	SourceID domain.DataSourceID
	Summary  SourcePlanSummary
}

type SourcePlanSummary struct {
	TotalEntries   int                `json:"total_entries"`
	FilesSeen      int                `json:"files_seen"`
	WouldImport    int                `json:"would_import"`
	Skipped        int                `json:"skipped"`
	Failed         int                `json:"failed"`
	EstimatedBytes int64              `json:"estimated_bytes"`
	Reasons        map[string]int     `json:"reasons"`
	Samples        []SourcePlanSample `json:"samples,omitempty"`
}

type SourcePlanSample struct {
	Path      string `json:"path"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Message   string `json:"message,omitempty"`
}

const sourcePlanSampleLimit = 12

func NewSourcePlanWorker(repos store.RepositorySet, clock Clock) SourcePlanWorker {
	return SourcePlanWorker{
		repos:  repos,
		clock:  clock,
		policy: DefaultSourceScanPolicy(),
	}
}

func (w SourcePlanWorker) WithPolicy(policy SourceScanPolicy) SourcePlanWorker {
	w.policy = policy.normalized()
	return w
}

func (w SourcePlanWorker) ProcessNext(ctx context.Context) (SourcePlanResult, error) {
	job, err := w.repos.ClaimNextQueuedJob(ctx, w.clock.Now(), domain.JobTypeSourcePlan)
	if err != nil {
		return SourcePlanResult{}, err
	}

	result, err := w.processClaimedJob(ctx, job)
	if err != nil {
		if errors.Is(err, ErrSourceArchived) {
			if transitionErr := job.Transition(domain.JobStateCanceled, w.clock.Now()); transitionErr != nil {
				return SourcePlanResult{}, transitionErr
			}
			if saveErr := w.repos.SaveJob(ctx, job); saveErr != nil {
				return SourcePlanResult{}, saveErr
			}
			return result, nil
		}
		if transitionErr := job.Fail(err, w.clock.Now()); transitionErr == nil {
			_ = w.repos.SaveJob(ctx, job)
		}
		return SourcePlanResult{}, err
	}

	encoded, err := json.Marshal(result.Summary)
	if err != nil {
		return SourcePlanResult{}, err
	}
	job.ResultJSON = string(encoded)
	if err := job.Transition(domain.JobStateSucceeded, w.clock.Now()); err != nil {
		return SourcePlanResult{}, err
	}
	if err := w.repos.SaveJob(ctx, job); err != nil {
		return SourcePlanResult{}, err
	}
	return result, nil
}

func (w SourcePlanWorker) processClaimedJob(ctx context.Context, job domain.Job) (SourcePlanResult, error) {
	if job.Type != domain.JobTypeSourcePlan || job.ResourceType != "data_source" || job.ResourceID == "" {
		return SourcePlanResult{}, fmt.Errorf("unsupported source plan job %s/%s/%s", job.Type, job.ResourceType, job.ResourceID)
	}

	sourceID := domain.DataSourceID(job.ResourceID)
	source, err := w.repos.GetDataSource(ctx, job.TenantID, sourceID)
	if err != nil {
		return SourcePlanResult{}, err
	}
	result := SourcePlanResult{
		JobID:    job.ID,
		SourceID: source.ID,
	}

	if source.Status == domain.DataSourceStatusArchived {
		return result, ErrSourceArchived
	}

	summary, err := w.planSource(ctx, source)
	result.Summary = summary
	return result, err
}

func (w SourcePlanWorker) planSource(ctx context.Context, source domain.DataSource) (SourcePlanSummary, error) {
	summary := SourcePlanSummary{
		Reasons: map[string]int{},
	}
	root := filepath.Clean(source.RootPath)
	info, err := os.Stat(root)
	if err != nil {
		return summary, fmt.Errorf("source plan cannot access %q: %w", source.RootPath, err)
	}
	if !info.IsDir() {
		return summary, fmt.Errorf("source plan root %q is not a directory", source.RootPath)
	}

	policy := w.policy.normalized()
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relativeName := sourceRelativePath(root, path)
		if walkErr != nil {
			summary.add("failed", "walk_error", relativeName, 0, walkErr.Error())
			return nil
		}
		summary.TotalEntries++
		if entry.IsDir() {
			if sourcePatternMatches(source.ExcludePatterns, relativeName, true) {
				summary.add("skipped", "excluded", relativeName+"/", 0, "directory excluded by source pattern")
				return filepath.SkipDir
			}
			if policy.shouldSkipName(entry.Name()) || policy.shouldSkipDirectory(entry.Name()) {
				summary.add("skipped", "policy", relativeName+"/", 0, "directory excluded by scan policy")
				return filepath.SkipDir
			}
			return nil
		}

		summary.FilesSeen++
		if entry.Type()&os.ModeSymlink != 0 {
			summary.add("skipped", "symlink", relativeName, 0, "symbolic links are skipped")
			return nil
		}
		if policy.shouldSkipName(entry.Name()) {
			summary.add("skipped", "policy", relativeName, 0, "hidden file excluded by scan policy")
			return nil
		}
		if sourcePatternMatches(source.ExcludePatterns, relativeName, false) {
			summary.add("skipped", "excluded", relativeName, 0, "file excluded by source pattern")
			return nil
		}
		if len(source.IncludePatterns) > 0 && !sourcePatternMatches(source.IncludePatterns, relativeName, false) {
			summary.add("skipped", "not_included", relativeName, 0, "file does not match source include patterns")
			return nil
		}

		fileInfo, err := entry.Info()
		if err != nil {
			summary.add("failed", "stat_failed", relativeName, 0, err.Error())
			return nil
		}
		if policy.MaxFileBytes > 0 && fileInfo.Size() > policy.MaxFileBytes {
			summary.add("skipped", "too_large", relativeName, fileInfo.Size(), fmt.Sprintf("%d bytes exceeds scan limit of %d bytes", fileInfo.Size(), policy.MaxFileBytes))
			return nil
		}

		contentType := contentTypeForPath(path)
		if err := ingest.ValidateDocumentType(relativeName, contentType); err != nil {
			summary.add("skipped", "unsupported_type", relativeName, fileInfo.Size(), err.Error())
			return nil
		}
		summary.add("would_import", "", relativeName, fileInfo.Size(), "")
		return nil
	})
	if err != nil {
		return summary, err
	}
	return summary, nil
}

func (s *SourcePlanSummary) add(outcome string, reason string, path string, sizeBytes int64, message string) {
	switch outcome {
	case "would_import":
		s.WouldImport++
		s.EstimatedBytes += sizeBytes
	case "skipped":
		s.Skipped++
	case "failed":
		s.Failed++
	}
	if reason != "" {
		s.Reasons[reason]++
	}
	if len(s.Samples) >= sourcePlanSampleLimit {
		return
	}
	s.Samples = append(s.Samples, SourcePlanSample{
		Path:      path,
		Outcome:   outcome,
		Reason:    reason,
		SizeBytes: sizeBytes,
		Message:   message,
	})
}
