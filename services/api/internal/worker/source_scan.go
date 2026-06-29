package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tm-lbenson/nexus-local/services/api/internal/app"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
	"github.com/tm-lbenson/nexus-local/services/api/internal/ingest"
	"github.com/tm-lbenson/nexus-local/services/api/internal/providers"
	"github.com/tm-lbenson/nexus-local/services/api/internal/store"
)

type SourceScanIDs interface {
	NewDocumentID() domain.DocumentID
	NewJobID() domain.JobID
}

type SourceScanWorker struct {
	repos     store.RepositorySet
	clock     Clock
	documents app.DocumentService
	policy    SourceScanPolicy
}

type SourceScanResult struct {
	JobID                   domain.JobID        `json:"job_id"`
	SourceID                domain.DataSourceID `json:"source_id"`
	StartedAt               time.Time           `json:"started_at"`
	FinishedAt              time.Time           `json:"finished_at"`
	DurationMS              int64               `json:"duration_ms"`
	ImportedCount           int                 `json:"imported"`
	SkippedCount            int                 `json:"skipped"`
	DeletedCount            int                 `json:"deleted"`
	SkippedUnsupportedCount int                 `json:"skipped_unsupported"`
	SkippedPolicyCount      int                 `json:"skipped_policy"`
	SkippedTooLargeCount    int                 `json:"skipped_too_large"`
	FailedCount             int                 `json:"failed"`
}

type SourceScanPolicy struct {
	MaxFileBytes      int64
	SkipHiddenNames   bool
	SkipDirectoryName map[string]bool
}

func NewSourceScanWorker(repos store.RepositorySet, ids SourceScanIDs, clock Clock) SourceScanWorker {
	return SourceScanWorker{
		repos:     repos,
		clock:     clock,
		documents: app.NewDocumentService(repos, ids, clock),
		policy:    DefaultSourceScanPolicy(),
	}
}

func (w SourceScanWorker) WithObjectStore(objects providers.ObjectStore) SourceScanWorker {
	w.documents = w.documents.WithObjectStore(objects)
	return w
}

func (w SourceScanWorker) WithVectorIndex(vectors providers.VectorIndex) SourceScanWorker {
	w.documents = w.documents.WithVectorIndex(vectors)
	return w
}

func (w SourceScanWorker) WithPolicy(policy SourceScanPolicy) SourceScanWorker {
	w.policy = policy.normalized()
	return w
}

const defaultSourceScanMaxFileBytes = 10 << 20

func DefaultSourceScanPolicy() SourceScanPolicy {
	return SourceScanPolicy{
		MaxFileBytes:    defaultSourceScanMaxFileBytes,
		SkipHiddenNames: true,
		SkipDirectoryName: map[string]bool{
			"node_modules":              true,
			".git":                      true,
			".hg":                       true,
			".svn":                      true,
			".cache":                    true,
			"__pycache__":               true,
			".pytest_cache":             true,
			".mypy_cache":               true,
			".next":                     true,
			"dist":                      true,
			"build":                     true,
			"target":                    true,
			"tmp":                       true,
			"temp":                      true,
			".venv":                     true,
			"venv":                      true,
			"$recycle.bin":              true,
			"system volume information": true,
		},
	}
}

