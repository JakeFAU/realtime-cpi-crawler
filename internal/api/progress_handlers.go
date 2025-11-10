package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

const (
	defaultJobLimit   = 50
	maxJobLimit       = 500
	defaultSitesLimit = 100
	maxSitesLimit     = 1000
	progressTimeout   = 3 * time.Second
)

// ProgressHandler exposes read-only job progress endpoints.
type ProgressHandler struct {
	repo    store.ProgressRepository
	timeout time.Duration
	logger  *zap.Logger
}

// NewProgressHandler wires the repository and logger.
func NewProgressHandler(repo store.ProgressRepository, logger *zap.Logger) *ProgressHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ProgressHandler{
		repo:    repo,
		timeout: progressTimeout,
		logger:  logger,
	}
}

// ListJobs handles GET /api/jobs?status=&limit=&offset=. It returns a JSON
// object {"jobs": [...]} on success, 400 for invalid filters, 503 when the repo
// is unavailable, or 500 if the repository call fails.
func (h *ProgressHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "progress repository unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	limit, offset, err := parseLimitOffset(r, defaultJobLimit, maxJobLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))
	var status *store.JobRunStatus
	if statusParam != "" {
		statusVal, parseErr := parseStatus(statusParam)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, parseErr.Error())
			return
		}
		status = &statusVal
	}
	jobs, err := h.repo.ListJobs(ctx, status, limit, offset)
	if err != nil {
		h.logger.Error("list jobs failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs": toJobDTOs(jobs),
	})
}

// GetJob handles GET /api/jobs/{job_id}. It returns {"job": {...}} on success,
// 400 for malformed IDs, 404 when the repository reports store.ErrNotFound,
// 503 if the repo is not initialized, or 500 otherwise.
func (h *ProgressHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "progress repository unavailable")
		return
	}
	jobID, err := parseJobID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	job, err := h.repo.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		h.logger.Error("get job failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to load job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": toJobDTO(job)})
}

// ListJobSites handles GET /api/jobs/{job_id}/sites?limit=&offset=. It returns
// {"sites": [...]} on success, 400 for invalid query parameters, 503 when the
// repository is missing, or 500 for repository errors.
func (h *ProgressHandler) ListJobSites(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "progress repository unavailable")
		return
	}
	jobID, err := parseJobID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, offset, err := parseLimitOffset(r, defaultSitesLimit, maxSitesLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	sites, err := h.repo.ListJobSites(ctx, jobID, limit, offset)
	if err != nil {
		h.logger.Error("list job sites failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list job sites")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sites": toSiteDTOs(sites),
	})
}

func parseJobID(r *http.Request) (uuid.UUID, error) {
	jobIDStr := chi.URLParam(r, "job_id")
	if jobIDStr == "" {
		return uuid.UUID{}, errors.New("job_id is required")
	}
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		return uuid.UUID{}, errors.New("invalid job_id")
	}
	return jobID, nil
}

func parseLimitOffset(r *http.Request, def, maxLimit int) (int, int, error) {
	q := r.URL.Query()
	limit := def
	if limStr := q.Get("limit"); limStr != "" {
		val, err := strconv.Atoi(limStr)
		if err != nil || val <= 0 {
			return 0, 0, errors.New("invalid limit")
		}
		if val > maxLimit {
			val = maxLimit
		}
		limit = val
	}
	offset := 0
	if offStr := q.Get("offset"); offStr != "" {
		val, err := strconv.Atoi(offStr)
		if err != nil || val < 0 {
			return 0, 0, errors.New("invalid offset")
		}
		offset = val
	}
	return limit, offset, nil
}

func parseStatus(input string) (store.JobRunStatus, error) {
	switch strings.ToLower(input) {
	case "", "running":
		return store.RunRunning, nil
	case "success":
		return store.RunSuccess, nil
	case "error", "failed", "failure":
		return store.RunError, nil
	default:
		return "", errors.New("invalid status")
	}
}

func toJobDTOs(in []store.JobRun) []jobDTO {
	out := make([]jobDTO, 0, len(in))
	for _, job := range in {
		out = append(out, toJobDTO(job))
	}
	return out
}

func toJobDTO(job store.JobRun) jobDTO {
	dto := jobDTO{
		ID:        job.ID.String(),
		JobID:     job.JobID.String(),
		StartedAt: job.StartedAt,
		Status:    string(job.Status),
		Error:     job.ErrorMessage,
	}
	if job.FinishedAt != nil {
		dto.FinishedAt = job.FinishedAt
	}
	return dto
}

func toSiteDTOs(in []store.SiteStats) []siteDTO {
	out := make([]siteDTO, 0, len(in))
	for _, s := range in {
		out = append(out, siteDTO{
			Site:       s.Site,
			LastUpdate: s.LastUpdate,
			Visits:     s.Visits,
			BytesTotal: s.BytesTotal,
			Fetch2xx:   s.Fetch2xx,
			Fetch3xx:   s.Fetch3xx,
			Fetch4xx:   s.Fetch4xx,
			Fetch5xx:   s.Fetch5xx,
		})
	}
	return out
}

type jobDTO struct {
	ID         string     `json:"id"`
	JobID      string     `json:"job_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     string     `json:"status"`
	Error      *string    `json:"error,omitempty"`
}

type siteDTO struct {
	Site       string    `json:"site"`
	LastUpdate time.Time `json:"last_update"`
	Visits     int64     `json:"visits"`
	BytesTotal int64     `json:"bytes_total"`
	Fetch2xx   int64     `json:"fetch_2xx"`
	Fetch3xx   int64     `json:"fetch_3xx"`
	Fetch4xx   int64     `json:"fetch_4xx"`
	Fetch5xx   int64     `json:"fetch_5xx"`
}
