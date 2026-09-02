package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	affiliateSignupRewardScanInterval = 30 * time.Second
	affiliateSignupRewardScanLimit    = 50
)

// AffiliateSignupRewardScanner retries registration rewards that could not be
// written during signup.  It is deliberately a separate, bounded worker: the
// feature gate is checked by the service and again by the repository's
// transaction before any balance mutation.
type AffiliateSignupRewardScanner struct {
	service   *AffiliateService
	stopCh    chan struct{}
	doneCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewAffiliateSignupRewardScanner(service *AffiliateService) *AffiliateSignupRewardScanner {
	return &AffiliateSignupRewardScanner{
		service: service,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// ProvideAffiliateSignupRewardScanner constructs and starts the optional
// reconciler.  A nil/partial service is accepted so old test graphs remain
// valid and simply produce an inert scanner.
func ProvideAffiliateSignupRewardScanner(service *AffiliateService) *AffiliateSignupRewardScanner {
	scanner := NewAffiliateSignupRewardScanner(service)
	scanner.Start()
	return scanner
}

func (s *AffiliateSignupRewardScanner) Start() {
	if s == nil || s.service == nil {
		return
	}
	s.startOnce.Do(func() { go s.loop() })
}

func (s *AffiliateSignupRewardScanner) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		select {
		case <-s.doneCh:
		case <-time.After(3 * time.Second):
			logger.LegacyPrintf("service.affiliate", "[Affiliate] signup reward scanner stop timed out")
		}
	})
}

func (s *AffiliateSignupRewardScanner) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(affiliateSignupRewardScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			result, err := s.service.ReconcileAffiliateSignupRewardsOnce(ctx, affiliateSignupRewardScanLimit)
			cancel()
			if err != nil {
				logger.LegacyPrintf("service.affiliate", "[Affiliate] signup reward reconciliation failed: %v", err)
				continue
			}
			if result.Examined > 0 {
				logger.LegacyPrintf("service.affiliate", "[Affiliate] signup reward reconciliation: examined=%d applied=%d skipped=%d retried=%d", result.Examined, result.Applied, result.Skipped, result.Retried)
			}
		case <-s.stopCh:
			return
		}
	}
}
