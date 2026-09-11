package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type modelEvaluationRepository struct{ db *sql.DB }

func NewModelEvaluationRepository(db *sql.DB) service.ModelEvaluationRepository {
	return &modelEvaluationRepository{db: db}
}

// This lock serializes short configuration/claim/finalization transactions across
// replicas. HTTP never runs under a database lock or transaction.
func (r *modelEvaluationRepository) transaction(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1296389708)"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

const modelEvaluationEnabledSQL = `EXISTS (SELECT 1 FROM settings WHERE key='subnexus_model_evaluation_enabled' AND value='true')`
const modelEvaluationTaskColumns = `t.id,t.name,t.group_id,g.name,t.endpoint,t.api_format,t.api_key_encrypted,t.model,t.reasoning_effort,t.enabled,t.interval_seconds,t.retention_days,t.max_records,t.next_run_at,t.created_at,t.updated_at,t.revision,t.lease_token,t.published,
CASE WHEN t.test_status='running' AND (t.lease_until IS NULL OR t.lease_until<=NOW()) THEN 'failed' ELSE t.test_status END,t.last_tested_at,
CASE WHEN t.test_status='running' AND (t.lease_until IS NULL OR t.lease_until<=NOW()) THEN '测试已中断，请重新测试' ELSE t.test_error END,t.configuration_revision,t.lease_is_test`

const modelEvaluationLeaseRuntimeSQL = `(t.lease_is_test OR (t.published AND t.enabled AND ` + modelEvaluationEnabledSQL + `))`

type modelEvaluationScanner interface{ Scan(...any) error }

func scanModelEvaluationTask(row modelEvaluationScanner) (*service.ModelEvaluationTask, error) {
	t := &service.ModelEvaluationTask{}
	err := row.Scan(&t.ID, &t.Name, &t.GroupID, &t.GroupName, &t.Endpoint, &t.APIFormat, &t.APIKeyEncrypted, &t.Model, &t.ReasoningEffort, &t.Enabled, &t.IntervalSeconds, &t.RetentionDays, &t.MaxRecords, &t.NextRunAt, &t.CreatedAt, &t.UpdatedAt, &t.Revision, &t.LeaseToken, &t.Published, &t.TestStatus, &t.LastTestedAt, &t.TestError, &t.ConfigurationRevision, &t.LeaseIsTest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrModelEvaluationNotFound
	}
	if err != nil {
		return nil, err
	}
	t.HasAPIKey = t.APIKeyEncrypted != ""
	return t, nil
}

