package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SettingKeyModelEvaluationEnabled = "subnexus_model_evaluation_enabled"
	ModelEvaluationPrompt            = "生成html，内容是svg绘制鹈鹕骑自行车2D动画，不用进行测试。"
	ModelEvaluationMaxTasks          = 100
	ModelEvaluationMaxHTMLBytes      = 512 * 1024
	// Request timeout is the provider HTTP budget; the worker and DB lease add
	// room for retries, backoff, and final persistence.
	ModelEvaluationRequestTimeoutSeconds = 600
	ModelEvaluationTotalTimeoutSeconds   = 660
	ModelEvaluationLeaseTimeoutSeconds   = 720
	ModelEvaluationMaxResponseBytes      = 16 * 1024 * 1024
)

// ModelEvaluationReasoningEfforts is the set accepted by the monitoring UI;
// the selected value remains subject to the configured upstream contract.
var ModelEvaluationReasoningEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

func IsValidModelEvaluationReasoningEffort(value string) bool {
	if value == "" {
		return true
	}
	for _, allowed := range ModelEvaluationReasoningEfforts {
		if value == allowed {
			return true
		}
	}
	return false
}

var (
	ErrModelEvaluationNotFound             = infraerrors.NotFound("MODEL_EVALUATION_NOT_FOUND", "model evaluation not found")
	ErrModelEvaluationDisabled             = infraerrors.Forbidden("MODEL_EVALUATION_DISABLED", "model evaluation monitoring is disabled")
	ErrModelEvaluationInvalid              = infraerrors.BadRequest("MODEL_EVALUATION_INVALID", "invalid model evaluation configuration")
	ErrModelEvaluationBusy                 = infraerrors.Conflict("MODEL_EVALUATION_BUSY", "model evaluation is running or worker capacity is full")
	ErrModelEvaluationLimit                = infraerrors.BadRequest("MODEL_EVALUATION_LIMIT", "model evaluation task limit reached")
	ErrModelEvaluationCredentialRequired   = infraerrors.BadRequest("MODEL_EVALUATION_CREDENTIAL_REQUIRED", "changing the endpoint, API format or group requires re-entering the API Key")
	ErrModelEvaluationTestRequired         = infraerrors.BadRequest("MODEL_EVALUATION_TEST_REQUIRED", "a successful test of the current configuration is required before publication")
	ErrModelEvaluationEndpointPathRequired = infraerrors.BadRequest("MODEL_EVALUATION_ENDPOINT_PATH_REQUIRED", "a complete request URL including the API path is required")
)

type ModelEvaluationConfig struct {
	Enabled bool `json:"enabled"`
}

type ModelEvaluationTaskInput struct {
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	Endpoint  string `json:"endpoint"`
	APIFormat string `json:"api_format"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	// ReasoningEffort optionally controls the upstream model's reasoning depth.
	// Empty preserves the historical request payload for providers that do not
	// support a reasoning control.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	Enabled         bool    `json:"enabled"`
	IntervalSeconds int     `json:"interval_seconds"`
	RetentionDays   int     `json:"retention_days"`
	MaxRecords      int     `json:"max_records"`
}

type ModelEvaluationTask struct {
	ID                    int64      `json:"id"`
	Name                  string     `json:"name"`
	GroupID               int64      `json:"group_id"`
	GroupName             string     `json:"group_name"`
	Endpoint              string     `json:"endpoint"`
	APIFormat             string     `json:"api_format"`
	Model                 string     `json:"model"`
	ReasoningEffort       string     `json:"reasoning_effort,omitempty"`
	Enabled               bool       `json:"enabled"`
	Published             bool       `json:"published"`
	TestStatus            string     `json:"test_status"`
	LastTestedAt          *time.Time `json:"last_tested_at"`
	TestError             string     `json:"test_error"`
	IntervalSeconds       int        `json:"interval_seconds"`
	RetentionDays         int        `json:"retention_days"`
	MaxRecords            int        `json:"max_records"`
	HasAPIKey             bool       `json:"has_api_key"`
	NextRunAt             *time.Time `json:"next_run_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	APIKeyEncrypted       string     `json:"-"`
	Revision              int64      `json:"-"`
	LeaseToken            string     `json:"-"`
	LeaseIsTest           bool       `json:"-"`
	ConfigurationRevision int64      `json:"-"`
}

type ModelEvaluationGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ModelEvaluationResult struct {
	ID           int64     `json:"id"`
	TaskID       int64     `json:"task_id"`
	GroupID      int64     `json:"group_id"`
	GroupName    string    `json:"group_name"`
	TaskName     string    `json:"task_name"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	IsTest       bool      `json:"is_test"`
	DurationMS   int64     `json:"duration_ms"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	HTML         string    `json:"html,omitempty"`
	// transientStreamFailure is internal retry metadata and is never persisted
	// or returned to clients.
	transientStreamFailure string
}

// Admin is set only by an authenticated administrator handler. Empty permitted
// groups deny every user result; nil never means unrestricted user access.
type ModelEvaluationListParams struct {
	Admin           bool
	AllowedGroupIDs []int64
	GroupID         int64
	TaskID          int64
	Page            int
	PageSize        int
}

type ModelEvaluationCleanupParams struct {
	TaskID int64 `json:"task_id"`
	All    bool  `json:"all"`
}

type ModelEvaluationRepository interface {
	SetEnabled(context.Context, bool) error
	ListTasks(context.Context) ([]*ModelEvaluationTask, error)
	GetTask(context.Context, int64) (*ModelEvaluationTask, error)
	CreateTask(context.Context, *ModelEvaluationTask) error
	UpdateTask(context.Context, *ModelEvaluationTask) error
	DeleteTask(context.Context, int64) error
	SetPublication(context.Context, int64, bool) (*ModelEvaluationTask, error)
	ListGroups(context.Context, []int64) ([]ModelEvaluationGroup, error)
	ListResults(context.Context, ModelEvaluationListParams) ([]*ModelEvaluationResult, int64, error)
	GetResult(context.Context, int64, ModelEvaluationListParams) (*ModelEvaluationResult, error)
	DeleteResult(context.Context, int64) error
	Cleanup(context.Context, ModelEvaluationCleanupParams) (int64, error)
	Claim(context.Context, int64, bool) (*ModelEvaluationTask, error)
	ClaimTest(context.Context, int64) (*ModelEvaluationTask, error)
	LeaseCurrent(context.Context, *ModelEvaluationTask) (bool, error)
	Complete(context.Context, *ModelEvaluationTask, *ModelEvaluationResult) (bool, error)
	Release(context.Context, *ModelEvaluationTask) error
}
