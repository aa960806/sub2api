package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// This opt-in acceptance test uses the real request/scheduler/repository path.
// The fixture helper refuses production/remote databases, and each run creates
// and removes its own schema. Credentials are supplied only through the process
// environment; no paid requests run in normal tests or CI.
func TestModelEvaluationLiveGenerationPersistenceAndPublication(t *testing.T) {
	key := os.Getenv("MODEL_EVALUATION_LIVE_KEY")
	endpoint := os.Getenv("MODEL_EVALUATION_LIVE_ENDPOINT")
	model := os.Getenv("MODEL_EVALUATION_LIVE_MODEL")
	if key == "" || endpoint == "" || model == "" {
		t.Skip("explicit live key, endpoint and model required")
	}
	db := modelEvaluationTestDB(t)
	repo := NewModelEvaluationRepository(db)
	ctx := context.Background()
	var groupID int64
	require.NoError(t, db.QueryRow(`INSERT INTO groups(name) VALUES('isolated live acceptance') RETURNING id`).Scan(&groupID))
	svc := service.NewModelEvaluationService(repo, liveEvaluationSettings{db: db}, liveEvaluationGroups{id: groupID}, liveEvaluationEncryptor{})
	defer svc.Stop()
	effort := "ultra" // A stored configuration from the broken release must work.
	task, err := svc.CreateTask(ctx, service.ModelEvaluationTaskInput{Name: "live acceptance", GroupID: groupID, Endpoint: endpoint, APIFormat: "responses", APIKey: key, Model: model, ReasoningEffort: &effort, Enabled: true, IntervalSeconds: 1800, RetentionDays: 1, MaxRecords: 5})
	require.NoError(t, err)
	// Simulate the legacy persisted value without changing real user settings.
	_, err = db.Exec(`UPDATE subnexus_model_evaluation_tasks SET reasoning_effort='ultra' WHERE id=$1`, task.ID)
	require.NoError(t, err)
	require.NoError(t, repo.SetEnabled(ctx, false))
	require.NoError(t, svc.TestTask(ctx, task.ID))
	waitResult := func(wantCount int) *service.ModelEvaluationResult {
		t.Helper()
		deadline := time.Now().Add(12 * time.Minute)
		for time.Now().Before(deadline) {
			items, total, listErr := svc.ListResults(ctx, service.ModelEvaluationListParams{Admin: true, TaskID: task.ID})
			require.NoError(t, listErr)
			if total >= int64(wantCount) && len(items) > 0 {
				result, getErr := svc.GetResult(ctx, items[0].ID, service.ModelEvaluationListParams{Admin: true})
				require.NoError(t, getErr)
				if result.Status != "success" {
					t.Fatalf("live generation failed: %s", strings.ReplaceAll(result.ErrorMessage, key, "[REDACTED]"))
				}
				require.Contains(t, strings.ToLower(result.HTML), "</html>")
				require.NotContains(t, result.HTML, key)
				t.Logf("persisted result=%d is_test=%t duration_ms=%d html_bytes=%d", result.ID, result.IsTest, result.DurationMS, len(result.HTML))
				return result
			}
			time.Sleep(time.Second)
		}
		t.Fatal("live generation did not finish within the acceptance deadline")
		return nil
	}
	private := waitResult(1)
	require.True(t, private.IsTest)
	_, total, err := svc.ListResults(ctx, service.ModelEvaluationListParams{AllowedGroupIDs: []int64{groupID}})
	require.NoError(t, err)
	require.Zero(t, total)
	_, err = svc.SetPublication(ctx, task.ID, true)
	require.NoError(t, err)
	require.NoError(t, repo.SetEnabled(ctx, true))
	visible, err := svc.GetResult(ctx, private.ID, service.ModelEvaluationListParams{AllowedGroupIDs: []int64{groupID}})
	require.NoError(t, err)
	require.Equal(t, private.HTML, visible.HTML)
	require.Empty(t, visible.ErrorMessage)
	_, err = svc.GetResult(ctx, private.ID, service.ModelEvaluationListParams{AllowedGroupIDs: []int64{groupID + 1}})
	require.Error(t, err)
	// RunNow uses the published scheduler path and the same real upstream call.
	require.NoError(t, svc.RunNow(ctx, task.ID))
	public := waitResult(2)
	require.False(t, public.IsTest)
	if out := os.Getenv("MODEL_EVALUATION_LIVE_OUTPUT"); out != "" {
		require.NoError(t, os.MkdirAll(out, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(out, "private-test.html"), []byte(private.HTML), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(out, "scheduled-result.html"), []byte(public.HTML), 0600))
	}
}

type liveEvaluationSettings struct {
	service.SettingRepository
	db *sql.DB
}

func (s liveEvaluationSettings) GetValue(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, key).Scan(&value)
	return value, err
}

type liveEvaluationGroups struct {
	service.GroupRepository
	id int64
}

func (s liveEvaluationGroups) GetByID(context.Context, int64) (*service.Group, error) {
	return &service.Group{ID: s.id, Name: "isolated live acceptance", Status: service.StatusActive}, nil
}

type liveEvaluationEncryptor struct{}

func (liveEvaluationEncryptor) Encrypt(value string) (string, error) {
	return "live-test:" + value, nil
}
func (liveEvaluationEncryptor) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, "live-test:") {
		return "", errors.New("invalid fixture credential")
	}
	return strings.TrimPrefix(value, "live-test:"), nil
}
