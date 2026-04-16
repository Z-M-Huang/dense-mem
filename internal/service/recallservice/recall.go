// Package recallservice implements hybrid semantic + keyword recall over
// SourceFragments for a single profile.
//
// Merge strategy: Reciprocal Rank Fusion (RRF). For each candidate fragment
// we compute:
//
//	score(id) = Σ_branch 1 / (RRFConstant + rank_in_branch)
//
// RRF is used because it does not require score normalization across branches
// and is robust to the difference in scale between BM25 (keyword) and cosine
// similarity (semantic). Alternative merge strategies (weighted sum, Borda
// count) are explicitly deferred per AC-51.
//
// Embedding failure policy: fail-closed. If the embedding provider errors we
// surface a sanitized error to the caller. We deliberately do NOT fall back
// to keyword-only recall because that would silently degrade result quality
// and make the degradation invisible to callers (AC-40).
//
// The query embedding is used only within Recall and is never persisted
// (AC-40). Branch results are post-filtered by profile_id as defense in
// depth, even though each branch already enforces the filter at the query
// layer. Only SourceFragment-typed hits are kept (AC-39).
package recallservice

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/embedding"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/tools/keywordsearch"
	"github.com/dense-mem/dense-mem/internal/tools/semanticsearch"
)

// Tuning constants.
const (
	// OverfetchMultiplier sets how many times the requested limit each branch
	// fetches before merge. The global vector index is shared across profiles,
	// so we overfetch and post-filter for profile isolation (AC-40).
	OverfetchMultiplier = 10
	// RRFConstant is the k parameter in Reciprocal Rank Fusion.
	RRFConstant = 60
	// DefaultLimit is used when RecallRequest.Limit is zero.
	DefaultLimit = 10
	// MinLimit and MaxLimit bound the effective result count.
	MinLimit = 1
	MaxLimit = 50
)

// ErrEmbeddingUnavailable is returned to callers when the embedding provider
// fails. The underlying provider error is logged (scrubbed) but never
// returned verbatim so provider keys / URLs cannot leak through the API.
var ErrEmbeddingUnavailable = errors.New("recall: embedding provider unavailable")

// ErrKeywordUnavailable is returned when the keyword branch fails.
var ErrKeywordUnavailable = errors.New("recall: keyword search unavailable")

// RecallRequest is the validated input to Recall. Validator tags are used by
// HTTP handlers via the shared BindAndValidate middleware; the service also
// enforces the clamp + non-empty invariants defensively.
type RecallRequest struct {
	Query string `json:"query" validate:"required,max=512"`
	Limit int    `json:"limit" validate:"gte=0,lte=50"`
}

// RecallHit is one merged, hydrated recall result.
type RecallHit struct {
	Fragment     *domain.Fragment `json:"fragment"`
	SemanticRank int              `json:"semantic_rank"` // 1-based; 0 if absent from that branch
	KeywordRank  int              `json:"keyword_rank"`  // 1-based; 0 if absent from that branch
	FinalScore   float64          `json:"final_score"`
}

// RecallService is the external contract consumed by handlers and the tool
// registry.
type RecallService interface {
	Recall(ctx context.Context, profileID string, req RecallRequest) ([]RecallHit, error)
}

// EmbeddingProvider is the narrow slice of embedding.EmbeddingProviderInterface
// used by recall. Restated locally so tests can stub without pulling the full
// provider surface.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, string, error)
}

// SemanticSearcher is the narrow slice of the vector index searcher.
type SemanticSearcher interface {
	QueryVectorIndex(ctx context.Context, profileID string, embedding []float32, limit int) ([]semanticsearch.SearchHit, error)
}

// KeywordSearcher is the narrow slice of the BM25 fragment searcher
// (fragments only — the fact searcher is intentionally NOT invoked, AC-39).
type KeywordSearcher interface {
	SearchContent(ctx context.Context, profileID string, query string, labels []string, limit int) ([]keywordsearch.FragmentSearchResult, error)
}

