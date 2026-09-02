package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const studentRechargeBenefitInterval = 5 * time.Second

// StudentRechargeBenefitScheduler grants isolated student bonuses and
// reconciles refund reversals without participating in payment fulfillment.
type StudentRechargeBenefitScheduler struct {
	studentService *StudentRechargeBenefitService
	stopCh         chan struct{}
	doneCh         chan struct{}
	startOnce      sync.Once
	stopOnce       sync.Once
}

func NewStudentRechargeBenefitScheduler(studentService *StudentRechargeBenefitService) *StudentRechargeBenefitScheduler {
	return &StudentRechargeBenefitScheduler{
		studentService: studentService,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
}

func (s *StudentRechargeBenefitScheduler) Start() {
	if s == nil || s.studentService == nil {
		return
	}
	s.startOnce.Do(func() { go s.loop() })
}

func (s *StudentRechargeBenefitScheduler) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		select {
		case <-s.doneCh:
		case <-time.After(3 * time.Second):
			logger.LegacyPrintf("service.student_recharge_benefit", "[StudentRechargeBenefit] scheduler stop timed out")
		}
	})
}

func (s *StudentRechargeBenefitScheduler) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(studentRechargeBenefitInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			granted, grantErr := s.studentService.ProcessStudentRechargeBenefits(ctx, 50)
			reversed, reverseErr := s.studentService.ProcessStudentRechargeRefundReversals(ctx, 50)
			cancel()
			if grantErr != nil {
				logger.LegacyPrintf("service.student_recharge_benefit", "[StudentRechargeBenefit] grant scan failed: %v", grantErr)
			} else if granted > 0 {
				logger.LegacyPrintf("service.student_recharge_benefit", "[StudentRechargeBenefit] granted bonuses: %d", granted)
			}
			if reverseErr != nil {
				logger.LegacyPrintf("service.student_recharge_benefit", "[StudentRechargeBenefit] refund reversal scan failed: %v", reverseErr)
			} else if reversed > 0 {
				logger.LegacyPrintf("service.student_recharge_benefit", "[StudentRechargeBenefit] reversed bonuses: %d", reversed)
			}
		case <-s.stopCh:
			return
		}
	}
}

func ProvideStudentRechargeBenefitScheduler(studentService *StudentRechargeBenefitService) *StudentRechargeBenefitScheduler {
	svc := NewStudentRechargeBenefitScheduler(studentService)
	svc.Start()
	return svc
}
