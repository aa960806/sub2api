package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type evaluationUserStub struct {
	modelEvaluationUserService
	enabled bool
	seen    service.ModelEvaluationListParams
	calls   int
}

func (s *evaluationUserStub) GetConfig(context.Context) (*service.ModelEvaluationConfig, error) {
	return &service.ModelEvaluationConfig{Enabled: s.enabled}, nil
}
func (s *evaluationUserStub) ListGroups(_ context.Context, ids []int64) ([]service.ModelEvaluationGroup, error) {
	s.calls++
	s.seen.AllowedGroupIDs = ids
	return []service.ModelEvaluationGroup{{ID: 7, Name: "Allowed"}}, nil
}
func (s *evaluationUserStub) ListResults(_ context.Context, p service.ModelEvaluationListParams) ([]*service.ModelEvaluationResult, int64, error) {
	s.calls++
	s.seen = p
	return []*service.ModelEvaluationResult{}, 0, nil
}
func (s *evaluationUserStub) GetResult(_ context.Context, _ int64, p service.ModelEvaluationListParams) (*service.ModelEvaluationResult, error) {
	s.calls++
	s.seen = p
	return nil, service.ErrModelEvaluationNotFound
}

type evaluationGroupsStub struct {
	calls int
	err   error
}

func (s *evaluationGroupsStub) GetAvailableGroups(_ context.Context, id int64) ([]service.Group, error) {
	s.calls++
	if id != 42 {
		return nil, errors.New("unexpected user")
	}
	return []service.Group{{ID: 7}}, s.err
}
func evaluationContext(path string, authenticated bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if authenticated {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	}
	return c, w
}
func TestModelEvaluationUserRequiresAuthentication(t *testing.T) {
	s := &evaluationUserStub{enabled: true}
	groups := &evaluationGroupsStub{}
	h := &ModelEvaluationUserHandler{service: s, groups: groups}
	c, w := evaluationContext("/model-evaluations/groups", false)
	h.Groups(c)
	require.Equal(t, 401, w.Code)
	require.Zero(t, groups.calls)
	require.Zero(t, s.calls)
}
func TestModelEvaluationUserDisabledDoesNotReadGroupsOrResults(t *testing.T) {
	s := &evaluationUserStub{}
	groups := &evaluationGroupsStub{}
	h := &ModelEvaluationUserHandler{service: s, groups: groups}
	c, w := evaluationContext("/model-evaluations/groups", true)
	h.Groups(c)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"enabled":false`)
	c, w = evaluationContext("/model-evaluations/results", true)
	h.ListResults(c)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"items":[]`)
	c, w = evaluationContext("/model-evaluations/results/99", true)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	h.GetResult(c)
	require.Equal(t, 404, w.Code)
	require.Zero(t, groups.calls)
	require.Zero(t, s.calls)
}
func TestModelEvaluationUserFiltersCannotBroadenGroupScope(t *testing.T) {
	s := &evaluationUserStub{enabled: true}
	groups := &evaluationGroupsStub{}
	h := &ModelEvaluationUserHandler{service: s, groups: groups}
	c, w := evaluationContext("/model-evaluations/results?group_id=99&task_id=23&page=2&page_size=12&admin=true", true)
	h.ListResults(c)
	require.Equal(t, 200, w.Code)
	require.False(t, s.seen.Admin)
	require.Equal(t, []int64{7}, s.seen.AllowedGroupIDs)
	require.EqualValues(t, 99, s.seen.GroupID)
	require.Equal(t, 2, s.seen.Page)
	require.Equal(t, 12, s.seen.PageSize)
	c, w = evaluationContext("/model-evaluations/results/99?admin=true", true)
	c.Params = gin.Params{{Key: "id", Value: "99"}}
	h.GetResult(c)
	require.Equal(t, 404, w.Code)
	require.False(t, s.seen.Admin)
	require.Equal(t, []int64{7}, s.seen.AllowedGroupIDs)
	require.Equal(t, 2, groups.calls, "permissions must be re-read for every list/detail request")
}
func TestModelEvaluationUserGroupLookupFailureDoesNotReadResults(t *testing.T) {
	s := &evaluationUserStub{enabled: true}
	groups := &evaluationGroupsStub{err: errors.New("unavailable")}
	h := &ModelEvaluationUserHandler{service: s, groups: groups}
	c, w := evaluationContext("/model-evaluations/results", true)
	h.ListResults(c)
	require.Equal(t, 500, w.Code)
	require.Zero(t, s.calls)
}