// FragmentHydrator loads the full domain.Fragment for a winning id.
type FragmentHydrator interface {
	GetByID(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error)
}

// recallService implements RecallService.
type recallService struct {
	embedder EmbeddingProvider
	semantic SemanticSearcher
	keyword  KeywordSearcher
	hydrator FragmentHydrator
	logger   observability.LogProvider
	metrics  observability.DiscoverabilityMetrics
}

var _ RecallService = (*recallService)(nil)

// NewRecallService constructs a RecallService. All dependencies are required
// except logger (may be nil — logging becomes a no-op) and metrics (may be
// nil — a noop recorder is substituted so call sites never need nil checks).
func NewRecallService(
	embedder EmbeddingProvider,
	semantic SemanticSearcher,
	keyword KeywordSearcher,
	hydrator FragmentHydrator,
	logger observability.LogProvider,
	metrics observability.DiscoverabilityMetrics,
) RecallService {
	if metrics == nil {
		metrics = observability.NoopDiscoverabilityMetrics()
	}
	return &recallService{
		embedder: embedder,
		semantic: semantic,
		keyword:  keyword,
		hydrator: hydrator,
		logger:   logger,
		metrics:  metrics,
	}
}

// Recall runs both branches in parallel, merges via RRF, and returns the top
// `limit` hydrated fragments for the caller's profile.
func (s *recallService) Recall(ctx context.Context, profileID string, req RecallRequest) ([]RecallHit, error) {
	start := time.Now()
	defer func() {
		s.metrics.ObserveRecallLatency(float64(time.Since(start).Milliseconds()))
	}()

	if profileID == "" {
		return nil, errors.New("recall: profile id is required")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.New("recall: query is required")
	}

	limit := clampLimit(req.Limit)
	overfetch := limit * OverfetchMultiplier

	var (
		wg      sync.WaitGroup
		semHits []semanticsearch.SearchHit
		semErr  error
		kwHits  []keywordsearch.FragmentSearchResult
		kwErr   error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		vec, _, err := s.embedder.Embed(ctx, query)
		if err != nil {
			semErr = sanitizeEmbeddingError(err)
			return
		}
		// vec is request-scoped: used only for this kNN query and never
		// written to any store (AC-40 explicit).
		hits, err := s.semantic.QueryVectorIndex(ctx, profileID, vec, overfetch)
		if err != nil {
			semErr = fmt.Errorf("recall: semantic branch: %w", err)
			return
		}
		semHits = hits
	}()
	go func() {
		defer wg.Done()
		hits, err := s.keyword.SearchContent(ctx, profileID, query, nil, overfetch)
		if err != nil {
			kwErr = fmt.Errorf("recall: keyword branch: %w", err)
			return
		}
		kwHits = hits
	}()
	wg.Wait()

	if semErr != nil {
		s.logEmbeddingError(semErr)
		return nil, ErrEmbeddingUnavailable
	}
	if kwErr != nil {
		s.logKeywordError(kwErr)
		return nil, ErrKeywordUnavailable
	}

	filteredSem := filterSemanticFragments(semHits, profileID)
	filteredKw := filterKeywordFragments(kwHits, profileID)

	merged := rrfMerge(filteredSem, filteredKw)

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].FinalScore != merged[j].FinalScore {
			return merged[i].FinalScore > merged[j].FinalScore
		}
		return merged[i].id < merged[j].id
	})

	if len(merged) > limit {
		merged = merged[:limit]
	}

	out := make([]RecallHit, 0, len(merged))
	for _, m := range merged {
		frag, err := s.hydrator.GetByID(ctx, profileID, m.id)
		if err != nil {
			// A winning id may vanish due to a concurrent delete; skip it
			// rather than failing the whole recall.
			s.logHydrateError(m.id, err)
			continue
		}
		out = append(out, RecallHit{
			Fragment:     frag,
			SemanticRank: m.SemanticRank,
			KeywordRank:  m.KeywordRank,
			FinalScore:   m.FinalScore,
		})
	}
	return out, nil
}

