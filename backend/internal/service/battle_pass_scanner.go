package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type BattlePassScanner struct {
	svc       *BattlePassService
	stopCh    chan struct{}
	doneCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewBattlePassScanner(svc *BattlePassService) *BattlePassScanner {
	return &BattlePassScanner{
		svc:    svc,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func ProvideBattlePassScanner(svc *BattlePassService) *BattlePassScanner {
	scanner := NewBattlePassScanner(svc)
	scanner.Start()
	return scanner
}

func (s *BattlePassScanner) Start() {
	if s == nil || s.svc == nil {
		return
	}
	s.startOnce.Do(func() {
		go s.loop()
	})
}

func (s *BattlePassScanner) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		select {
		case <-s.doneCh:
		case <-time.After(3 * time.Second):
			logger.LegacyPrintf("service.battle_pass", "[BattlePass] scanner stop timed out")
		}
	})
}

func (s *BattlePassScanner) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(battlePassScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := s.runOnce(ctx, time.Now())
			cancel()
			if err != nil {
				logger.LegacyPrintf("service.battle_pass", "[BattlePass] scan failed: %v", err)
			}
		case <-s.stopCh:
			return
		}
	}
}

func (s *BattlePassScanner) runOnce(ctx context.Context, now time.Time) error {
	if s == nil || s.svc == nil {
		return nil
	}
	if _, err := s.svc.ScanUsageOnce(ctx, now); err != nil {
		return err
	}
	if err := s.svc.ReconcilePremiumRewardGrantsOnce(ctx, now); err != nil {
		return err
	}
	_, err := s.svc.ProcessRewardGrantsOnce(ctx, now)
	return err
}