func (w SourceScanWorker) ProcessNext(ctx context.Context) (SourceScanResult, error) {
	job, err := w.repos.ClaimNextQueuedJob(ctx, w.clock.Now(), domain.JobTypeSourceScan)
	if err != nil {
		return SourceScanResult{}, err
	}

	result, err := w.processClaimedJob(ctx, job)
	if err != nil {
		if errors.Is(err, ErrSourceArchived) || errors.Is(err, ErrSourceScanCanceled) {
			if err := w.finishSourceScanJob(&job, &result); err != nil {
				return SourceScanResult{}, err
			}
			if transitionErr := job.Transition(domain.JobStateCanceled, result.FinishedAt); transitionErr != nil {
				return SourceScanResult{}, transitionErr
			}
			if saveErr := w.repos.SaveJob(ctx, job); saveErr != nil {
				return SourceScanResult{}, saveErr
			}
			return result, nil
		}
		if err := w.finishSourceScanJob(&job, &result); err != nil {
			return SourceScanResult{}, err
		}
		if transitionErr := job.Fail(err, result.FinishedAt); transitionErr == nil {
			_ = w.repos.SaveJob(ctx, job)
		}
		return result, err
	}

	if err := w.finishSourceScanJob(&job, &result); err != nil {
		return SourceScanResult{}, err
	}
	if err := job.Transition(domain.JobStateSucceeded, result.FinishedAt); err != nil {
		return SourceScanResult{}, err
	}
	if err := w.repos.SaveJob(ctx, job); err != nil {
		return SourceScanResult{}, err
	}
	return result, nil
}

var ErrSourceArchived = errors.New("source was archived before scan")
var ErrSourceScanCanceled = errors.New("source scan was canceled")

func (w SourceScanWorker) processClaimedJob(ctx context.Context, job domain.Job) (SourceScanResult, error) {
	if job.Type != domain.JobTypeSourceScan || job.ResourceType != "data_source" || job.ResourceID == "" {
		return SourceScanResult{}, fmt.Errorf("unsupported source scan job %s/%s/%s", job.Type, job.ResourceType, job.ResourceID)
	}

	sourceID := domain.DataSourceID(job.ResourceID)
	source, err := w.repos.GetDataSource(ctx, job.TenantID, sourceID)
	if err != nil {
		return SourceScanResult{}, err
	}
	result := SourceScanResult{
		JobID:     job.ID,
		SourceID:  source.ID,
		StartedAt: job.UpdatedAt,
	}

	if source.Status == domain.DataSourceStatusArchived {
		return result, ErrSourceArchived
	}
	if source.Status != domain.DataSourceStatusScanning {
		if err := source.Transition(domain.DataSourceStatusScanning, w.clock.Now()); err != nil {
			return SourceScanResult{}, err
		}
		if err := w.repos.SaveDataSource(ctx, source); err != nil {
			return SourceScanResult{}, err
		}
	}

	result, scanErr := w.scanSource(ctx, source, result)
	if scanErr != nil {
		if errors.Is(scanErr, ErrSourceScanCanceled) {
			_ = w.markSourceCanceled(ctx, source)
			return result, scanErr
		}
		_ = w.markSourceFailed(ctx, source, result)
		return result, scanErr
	}

	latestSource, err := w.repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		return SourceScanResult{}, err
	}
	if latestSource.Status != domain.DataSourceStatusArchived {
		if latestSource.Status != domain.DataSourceStatusScanning {
			if err := latestSource.Transition(domain.DataSourceStatusScanning, w.clock.Now()); err != nil {
				return SourceScanResult{}, err
			}
		}
		if err := latestSource.CompleteScan(result.ImportedCount, result.SkippedCount, result.FailedCount, w.clock.Now()); err != nil {
			return SourceScanResult{}, err
		}
		if err := w.repos.SaveDataSource(ctx, latestSource); err != nil {
			return SourceScanResult{}, err
		}
	}
	return result, nil
}

