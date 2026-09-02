package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

const (
	leaderboardRewardWeeklySpec  = "5 0 * * 1"
	leaderboardRewardMonthlySpec = "10 0 1 * *"
	leaderboardRewardLockTTL     = 30 * time.Minute
	leaderboardRewardRunTimeout  = 10 * time.Minute
)

var leaderboardRewardCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// LeaderboardRewardScheduler settles the previous complete week/month. It is
// intentionally independent from display reads, and its service gate is
// checked before every run.
type LeaderboardRewardScheduler struct {
	service    *LeaderboardService
	cfg        *config.Config
	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string

	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewLeaderboardRewardScheduler(service *LeaderboardService, cfg *config.Config) *LeaderboardRewardScheduler {
	return &LeaderboardRewardScheduler{service: service, cfg: cfg, instanceID: uuid.NewString()}
}

// SetLeaderLock injects the cross-instance lock and advisory-lock fallback.
func (s *LeaderboardRewardScheduler) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache, s.db = lockCache, db
}

func (s *LeaderboardRewardScheduler) Start() {
	if s == nil || s.service == nil {
		return
	}
	s.startOnce.Do(func() {
		loc := timezone.Location()
		if s.cfg != nil && strings.TrimSpace(s.cfg.Timezone) != "" {
			if parsed, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil && parsed != nil {
				loc = parsed
			}
		}
		c := cron.New(cron.WithParser(leaderboardRewardCronParser), cron.WithLocation(loc))
		if _, err := c.AddFunc(leaderboardRewardWeeklySpec, func() { s.run(LeaderboardWindowWeek, time.Now().In(loc)) }); err != nil {
			logger.LegacyPrintf("service.subnexus_leaderboard", "[LeaderboardReward] weekly job not started: %v", err)
			return
		}
		if _, err := c.AddFunc(leaderboardRewardMonthlySpec, func() { s.run(LeaderboardWindowMonth, time.Now().In(loc)) }); err != nil {
			logger.LegacyPrintf("service.subnexus_leaderboard", "[LeaderboardReward] monthly job not started: %v", err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.subnexus_leaderboard", "[LeaderboardReward] started (week=%s, month=%s, tz=%s)", leaderboardRewardWeeklySpec, leaderboardRewardMonthlySpec, loc.String())
	})
}

func (s *LeaderboardRewardScheduler) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron == nil {
			return
		}
		ctx := s.cron.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
			logger.LegacyPrintf("service.subnexus_leaderboard", "[LeaderboardReward] cron stop timed out")
		}
	})
}

// RunOnce executes a completed-period settlement synchronously, which is
// useful for operators and deterministic tests.
func (s *LeaderboardRewardScheduler) RunOnce(ctx context.Context, window string, now time.Time) (int, float64, error) {
	if s == nil || s.service == nil {
		return 0, 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	window = normalizeLeaderboardWindow(window)
	current, _, _, err := resolveLeaderboardWindow(window, now)
	if err != nil {
		return 0, 0, err
	}
	var previousStart time.Time
	if window == LeaderboardWindowMonth {
		previousStart = current.AddDate(0, -1, 0)
	} else if window == LeaderboardWindowWeek {
		previousStart = current.AddDate(0, 0, -7)
	} else {
		return 0, 0, fmt.Errorf("unsupported leaderboard reward window %q", window)
	}
	period := leaderboardPeriodKey(window, previousStart)
	lockKey := fmt.Sprintf("subnexus:leaderboard:reward:%s:%s", window, period)
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	release, ok := tryAcquireSingletonLeaderLock(lockCtx, s.lockCache, s.db, lockKey, s.instanceID, leaderboardRewardLockTTL)
	cancel()
	if !ok {
		return 0, 0, nil
	}
	defer release()
	runCtx, runCancel := context.WithTimeout(ctx, leaderboardRewardRunTimeout)
	defer runCancel()
	return s.service.GrantLeaderboardRewardsForPeriod(runCtx, window, period)
}

func (s *LeaderboardRewardScheduler) run(window string, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), leaderboardRewardRunTimeout+5*time.Second)
	defer cancel()
	granted, total, err := s.RunOnce(ctx, window, now)
	if err != nil {
		if err == ErrLeaderboardDisabled || err == ErrLeaderboardRewardDisabled {
			return
		}
		logger.LegacyPrintf("service.subnexus_leaderboard", "[LeaderboardReward] %s settlement failed: %v", window, err)
		return
	}
	if granted > 0 {
		logger.LegacyPrintf("service.subnexus_leaderboard", "[LeaderboardReward] %s granted=%d total=%.2f", window, granted, total)
	}
}

// ProvideLeaderboardRewardScheduler constructs and starts the scheduler for
// the production Wire graph.
func ProvideLeaderboardRewardScheduler(
	service *LeaderboardService,
	lockCache LeaderLockCache,
	db *sql.DB,
	cfg *config.Config,
) *LeaderboardRewardScheduler {
	scheduler := NewLeaderboardRewardScheduler(service, cfg)
	scheduler.SetLeaderLock(lockCache, db)
	scheduler.Start()
	return scheduler
}
