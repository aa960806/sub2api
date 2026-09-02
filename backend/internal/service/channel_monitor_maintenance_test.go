//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// maintenanceRepoProbe embeds the full repository contract while overriding
// every method RunDailyMaintenance could call. Any invocation increments the
// counter, making the disabled/no-settings assertions independent of SQL.
type maintenanceRepoProbe struct {
	ChannelMonitorRepository
	calls atomic.Int64
}

func (r *maintenanceRepoProbe) LoadAggregationWatermark(context.Context) (*time.Time, error) {
	r.calls.Add(1)
	return nil, nil
}

func (r *maintenanceRepoProbe) UpsertDailyRollupsFor(context.Context, time.Time) (int64, error) {
	r.calls.Add(1)
	return 0, nil
}

func (r *maintenanceRepoProbe) UpdateAggregationWatermark(context.Context, time.Time) error {
	r.calls.Add(1)
	return nil
}

func (r *maintenanceRepoProbe) DeleteHistoryBefore(context.Context, time.Time) (int64, error) {
	r.calls.Add(1)
	return 0, nil
}

func (r *maintenanceRepoProbe) DeleteRollupsBefore(context.Context, time.Time) (int64, error) {
	r.calls.Add(1)
	return 0, nil
}

type maintenanceRuntimeReader struct {
	enabled bool
}

func (r maintenanceRuntimeReader) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return ChannelMonitorRuntime{Enabled: r.enabled, Mode: ChannelMonitorModeV1}
}

func TestRunDailyMaintenance_DisabledDoesNotTouchRepository(t *testing.T) {
	repo := &maintenanceRepoProbe{}
	svc := NewChannelMonitorService(repo, nil)
	svc.SetRuntimeReader(maintenanceRuntimeReader{enabled: false})

	require.NoError(t, svc.RunDailyMaintenance(context.Background()))
	require.Zero(t, repo.calls.Load())
}

func TestRunDailyMaintenance_NilSettingsFailsClosed(t *testing.T) {
	repo := &maintenanceRepoProbe{}
	svc := NewChannelMonitorService(repo, nil)

	require.NoError(t, svc.RunDailyMaintenance(context.Background()))
	require.Zero(t, repo.calls.Load())
}