func (w SourceScanWorker) scanSource(ctx context.Context, source domain.DataSource, result SourceScanResult) (SourceScanResult, error) {
	root := filepath.Clean(source.RootPath)
	info, err := os.Stat(root)
	if err != nil {
		result.FailedCount++
		if saveErr := w.recordScanEntry(ctx, source, result.JobID, ".", domain.DataSourceScanOutcomeFailed, "cannot_access", err.Error(), "", 0, ""); saveErr != nil {
			return result, saveErr
		}
		return result, fmt.Errorf("source scan cannot access %q: %w", source.RootPath, err)
	}
	if !info.IsDir() {
		result.FailedCount++
		if saveErr := w.recordScanEntry(ctx, source, result.JobID, ".", domain.DataSourceScanOutcomeFailed, "not_directory", "source root is not a directory", "", 0, ""); saveErr != nil {
			return result, saveErr
		}
		return result, fmt.Errorf("source scan root %q is not a directory", source.RootPath)
	}

	policy := w.policy.normalized()
	previousImported, err := w.activeSourceFileEntries(ctx, source)
	if err != nil {
		return result, err
	}
	seenPaths := map[string]bool{}
	skippedDirectoryPrefixes := make([]string, 0)
	var firstFailure error
	if err := w.ensureSourceScanNotCanceled(ctx, source.TenantID, result.JobID); err != nil {
		return result, err
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			result.FailedCount++
			relativeName := sourceRelativePath(root, path)
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "walk_error", walkErr.Error(), "", 0, ""); saveErr != nil {
				return saveErr
			}
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, walkErr)
			}
			return nil
		}
		if path == root {
			return nil
		}
		if err := w.ensureSourceScanNotCanceled(ctx, source.TenantID, result.JobID); err != nil {
			return err
		}
		relativeName := sourceRelativePath(root, path)
		if entry.IsDir() {
			if sourcePatternMatches(source.ExcludePatterns, relativeName, true) {
				result.SkippedCount++
				result.SkippedPolicyCount++
				if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName+"/", domain.DataSourceScanOutcomeSkipped, "excluded", "directory excluded by source pattern", "", 0, ""); saveErr != nil {
					return saveErr
				}
				return filepath.SkipDir
			}
			if policy.shouldSkipName(entry.Name()) || policy.shouldSkipDirectory(entry.Name()) {
				result.SkippedCount++
				result.SkippedPolicyCount++
				skippedDirectoryPrefixes = append(skippedDirectoryPrefixes, relativeName+"/")
				if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName+"/", domain.DataSourceScanOutcomeSkipped, "policy", "directory excluded by scan policy", "", 0, ""); saveErr != nil {
					return saveErr
				}
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			seenPaths[relativeName] = true
			result.SkippedCount++
			result.SkippedPolicyCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "symlink", "symbolic links are skipped", "", 0, ""); saveErr != nil {
				return saveErr
			}
			return nil
		}
		if policy.shouldSkipName(entry.Name()) {
			seenPaths[relativeName] = true
			result.SkippedCount++
			result.SkippedPolicyCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "policy", "hidden file excluded by scan policy", "", 0, ""); saveErr != nil {
				return saveErr
			}
			return nil
		}
		if sourcePatternMatches(source.ExcludePatterns, relativeName, false) {
			result.SkippedCount++
			result.SkippedPolicyCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "excluded", "file excluded by source pattern", "", 0, ""); saveErr != nil {
				return saveErr
			}
			return nil
		}
		if len(source.IncludePatterns) > 0 && !sourcePatternMatches(source.IncludePatterns, relativeName, false) {
			result.SkippedCount++
			result.SkippedPolicyCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "not_included", "file does not match source include patterns", "", 0, ""); saveErr != nil {
				return saveErr
			}
			return nil
		}
		seenPaths[relativeName] = true

		fileInfo, err := entry.Info()
		if err != nil {
			result.FailedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "stat_failed", err.Error(), "", 0, ""); saveErr != nil {
				return saveErr
			}
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, err)
			}
			return nil
		}
		if policy.MaxFileBytes > 0 && fileInfo.Size() > policy.MaxFileBytes {
			result.SkippedCount++
			result.SkippedTooLargeCount++
			message := fmt.Sprintf("%d bytes exceeds scan limit of %d bytes", fileInfo.Size(), policy.MaxFileBytes)
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "too_large", message, "", fileInfo.Size(), ""); saveErr != nil {
				return saveErr
			}
			return nil
		}

		contentType := contentTypeForPath(path)
		if err := ingest.ValidateDocumentType(relativeName, contentType); err != nil {
			result.SkippedCount++
			result.SkippedUnsupportedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "unsupported_type", err.Error(), "", fileInfo.Size(), ""); saveErr != nil {
				return saveErr
			}
			return nil
		}
		contentHash, err := fileContentHash(path)
		if err != nil {
			result.FailedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "hash_failed", err.Error(), "", fileInfo.Size(), ""); saveErr != nil {
				return saveErr
			}
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, err)
			}
			return nil
		}
		if previous, ok := previousImported[relativeName]; ok && previous.ContentHash == contentHash {
			result.SkippedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeSkipped, "unchanged", "unchanged since previous scan", previous.DocumentID, fileInfo.Size(), contentHash); saveErr != nil {
				return saveErr
			}
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			result.FailedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "open_failed", err.Error(), "", fileInfo.Size(), contentHash); saveErr != nil {
				return saveErr
			}
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, err)
			}
			return nil
		}
		uploadResult, uploadErr := w.documents.UploadDocument(ctx, app.UploadDocumentInput{
			TenantID:    source.TenantID,
			OwnerID:     source.OwnerID,
			Name:        relativeName,
			ContentType: contentType,
			SizeBytes:   fileInfo.Size(),
			Body:        file,
		})
		closeErr := file.Close()
		if uploadErr != nil {
			result.FailedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "upload_failed", uploadErr.Error(), "", fileInfo.Size(), contentHash); saveErr != nil {
				return saveErr
			}
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, uploadErr)
			}
			return nil
		}
		if closeErr != nil {
			result.FailedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "close_failed", closeErr.Error(), uploadResult.Document.ID, fileInfo.Size(), contentHash); saveErr != nil {
				return saveErr
			}
			if firstFailure == nil {
				firstFailure = fmt.Errorf("%s: %w", relativeName, closeErr)
			}
			return nil
		}
		previous := previousImported[relativeName]
		reason := ""
		message := ""
		if previous.DocumentID != "" && previous.DocumentID != uploadResult.Document.ID {
			if _, err := w.documents.DeleteDocument(ctx, app.DeleteDocumentInput{
				TenantID:   source.TenantID,
				DocumentID: previous.DocumentID,
			}); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					reason = "changed"
					message = fmt.Sprintf("previous document %s was already gone", previous.DocumentID)
					result.ImportedCount++
					if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeImported, reason, message, uploadResult.Document.ID, fileInfo.Size(), contentHash); saveErr != nil {
						return saveErr
					}
					return nil
				}
				result.FailedCount++
				if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeFailed, "replace_failed", err.Error(), uploadResult.Document.ID, fileInfo.Size(), contentHash); saveErr != nil {
					return saveErr
				}
				if firstFailure == nil {
					firstFailure = fmt.Errorf("%s: replace previous document %s: %w", relativeName, previous.DocumentID, err)
				}
				return nil
			}
			reason = "changed"
			message = fmt.Sprintf("replaced previous document %s", previous.DocumentID)
		}
		result.ImportedCount++
		if saveErr := w.recordScanEntry(ctx, source, result.JobID, relativeName, domain.DataSourceScanOutcomeImported, reason, message, uploadResult.Document.ID, fileInfo.Size(), contentHash); saveErr != nil {
			return saveErr
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if err := w.ensureSourceScanNotCanceled(ctx, source.TenantID, result.JobID); err != nil {
		return result, err
	}
	if result.FailedCount == 0 {
		var reconcileErr error
		result, reconcileErr = w.reconcileDeletedSourceFiles(ctx, source, result, previousImported, seenPaths, skippedDirectoryPrefixes)
		if reconcileErr != nil {
			return result, reconcileErr
		}
	}
	if result.FailedCount > 0 {
		return result, fmt.Errorf("source scan imported %d files, skipped %d files, failed %d files; first failure: %w", result.ImportedCount, result.SkippedCount, result.FailedCount, firstFailure)
	}
	return result, nil
}

