package repository

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// This optional test uses a freshly created namespace in a dedicated local test
// database. It deliberately refuses remote URLs and ordinary application DBs.
func modelEvaluationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SUB2API_MODEL_EVALUATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set SUB2API_MODEL_EVALUATION_TEST_DSN to an isolated local PostgreSQL test database")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, u.Hostname())
	require.True(t, strings.HasPrefix(u.Path, "/model_evaluation_test_"), "test database name must start with model_evaluation_test_")
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := "model_eval_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = base.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(); _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); _ = base.Close() })
	_, err = db.Exec(`CREATE TABLE groups(id BIGSERIAL PRIMARY KEY,name VARCHAR(100) NOT NULL,status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ); CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`)
	require.NoError(t, err)
	migration, err := dbmigrations.FS.ReadFile("9014_subnexus_model_evaluations.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	// Replay is harmless and must not overwrite an administrator's enabled flag.
	_, err = db.Exec(`UPDATE settings SET value='true'`)
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	var value string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings`).Scan(&value))
	require.Equal(t, "true", value)
	return db
}

func TestModelEvaluationPostgresLeaseVisibilityRetentionAndFencing(t *testing.T) {
	db := modelEvaluationTestDB(t)
	repo := NewModelEvaluationRepository(db)
	ctx := context.Background()
	var groupID, otherID int64
	require.NoError(t, db.QueryRow(`INSERT INTO groups(name) VALUES('allowed') RETURNING id`).Scan(&groupID))
	require.NoError(t, db.QueryRow(`INSERT INTO groups(name) VALUES('other') RETURNING id`).Scan(&otherID))
	newTask := func(group int64) *service.ModelEvaluationTask {
		task := &service.ModelEvaluationTask{Name: "task", GroupID: group, GroupName: "allowed", Endpoint: "https://example.test/v1/chat/completions", APIFormat: "chat_completions", APIKeyEncrypted: "encrypted", Model: "configured-model", Enabled: true, IntervalSeconds: 60, RetentionDays: 7, MaxRecords: 2}
		require.NoError(t, repo.CreateTask(ctx, task))
		return task
	}
	task := newTask(groupID)
	other := newTask(otherID)
	third := newTask(groupID)
	// Fresh schedules wait their interval; no catch-up replay or immediate startup bill.
	claimed, err := repo.Claim(ctx, task.ID, false)
	require.NoError(t, err)
	require.Nil(t, claimed)
	require.NoError(t, repo.SetEnabled(ctx, false))
	claimed, err = repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.Nil(t, claimed)
	require.NoError(t, repo.SetEnabled(ctx, true))
	// Concurrent replicas can claim a given task only once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var claims []*service.ModelEvaluationTask
	var claimErrors []error
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := repo.Claim(ctx, task.ID, true)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				claimErrors = append(claimErrors, e)
			}
			if v != nil {
				claims = append(claims, v)
			}
		}()
	}
	wg.Wait()
	require.Empty(t, claimErrors)
	require.Len(t, claims, 1)
	lease := claims[0]
	second, err := repo.Claim(ctx, other.ID, true)
	require.NoError(t, err)
	require.NotNil(t, second)
	blocked, err := repo.Claim(ctx, third.ID, true)
	require.NoError(t, err)
	require.Nil(t, blocked)
	// Removing a running task keeps its independent capacity slot until cancellation.
	require.NoError(t, repo.DeleteTask(ctx, other.ID))
	blocked, err = repo.Claim(ctx, third.ID, true)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.NoError(t, repo.Release(ctx, second))
	complete := func(l *service.ModelEvaluationTask, status string) {
		r := &service.ModelEvaluationResult{Status: status, DurationMS: 42}
		if status == "success" {
			r.HTML = "<!doctype html><html><body>original</body></html>"
		} else {
			r.ErrorMessage = "safe failure"
		}
		saved, e := repo.Complete(ctx, l, r)
		require.NoError(t, e)
		require.True(t, saved)
	}
	complete(lease, "success")
	for _, status := range []string{"error", "success"} {
		l, e := repo.Claim(ctx, task.ID, true)
		require.NoError(t, e)
		require.NotNil(t, l)
		complete(l, status)
	}
	items, total, err := repo.ListResults(ctx, service.ModelEvaluationListParams{Admin: true, Page: 1, PageSize: 20, TaskID: task.ID})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, items, 2)
	require.Empty(t, items[0].HTML)
	detail, err := repo.GetResult(ctx, items[0].ID, service.ModelEvaluationListParams{AllowedGroupIDs: []int64{groupID}})
	require.NoError(t, err)
	require.Contains(t, detail.HTML, "original")
	_, err = repo.GetResult(ctx, items[0].ID, service.ModelEvaluationListParams{AllowedGroupIDs: []int64{otherID}})
	require.ErrorIs(t, err, service.ErrModelEvaluationNotFound)
	_, err = repo.GetResult(ctx, items[0].ID, service.ModelEvaluationListParams{})
	require.ErrorIs(t, err, service.ErrModelEvaluationNotFound)
	groups, err := repo.ListGroups(ctx, []int64{groupID, otherID})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, groupID, groups[0].ID)
	// Disabling while an upstream request is active invalidates every old completion.
	l, err := repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.NotNil(t, l)
	require.NoError(t, repo.SetEnabled(ctx, false))
	require.NoError(t, repo.SetEnabled(ctx, true))
	saved, err := repo.Complete(ctx, l, &service.ModelEvaluationResult{Status: "error", ErrorMessage: "late"})
	require.NoError(t, err)
	require.False(t, saved)
	require.NoError(t, repo.Release(ctx, l))
	// Updating a task also fences the active generation, even if re-enabled.
	l, err = repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.NotNil(t, l)
	current, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	current.Model = "new-model"
	require.NoError(t, repo.UpdateTask(ctx, current))
	saved, err = repo.Complete(ctx, l, &service.ModelEvaluationResult{Status: "error", ErrorMessage: "old model"})
	require.NoError(t, err)
	require.False(t, saved)
	require.NoError(t, repo.Release(ctx, l))
	// Explicit cleanup removes data and fences the generation already in progress.
	l, err = repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.NotNil(t, l)
	deleted, err := repo.Cleanup(ctx, service.ModelEvaluationCleanupParams{TaskID: task.ID, All: true})
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	saved, err = repo.Complete(ctx, l, &service.ModelEvaluationResult{Status: "error", ErrorMessage: "late"})
	require.NoError(t, err)
	require.False(t, saved)
	require.NoError(t, repo.Release(ctx, l))
	l, err = repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.NotNil(t, l)
	complete(l, "error")
	_, err = db.Exec(`UPDATE subnexus_model_evaluation_results SET created_at=$1 WHERE task_id=$2`, time.Now().Add(-8*24*time.Hour), task.ID)
	require.NoError(t, err)
	deleted, err = repo.Cleanup(ctx, service.ModelEvaluationCleanupParams{})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	// Group suspension hides existing history and prevents new requests.
	_, err = db.Exec(`UPDATE groups SET status='inactive' WHERE id=$1`, groupID)
	require.NoError(t, err)
	l, err = repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.Nil(t, l)
	groups, err = repo.ListGroups(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Empty(t, groups)
	_, err = db.Exec(`UPDATE groups SET status='active' WHERE id=$1`, groupID)
	require.NoError(t, err)
	oldLease, err := repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.NotNil(t, oldLease)
	_, err = db.Exec(`UPDATE subnexus_model_evaluation_tasks SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, task.ID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE subnexus_model_evaluation_slots SET lease_until=NOW()-INTERVAL '1 second' WHERE lease_token=$1`, oldLease.LeaseToken)
	require.NoError(t, err)
	newLease, err := repo.Claim(ctx, task.ID, true)
	require.NoError(t, err)
	require.NotNil(t, newLease)
	require.NotEqual(t, oldLease.LeaseToken, newLease.LeaseToken)
	saved, err = repo.Complete(ctx, oldLease, &service.ModelEvaluationResult{Status: "error", ErrorMessage: "expired"})
	require.NoError(t, err)
	require.False(t, saved)
	require.NoError(t, repo.Release(ctx, oldLease))
	valid, err := repo.LeaseCurrent(ctx, newLease)
	require.NoError(t, err)
	require.True(t, valid)
	require.NoError(t, repo.Release(ctx, newLease))
	// The global hard limit is enforced under the same transaction lock as creates.
	_, err = db.Exec(`INSERT INTO subnexus_model_evaluation_tasks(name,group_id,endpoint,api_key_encrypted,model) SELECT 'capacity',$1,'https://example.test/v1/chat/completions','encrypted','model' FROM generate_series(1,$2)`, groupID, service.ModelEvaluationMaxTasks-2)
	require.NoError(t, err)
	require.ErrorIs(t, repo.CreateTask(ctx, &service.ModelEvaluationTask{}), service.ErrModelEvaluationLimit)
	// Deleted groups remain administratively visible so their tasks can be removed
	// or reassigned rather than permanently consuming the bounded task allowance.
	_, err = db.Exec(`UPDATE groups SET deleted_at=NOW() WHERE id=$1`, groupID)
	require.NoError(t, err)
	adminTasks, err := repo.ListTasks(ctx)
	require.NoError(t, err)
	require.Len(t, adminTasks, service.ModelEvaluationMaxTasks)
	_, err = repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	groups, err = repo.ListGroups(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Empty(t, groups)
}
