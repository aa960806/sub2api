package media

import (
	"context"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrMediaIdempotencyConflict = infraerrors.New(http.StatusConflict, "MEDIA_IDEMPOTENCY_CONFLICT", "idempotency key was already used for a different media request")
	ErrMediaLeaseLost           = infraerrors.New(http.StatusServiceUnavailable, "MEDIA_LEASE_LOST", "media task is being processed by another worker")
	ErrMediaKeyQuotaExceeded    = infraerrors.New(http.StatusPaymentRequired, "MEDIA_KEY_QUOTA_EXCEEDED", "API key quota is insufficient for this media task and its pending tasks")
	ErrMediaKeyRateLimited      = infraerrors.New(http.StatusTooManyRequests, "MEDIA_KEY_RATE_LIMITED", "API key rate limit is insufficient for this media task and its pending tasks")
	ErrMediaKeyUnavailable      = infraerrors.New(http.StatusForbidden, "MEDIA_KEY_UNAVAILABLE", "API key is disabled or expired")
)

// Kept separate so legacy callers and lightweight test stores remain compatible.
type DurableMediaTaskStore interface {
	MediaTaskStore
	CreateTask(ctx context.Context, task *MediaTaskRecord) (*MediaTaskRecord, bool, error)
	AcquireTask(ctx context.Context, id, token string, ttl time.Duration) (bool, error)
	ReleaseTask(ctx context.Context, id, token string) error
	ListRecoverableTasks(ctx context.Context, limit int) ([]*MediaTaskRecord, error)
}

type mediaLeaseContextKey struct{}

// WithMediaLeaseContext fences task writes against a lost or expired lease.
// The token must be unique and have been acquired by AcquireTask successfully.
func WithMediaLeaseContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, mediaLeaseContextKey{}, token)
}