func (w SourceScanWorker) activeSourceFileEntries(ctx context.Context, source domain.DataSource) (map[string]domain.DataSourceScanEntry, error) {
	entries, err := w.repos.ListDataSourceScanEntries(ctx, source.TenantID, source.ID, 0)
	if err != nil {
		return nil, err
	}
	active := make(map[string]domain.DataSourceScanEntry)
	latestSeen := map[string]bool{}
	for _, entry := range entries {
		if latestSeen[entry.Path] {
			continue
		}
		latestSeen[entry.Path] = true
		if isActiveSourceFileEntry(entry) {
			active[entry.Path] = entry
		}
	}
	return active, nil
}

func (w SourceScanWorker) reconcileDeletedSourceFiles(ctx context.Context, source domain.DataSource, result SourceScanResult, previous map[string]domain.DataSourceScanEntry, seenPaths map[string]bool, skippedDirectoryPrefixes []string) (SourceScanResult, error) {
	for path, entry := range previous {
		if seenPaths[path] || pathUnderSkippedDirectory(path, skippedDirectoryPrefixes) {
			continue
		}
		message := fmt.Sprintf("source file no longer exists; deleted document %s", entry.DocumentID)
		if _, err := w.documents.DeleteDocument(ctx, app.DeleteDocumentInput{
			TenantID:   source.TenantID,
			DocumentID: entry.DocumentID,
		}); err != nil && !errors.Is(err, store.ErrNotFound) {
			result.FailedCount++
			if saveErr := w.recordScanEntry(ctx, source, result.JobID, path, domain.DataSourceScanOutcomeFailed, "delete_missing_failed", err.Error(), entry.DocumentID, entry.SizeBytes, entry.ContentHash); saveErr != nil {
				return result, saveErr
			}
			return result, fmt.Errorf("%s: delete missing source document %s: %w", path, entry.DocumentID, err)
		}
		result.SkippedCount++
		result.DeletedCount++
		if saveErr := w.recordScanEntry(ctx, source, result.JobID, path, domain.DataSourceScanOutcomeDeleted, "missing", message, entry.DocumentID, entry.SizeBytes, entry.ContentHash); saveErr != nil {
			return result, saveErr
		}
	}
	return result, nil
}