func (r *modelEvaluationRepository) SetEnabled(ctx context.Context, enabled bool) error {
	tx, err := r.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES ('subnexus_model_evaluation_enabled',$1,NOW()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=NOW()`, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	if !enabled {
		if _, err = tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET revision=revision+1 WHERE lease_token<>'' AND NOT lease_is_test`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *modelEvaluationRepository) ListTasks(ctx context.Context) ([]*service.ModelEvaluationTask, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+modelEvaluationTaskColumns+` FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id ORDER BY t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*service.ModelEvaluationTask, 0)
	for rows.Next() {
		t, err := scanModelEvaluationTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *modelEvaluationRepository) GetTask(ctx context.Context, id int64) (*service.ModelEvaluationTask, error) {
	return scanModelEvaluationTask(r.db.QueryRowContext(ctx, `SELECT `+modelEvaluationTaskColumns+` FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id WHERE t.id=$1`, id))
}

func (r *modelEvaluationRepository) CreateTask(ctx context.Context, t *service.ModelEvaluationTask) error {
	tx, err := r.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subnexus_model_evaluation_tasks`).Scan(&count); err != nil {
		return err
	}
	if count >= service.ModelEvaluationMaxTasks {
		return service.ErrModelEvaluationLimit
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO subnexus_model_evaluation_tasks(name,group_id,endpoint,api_format,api_key_encrypted,model,reasoning_effort,enabled,interval_seconds,retention_days,max_records,next_run_at)
SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9::integer,$10,$11,NOW()+$9::integer*INTERVAL '1 second' WHERE EXISTS(SELECT 1 FROM groups WHERE id=$2 AND status='active' AND deleted_at IS NULL)
RETURNING id,revision,next_run_at,created_at,updated_at,published,test_status,configuration_revision`, t.Name, t.GroupID, t.Endpoint, t.APIFormat, t.APIKeyEncrypted, t.Model, t.ReasoningEffort, t.Enabled, t.IntervalSeconds, t.RetentionDays, t.MaxRecords).Scan(&t.ID, &t.Revision, &t.NextRunAt, &t.CreatedAt, &t.UpdatedAt, &t.Published, &t.TestStatus, &t.ConfigurationRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrModelEvaluationInvalid
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *modelEvaluationRepository) UpdateTask(ctx context.Context, t *service.ModelEvaluationTask) error {
	tx, err := r.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := scanModelEvaluationTask(tx.QueryRowContext(ctx, `SELECT `+modelEvaluationTaskColumns+` FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id WHERE t.id=$1 FOR UPDATE OF t`, t.ID))
	if err != nil {
		return err
	}
	material := t.GroupID != old.GroupID || t.Endpoint != old.Endpoint || t.APIFormat != old.APIFormat || t.APIKeyEncrypted != old.APIKeyEncrypted || t.Model != old.Model || t.ReasoningEffort != old.ReasoningEffort
	reset := material || old.TestStatus == "running" || (old.LeaseIsTest && old.LeaseToken != "")
	err = tx.QueryRowContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET name=$2,group_id=$3,endpoint=$4,api_format=$5,api_key_encrypted=$6,model=$7,reasoning_effort=$8,enabled=$9,interval_seconds=$10::integer,retention_days=$11,max_records=$12,revision=revision+1,next_run_at=NOW()+$10::integer*INTERVAL '1 second',updated_at=NOW()
,configuration_revision=configuration_revision+CASE WHEN $14 THEN 1 ELSE 0 END,published=CASE WHEN $15 THEN FALSE ELSE published END,test_status=CASE WHEN $15 THEN 'untested' ELSE test_status END,last_tested_at=CASE WHEN $15 THEN NULL ELSE last_tested_at END,test_error=CASE WHEN $15 THEN '' ELSE test_error END
WHERE id=$1 AND revision=$13 AND EXISTS(SELECT 1 FROM groups WHERE id=$3 AND status='active' AND deleted_at IS NULL) RETURNING revision,next_run_at,updated_at,published,test_status,last_tested_at,test_error,configuration_revision`, t.ID, t.Name, t.GroupID, t.Endpoint, t.APIFormat, t.APIKeyEncrypted, t.Model, t.ReasoningEffort, t.Enabled, t.IntervalSeconds, t.RetentionDays, t.MaxRecords, t.Revision, material, reset).Scan(&t.Revision, &t.NextRunAt, &t.UpdatedAt, &t.Published, &t.TestStatus, &t.LastTestedAt, &t.TestError, &t.ConfigurationRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrModelEvaluationBusy
	}
	if err != nil {
		return err
	}
	if _, err = cleanupModelEvaluationTx(ctx, tx, service.ModelEvaluationCleanupParams{TaskID: t.ID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *modelEvaluationRepository) DeleteTask(ctx context.Context, id int64) error {
	tx, err := r.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM subnexus_model_evaluation_tasks WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrModelEvaluationNotFound
	}
	return tx.Commit()
}

func (r *modelEvaluationRepository) SetPublication(ctx context.Context, id int64, published bool) (*service.ModelEvaluationTask, error) {
	tx, err := r.transaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	task, err := scanModelEvaluationTask(tx.QueryRowContext(ctx, `SELECT `+modelEvaluationTaskColumns+` FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id WHERE t.id=$1 FOR UPDATE OF t`, id))
	if err != nil {
		return nil, err
	}
	if published && task.TestStatus != "passed" {
		return nil, service.ErrModelEvaluationTestRequired
	}
	_, err = tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET published=$2,revision=revision+1,updated_at=NOW(),next_run_at=NOW()+interval_seconds*INTERVAL '1 second' WHERE id=$1`, id, published)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetTask(ctx, id)
}

func (r *modelEvaluationRepository) ListGroups(ctx context.Context, allowed []int64) ([]service.ModelEvaluationGroup, error) {
	items := make([]service.ModelEvaluationGroup, 0)
	if len(allowed) == 0 {
		return items, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT g.id,g.name FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id WHERE t.enabled AND t.published AND g.status='active' AND g.deleted_at IS NULL AND g.id=ANY($1) AND `+modelEvaluationEnabledSQL+` ORDER BY g.name,g.id`, pq.Array(allowed))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var g service.ModelEvaluationGroup
		if err = rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		items = append(items, g)
	}
	return items, rows.Err()
}