// rrfEntry is the internal accumulator keyed by fragment id.
type rrfEntry struct {
	id           string
	SemanticRank int
	KeywordRank  int
	FinalScore   float64
}

// rrfMerge computes score(id) = Σ 1 / (RRFConstant + rank) across branches.
// Each branch contributes the 1-based rank of the id within that branch.
func rrfMerge(sem []semanticsearch.SearchHit, kw []keywordsearch.FragmentSearchResult) []rrfEntry {
	byID := make(map[string]*rrfEntry, len(sem)+len(kw))
	for i, h := range sem {
		rank := i + 1
		e, ok := byID[h.ID]
		if !ok {
			e = &rrfEntry{id: h.ID}
			byID[h.ID] = e
		}
		if e.SemanticRank == 0 || rank < e.SemanticRank {
			e.SemanticRank = rank
		}
		e.FinalScore += 1.0 / float64(RRFConstant+rank)
	}
	for i, h := range kw {
		rank := i + 1
		e, ok := byID[h.FragmentID]
		if !ok {
			e = &rrfEntry{id: h.FragmentID}
			byID[h.FragmentID] = e
		}
		if e.KeywordRank == 0 || rank < e.KeywordRank {
			e.KeywordRank = rank
		}
		e.FinalScore += 1.0 / float64(RRFConstant+rank)
	}
	out := make([]rrfEntry, 0, len(byID))
	for _, e := range byID {
		out = append(out, *e)
	}
	return out
}

// filterSemanticFragments drops hits outside the caller's profile and any
// non-fragment hits (defense-in-depth; the searcher already restricts both).
func filterSemanticFragments(hits []semanticsearch.SearchHit, profileID string) []semanticsearch.SearchHit {
	out := make([]semanticsearch.SearchHit, 0, len(hits))
	for _, h := range hits {
		if h.ProfileID != profileID {
			continue
		}
		if h.Type != "" && h.Type != "fragment" {
			continue
		}
		out = append(out, h)
	}
	return out
}

// filterKeywordFragments drops any hit not belonging to the caller's profile.
func filterKeywordFragments(hits []keywordsearch.FragmentSearchResult, profileID string) []keywordsearch.FragmentSearchResult {
	out := make([]keywordsearch.FragmentSearchResult, 0, len(hits))
	for _, h := range hits {
		if h.ProfileID != profileID {
			continue
		}
		out = append(out, h)
	}
	return out
}

// clampLimit enforces the [MinLimit, MaxLimit] bound and defaults zero to
// DefaultLimit.
func clampLimit(req int) int {
	if req <= 0 {
		return DefaultLimit
	}
	if req > MaxLimit {
		return MaxLimit
	}
	if req < MinLimit {
		return MinLimit
	}
	return req
}

// sanitizeEmbeddingError classifies the provider error type but strips any
// message contents so provider internals never surface to callers.
func sanitizeEmbeddingError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, embedding.ErrEmbeddingTimeout):
		return errors.New("recall: embedding timeout")
	case errors.Is(err, embedding.ErrEmbeddingRateLimit):
		return errors.New("recall: embedding rate limited")
	case errors.Is(err, embedding.ErrEmbeddingProvider):
		return errors.New("recall: embedding provider error")
	}
	return errors.New("recall: embedding unavailable")
}

func (s *recallService) logEmbeddingError(err error) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("recall: embedding provider failed", observability.String("error", err.Error()))
}

func (s *recallService) logKeywordError(err error) {
	if s.logger == nil {
		return
	}
	s.logger.Error("recall: keyword branch failed", err)
}

func (s *recallService) logHydrateError(id string, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("recall: hydrate miss",
		observability.String("fragment_id", id),
		observability.String("error", err.Error()),
	)
}
