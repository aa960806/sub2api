package media

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// MediaHoldSweeper recovers durable intents, including tasks abandoned by the
// client. It never refunds merely because a timer or cache key expired.
type MediaHoldSweeper struct {
	handler *MediaTaskHandler
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
}

func NewMediaHoldSweeper(h *MediaTaskHandler) *MediaHoldSweeper { return &MediaHoldSweeper{handler: h} }
func (s *MediaHoldSweeper) Start() {
	if s == nil || s.handler == nil || s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			s.SweepOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func (s *MediaHoldSweeper) Stop() {
	if s == nil || s.cancel == nil {
		return
	}
	s.once.Do(s.cancel)
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
}
func (s *MediaHoldSweeper) SweepOnce(ctx context.Context) (processed, failed int) {
	if s == nil || s.handler == nil || s.handler.tasks == nil {
		return 0, 0
	}
	store, ok := s.handler.tasks.Store().(DurableMediaTaskStore)
	if !ok {
		return 0, 0
	}
	if cleaner, ok := store.(interface {
		CleanupExpiredFiles(context.Context, int) (int64, error)
	}); ok {
		if _, err := cleaner.CleanupExpiredFiles(ctx, 200); err != nil {
			logger.L().Warn("media.reference_cleanup_failed", zap.Error(err))
		}
	}
	tasks, err := store.ListRecoverableTasks(ctx, 20)
	if err != nil {
		logger.L().Warn("media.recovery_list_failed", zap.Error(err))
		return 0, 1
	}
	// Four bounded workers avoid a slow account blocking every other task.
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return processed, failed
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			err := s.handler.advanceTask(ctx, id, false)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
			} else {
				processed++
			}
		}(task.ID)
	}
	wg.Wait()
	return processed, failed
}