func isActiveSourceFileEntry(entry domain.DataSourceScanEntry) bool {
	if entry.DocumentID == "" || entry.ContentHash == "" {
		return false
	}
	if entry.Outcome == domain.DataSourceScanOutcomeImported {
		return true
	}
	return entry.Outcome == domain.DataSourceScanOutcomeSkipped && entry.Reason == "unchanged"
}

func pathUnderSkippedDirectory(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (w SourceScanWorker) recordScanEntry(ctx context.Context, source domain.DataSource, jobID domain.JobID, path string, outcome domain.DataSourceScanOutcome, reason string, message string, documentID domain.DocumentID, sizeBytes int64, contentHash string) error {
	entry, err := domain.NewDataSourceScanEntry(domain.DataSourceScanEntryCreate{
		TenantID:    source.TenantID,
		JobID:       jobID,
		SourceID:    source.ID,
		Path:        path,
		Outcome:     outcome,
		Reason:      reason,
		Message:     message,
		DocumentID:  documentID,
		SizeBytes:   sizeBytes,
		ContentHash: contentHash,
		Now:         w.clock.Now(),
	})
	if err != nil {
		return err
	}
	return w.repos.SaveDataSourceScanEntry(ctx, entry)
}

func fileContentHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (w SourceScanWorker) markSourceFailed(ctx context.Context, source domain.DataSource, result SourceScanResult) error {
	latest, err := w.repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		return err
	}
	if latest.Status == domain.DataSourceStatusArchived {
		return nil
	}
	if err := latest.FailScan(result.ImportedCount, result.SkippedCount, result.FailedCount, w.clock.Now()); err != nil {
		return err
	}
	return w.repos.SaveDataSource(ctx, latest)
}