const modelEvaluationResultColumns = `r.id,r.task_id,r.group_id,r.group_name,r.task_name,r.model,r.status,r.duration_ms,r.error_message,r.created_at,r.is_test`

func modelEvaluationResultFilter(p service.ModelEvaluationListParams) (string, []any) {
	filter := ` FROM subnexus_model_evaluation_results r JOIN subnexus_model_evaluation_tasks t ON t.id=r.task_id JOIN groups g ON g.id=r.group_id WHERE ($1::bigint=0 OR r.group_id=$1) AND ($2::bigint=0 OR r.task_id=$2)`
	args := []any{p.GroupID, p.TaskID}
	if !p.Admin {
		filter += ` AND t.enabled AND t.published AND t.group_id=r.group_id AND t.configuration_revision=r.configuration_revision AND (NOT r.is_test OR r.status='success') AND g.status='active' AND g.deleted_at IS NULL AND r.group_id=ANY($3) AND ` + modelEvaluationEnabledSQL
		args = append(args, pq.Array(p.AllowedGroupIDs))
	}
	return filter, args
}

func scanModelEvaluationResult(row modelEvaluationScanner, withHTML bool) (*service.ModelEvaluationResult, error) {
	r := &service.ModelEvaluationResult{}
	dest := []any{&r.ID, &r.TaskID, &r.GroupID, &r.GroupName, &r.TaskName, &r.Model, &r.Status, &r.DurationMS, &r.ErrorMessage, &r.CreatedAt, &r.IsTest}
	if withHTML {
		dest = append(dest, &r.HTML)
	}
	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrModelEvaluationNotFound
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *modelEvaluationRepository) ListResults(ctx context.Context, p service.ModelEvaluationListParams) ([]*service.ModelEvaluationResult, int64, error) {
	filter, args := modelEvaluationResultFilter(p)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+filter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitPosition := len(args) + 1
	args = append(args, p.PageSize, (p.Page-1)*p.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+modelEvaluationResultColumns+filter+fmt.Sprintf(` ORDER BY r.created_at DESC,r.id DESC LIMIT $%d OFFSET $%d`, limitPosition, limitPosition+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*service.ModelEvaluationResult, 0)
	for rows.Next() {
		item, err := scanModelEvaluationResult(rows, false)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *modelEvaluationRepository) GetResult(ctx context.Context, id int64, p service.ModelEvaluationListParams) (*service.ModelEvaluationResult, error) {
	filter, args := modelEvaluationResultFilter(p)
	args = append(args, id)
	return scanModelEvaluationResult(r.db.QueryRowContext(ctx, `SELECT `+modelEvaluationResultColumns+`,r.html`+filter+fmt.Sprintf(` AND r.id=$%d`, len(args)), args...), true)
}

func (r *modelEvaluationRepository) DeleteResult(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM subnexus_model_evaluation_results WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrModelEvaluationNotFound
	}
	return nil
}

func cleanupModelEvaluationTx(ctx context.Context, tx *sql.Tx, p service.ModelEvaluationCleanupParams) (int64, error) {
	if p.All {
		// Fence the in-flight generation, otherwise clearing records could immediately
		// be followed by a stale result from the request that was already running.
		if _, err := tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET revision=revision+1,test_status=CASE WHEN test_status='running' THEN 'untested' ELSE test_status END,test_error=CASE WHEN test_status='running' THEN '' ELSE test_error END WHERE ($1::bigint=0 OR id=$1)`, p.TaskID); err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM subnexus_model_evaluation_results WHERE ($1::bigint=0 OR task_id=$1)`, p.TaskID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	res, err := tx.ExecContext(ctx, `WITH ranked AS (
 SELECT r.id,r.created_at,t.retention_days,t.max_records,ROW_NUMBER() OVER (PARTITION BY r.task_id ORDER BY r.created_at DESC,r.id DESC) AS rn
 FROM subnexus_model_evaluation_results r JOIN subnexus_model_evaluation_tasks t ON t.id=r.task_id WHERE ($1::bigint=0 OR r.task_id=$1)
) DELETE FROM subnexus_model_evaluation_results WHERE id IN (SELECT id FROM ranked WHERE rn>max_records OR created_at<NOW()-retention_days*INTERVAL '1 day')`, p.TaskID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *modelEvaluationRepository) Cleanup(ctx context.Context, p service.ModelEvaluationCleanupParams) (int64, error) {
	tx, err := r.transaction(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := cleanupModelEvaluationTx(ctx, tx, p)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

func (r *modelEvaluationRepository) Claim(ctx context.Context, id int64, force bool) (*service.ModelEvaluationTask, error) {
	return r.claim(ctx, id, force, false)
}

func (r *modelEvaluationRepository) ClaimTest(ctx context.Context, id int64) (*service.ModelEvaluationTask, error) {
	if id <= 0 {
		return nil, service.ErrModelEvaluationInvalid
	}
	return r.claim(ctx, id, true, true)
}

func (r *modelEvaluationRepository) claim(ctx context.Context, id int64, force bool, isTest bool) (*service.ModelEvaluationTask, error) {
	tx, err := r.transaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var slotID int
	err = tx.QueryRowContext(ctx, `SELECT id FROM subnexus_model_evaluation_slots WHERE (lease_until IS NULL OR lease_until<=NOW()) AND ($1 OR `+modelEvaluationEnabledSQL+`) ORDER BY id LIMIT 1 FOR UPDATE`, isTest).Scan(&slotID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	task, err := scanModelEvaluationTask(tx.QueryRowContext(ctx, `SELECT `+modelEvaluationTaskColumns+` FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id
 WHERE ($3 OR (t.enabled AND t.published)) AND g.status='active' AND g.deleted_at IS NULL AND (t.lease_until IS NULL OR t.lease_until<=NOW()) AND ($1::bigint=0 OR t.id=$1) AND ($2 OR t.next_run_at<=NOW()) ORDER BY t.next_run_at,t.id LIMIT 1 FOR UPDATE OF t`, id, force, isTest))
	if errors.Is(err, service.ErrModelEvaluationNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	task.LeaseToken = uuid.NewString()
	task.LeaseIsTest = isTest
	if _, err = tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_slots SET lease_token=$2,lease_until=NOW()+INTERVAL '240 seconds' WHERE id=$1`, slotID, task.LeaseToken); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET lease_token=$2,lease_until=NOW()+INTERVAL '240 seconds',next_run_at=NOW()+interval_seconds*INTERVAL '1 second',lease_is_test=$3,revision=revision+1,
published=CASE WHEN $3 THEN FALSE ELSE published END,test_status=CASE WHEN $3 THEN 'running' ELSE test_status END,test_error=CASE WHEN $3 THEN '' ELSE test_error END,last_tested_at=CASE WHEN $3 THEN NULL ELSE last_tested_at END WHERE id=$1 RETURNING revision,published,test_status,test_error,last_tested_at`, task.ID, task.LeaseToken, isTest).Scan(&task.Revision, &task.Published, &task.TestStatus, &task.TestError, &task.LastTestedAt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func (r *modelEvaluationRepository) LeaseCurrent(ctx context.Context, t *service.ModelEvaluationTask) (bool, error) {
	var current bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id WHERE t.id=$1 AND t.revision=$2 AND t.lease_token=$3 AND t.lease_until>NOW() AND g.status='active' AND g.deleted_at IS NULL AND `+modelEvaluationLeaseRuntimeSQL+`)`, t.ID, t.Revision, t.LeaseToken).Scan(&current)
	return current, err
}

func (r *modelEvaluationRepository) Complete(ctx context.Context, t *service.ModelEvaluationTask, result *service.ModelEvaluationResult) (bool, error) {
	tx, err := r.transaction(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var current bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subnexus_model_evaluation_tasks t JOIN groups g ON g.id=t.group_id WHERE t.id=$1 AND t.revision=$2 AND t.lease_token=$3 AND t.lease_until>NOW() AND g.status='active' AND g.deleted_at IS NULL AND `+modelEvaluationLeaseRuntimeSQL+`)`, t.ID, t.Revision, t.LeaseToken).Scan(&current)
	if err != nil || !current {
		return false, err
	}
	result.IsTest = t.LeaseIsTest
	err = tx.QueryRowContext(ctx, `INSERT INTO subnexus_model_evaluation_results(task_id,group_id,group_name,task_name,model,status,duration_ms,error_message,html,created_at,is_test,configuration_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),$10,$11) RETURNING id,created_at`, t.ID, t.GroupID, t.GroupName, t.Name, t.Model, result.Status, result.DurationMS, result.ErrorMessage, result.HTML, t.LeaseIsTest, t.ConfigurationRevision).Scan(&result.ID, &result.CreatedAt)
	if err != nil {
		return false, err
	}
	if t.LeaseIsTest {
		status := "failed"
		if result.Status == "success" {
			status = "passed"
		}
		if _, err = tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET test_status=$2,test_error=$3,last_tested_at=NOW(),updated_at=NOW() WHERE id=$1`, t.ID, status, result.ErrorMessage); err != nil {
			return false, err
		}
	}
	if _, err = cleanupModelEvaluationTx(ctx, tx, service.ModelEvaluationCleanupParams{TaskID: t.ID}); err != nil {
		return false, err
	}
	if err = releaseModelEvaluationTx(ctx, tx, t); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func releaseModelEvaluationTx(ctx context.Context, tx *sql.Tx, t *service.ModelEvaluationTask) error {
	if _, err := tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_slots SET lease_token='',lease_until=NULL WHERE lease_token=$1`, t.LeaseToken); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE subnexus_model_evaluation_tasks SET lease_token='',lease_until=NULL,lease_is_test=FALSE,next_run_at=GREATEST(next_run_at,NOW()+interval_seconds*INTERVAL '1 second'),
test_status=CASE WHEN test_status='running' THEN 'failed' ELSE test_status END,test_error=CASE WHEN test_status='running' THEN '测试已中断，请重新测试' ELSE test_error END,last_tested_at=CASE WHEN test_status='running' THEN NOW() ELSE last_tested_at END WHERE id=$1 AND lease_token=$2`, t.ID, t.LeaseToken)
	return err
}

func (r *modelEvaluationRepository) Release(ctx context.Context, t *service.ModelEvaluationTask) error {
	tx, err := r.transaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = releaseModelEvaluationTx(ctx, tx, t); err != nil {
		return err
	}
	return tx.Commit()
}