func (w SourceScanWorker) markSourceCanceled(ctx context.Context, source domain.DataSource) error {
	latest, err := w.repos.GetDataSource(ctx, source.TenantID, source.ID)
	if err != nil {
		return err
	}
	if latest.Status != domain.DataSourceStatusScanning {
		return nil
	}
	if err := latest.CancelScan(w.clock.Now()); err != nil {
		return err
	}
	return w.repos.SaveDataSource(ctx, latest)
}

func (w SourceScanWorker) ensureSourceScanNotCanceled(ctx context.Context, tenantID domain.TenantID, jobID domain.JobID) error {
	job, err := w.repos.GetJob(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if job.State == domain.JobStateCanceled {
		return ErrSourceScanCanceled
	}
	return nil
}

func (w SourceScanWorker) finishSourceScanJob(job *domain.Job, result *SourceScanResult) error {
	if result.JobID == "" {
		result.JobID = job.ID
	}
	if result.SourceID == "" && job.ResourceID != "" {
		result.SourceID = domain.DataSourceID(job.ResourceID)
	}
	if result.StartedAt.IsZero() {
		result.StartedAt = job.UpdatedAt
	}
	result.FinishedAt = w.clock.Now()
	result.DurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	if result.DurationMS < 0 {
		result.DurationMS = 0
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	job.ResultJSON = string(encoded)
	return nil
}

func sourceRelativePath(root string, path string) string {
	relativeName, err := filepath.Rel(root, path)
	if err != nil {
		relativeName = filepath.Base(path)
	}
	relativeName = filepath.ToSlash(relativeName)
	if relativeName == "." || relativeName == "" {
		return "."
	}
	return relativeName
}

func sourcePatternMatches(patterns []string, relativePath string, directory bool) bool {
	relativePath = strings.Trim(strings.ReplaceAll(relativePath, "\\", "/"), "/")
	if directory && relativePath != "" {
		relativePath += "/"
	}
	for _, pattern := range patterns {
		if sourcePatternMatchesOne(pattern, relativePath, directory) {
			return true
		}
	}
	return false
}

func sourcePatternMatchesOne(pattern string, relativePath string, directory bool) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/")), "/")
	if pattern == "" || relativePath == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		pathWithoutSlash := strings.TrimSuffix(relativePath, "/")
		return pathWithoutSlash == prefix || strings.HasPrefix(pathWithoutSlash, prefix+"/")
	}
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(relativePath, pattern)
	}
	if !strings.Contains(pattern, "/") {
		name := path.Base(strings.TrimSuffix(relativePath, "/"))
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return true
		}
		return pattern == name
	}
	expression := "^" + globPatternToRegex(pattern) + "$"
	if directory {
		expression = "^" + globPatternToRegex(strings.TrimSuffix(pattern, "/")) + "/?$"
	}
	matched, err := regexp.MatchString(expression, strings.TrimSuffix(relativePath, "/"))
	return err == nil && matched
}

func globPatternToRegex(pattern string) string {
	var builder strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					builder.WriteString("(?:.*/)?")
					continue
				}
				builder.WriteString(".*")
				continue
			}
			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	return builder.String()
}

func (p SourceScanPolicy) normalized() SourceScanPolicy {
	if p.MaxFileBytes == 0 {
		p.MaxFileBytes = defaultSourceScanMaxFileBytes
	}
	if p.SkipDirectoryName == nil {
		p.SkipDirectoryName = map[string]bool{}
	}
	normalized := make(map[string]bool, len(p.SkipDirectoryName))
	for name, skip := range p.SkipDirectoryName {
		normalized[strings.ToLower(strings.TrimSpace(name))] = skip
	}
	p.SkipDirectoryName = normalized
	return p
}

func (p SourceScanPolicy) shouldSkipDirectory(name string) bool {
	return p.SkipDirectoryName[strings.ToLower(strings.TrimSpace(name))]
}

func (p SourceScanPolicy) shouldSkipName(name string) bool {
	return p.SkipHiddenNames && strings.HasPrefix(name, ".")
}

func contentTypeForPath(path string) string {
	contentType := strings.TrimSpace(mime.TypeByExtension(filepath.Ext(path)))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}
