package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ModelEvaluationService owns a bounded, durable scheduler. Nothing starts until
// Start, and a missing/invalid global setting always closes provider traffic.
type ModelEvaluationService struct {
	repo         ModelEvaluationRepository
	settings     SettingRepository
	groups       GroupRepository
	encryptor    SecretEncryptor
	client       *http.Client
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	started      bool
	stopped      bool
	active       map[int64]modelEvaluationActiveRun
	wg           sync.WaitGroup
	tickInterval time.Duration
}

type modelEvaluationActiveRun struct {
	token  string
	cancel context.CancelFunc
	isTest bool
}

func NewModelEvaluationService(repo ModelEvaluationRepository, settings SettingRepository, groups GroupRepository, encryptor SecretEncryptor) *ModelEvaluationService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ModelEvaluationService{repo: repo, settings: settings, groups: groups, encryptor: encryptor, ctx: ctx, cancel: cancel, tickInterval: 5 * time.Second, active: make(map[int64]modelEvaluationActiveRun), client: &http.Client{
		Timeout:       180 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DialContext: modelEvaluationSafeDialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 170 * time.Second, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("redirects are disabled") },
	}}
}

func (s *ModelEvaluationService) GetConfig(ctx context.Context) (*ModelEvaluationConfig, error) {
	if s == nil || s.settings == nil {
		return &ModelEvaluationConfig{}, nil
	}
	v, err := s.settings.GetValue(ctx, SettingKeyModelEvaluationEnabled)
	if err != nil {
		return &ModelEvaluationConfig{}, nil
	}
	return &ModelEvaluationConfig{Enabled: v == "true"}, nil
}

func (s *ModelEvaluationService) UpdateConfig(ctx context.Context, cfg ModelEvaluationConfig) (*ModelEvaluationConfig, error) {
	if err := s.repo.SetEnabled(ctx, cfg.Enabled); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		s.cancelScheduled()
	}
	return &cfg, nil
}

func (s *ModelEvaluationService) ListTasks(ctx context.Context) ([]*ModelEvaluationTask, error) {
	return s.repo.ListTasks(ctx)
}

func (s *ModelEvaluationService) CreateTask(ctx context.Context, in ModelEvaluationTaskInput) (*ModelEvaluationTask, error) {
	task, err := s.prepareTask(ctx, in, nil)
	if err != nil {
		return nil, err
	}
	if err = s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ModelEvaluationService) UpdateTask(ctx context.Context, id int64, in ModelEvaluationTaskInput) (*ModelEvaluationTask, error) {
	old, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	task, err := s.prepareTask(ctx, in, old)
	if err != nil {
		return nil, err
	}
	if err = s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	s.cancelActive(id)
	return task, nil
}

func (s *ModelEvaluationService) DeleteTask(ctx context.Context, id int64) error {
	if err := s.repo.DeleteTask(ctx, id); err != nil {
		return err
	}
	s.cancelActive(id)
	return nil
}

func (s *ModelEvaluationService) ListGroups(ctx context.Context, allowedGroupIDs []int64) ([]ModelEvaluationGroup, error) {
	cfg, _ := s.GetConfig(ctx)
	if !cfg.Enabled || len(allowedGroupIDs) == 0 {
		return []ModelEvaluationGroup{}, nil
	}
	return s.repo.ListGroups(ctx, allowedGroupIDs)
}

func (s *ModelEvaluationService) ListResults(ctx context.Context, p ModelEvaluationListParams) ([]*ModelEvaluationResult, int64, error) {
	if !p.Admin {
		cfg, _ := s.GetConfig(ctx)
		if !cfg.Enabled || len(p.AllowedGroupIDs) == 0 {
			return []*ModelEvaluationResult{}, 0, nil
		}
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Page > 100000 {
		p.Page = 100000
	}
	if p.PageSize < 1 || p.PageSize > 100 {
		p.PageSize = 20
	}
	items, total, err := s.repo.ListResults(ctx, p)
	if err != nil || p.Admin {
		return items, total, err
	}
	visible := make([]*ModelEvaluationResult, 0, len(items))
	for _, item := range items {
		if safe := ModelEvaluationUserResult(item); safe != nil {
			visible = append(visible, safe)
		} else if total > 0 {
			total--
		}
	}
	return visible, total, nil
}

func (s *ModelEvaluationService) GetResult(ctx context.Context, id int64, p ModelEvaluationListParams) (*ModelEvaluationResult, error) {
	if !p.Admin {
		cfg, _ := s.GetConfig(ctx)
		if !cfg.Enabled || len(p.AllowedGroupIDs) == 0 {
			return nil, ErrModelEvaluationNotFound
		}
	}
	result, err := s.repo.GetResult(ctx, id, p)
	if err != nil || p.Admin {
		return result, err
	}
	safe := ModelEvaluationUserResult(result)
	if safe == nil {
		return nil, ErrModelEvaluationNotFound
	}
	return safe, nil
}

// ModelEvaluationUserResult is the final disclosure boundary for generated
// failures. Return a copy so shared/cache/admin objects retain their diagnostics.
func ModelEvaluationUserResult(result *ModelEvaluationResult) *ModelEvaluationResult {
	if result == nil {
		return nil
	}
	if result.IsTest && result.Status != "success" {
		return nil
	}
	copy := *result
	copy.ErrorMessage = ""
	if copy.Status != "success" {
		copy.ErrorMessage = "生成失败"
		copy.HTML = ""
	}
	return &copy
}

func (s *ModelEvaluationService) SetPublication(ctx context.Context, id int64, published bool) (*ModelEvaluationTask, error) {
	task, err := s.repo.SetPublication(ctx, id, published)
	if err == nil && !published {
		s.cancelActive(id)
	}
	return task, err
}

// TestTask is an explicit administrator action and intentionally works while
// scheduling and public visibility are disabled. It shares the same DB slots.
func (s *ModelEvaluationService) TestTask(ctx context.Context, id int64) error {
	task, err := s.repo.ClaimTest(ctx, id)
	if err != nil {
		return err
	}
	if task == nil {
		return ErrModelEvaluationBusy
	}
	return s.launch(task)
}

func (s *ModelEvaluationService) DeleteResult(ctx context.Context, id int64) error {
	return s.repo.DeleteResult(ctx, id)
}

func (s *ModelEvaluationService) Cleanup(ctx context.Context, p ModelEvaluationCleanupParams) (int64, error) {
	if p.TaskID < 0 {
		return 0, ErrModelEvaluationInvalid
	}
	n, err := s.repo.Cleanup(ctx, p)
	if err == nil && p.All {
		s.cancelActive(p.TaskID)
	}
	return n, err
}

func (s *ModelEvaluationService) prepareTask(ctx context.Context, in ModelEvaluationTaskInput, old *ModelEvaluationTask) (*ModelEvaluationTask, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.APIKey = strings.TrimSpace(in.APIKey)
	in.Model = strings.TrimSpace(in.Model)
	if in.IntervalSeconds == 0 {
		in.IntervalSeconds = 3600
	}
	if in.RetentionDays == 0 {
		in.RetentionDays = 7
	}
	if in.MaxRecords == 0 {
		in.MaxRecords = 50
	}
	if in.APIFormat == "" {
		in.APIFormat = "chat_completions"
	}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 || in.GroupID <= 0 || in.Model == "" || len(in.Model) > 200 || strings.ContainsAny(in.Model, "\r\n\x00") || in.IntervalSeconds < 60 || in.IntervalSeconds > 604800 || in.RetentionDays < 1 || in.RetentionDays > 90 || in.MaxRecords < 1 || in.MaxRecords > 200 || len(in.APIKey) > 4096 || strings.ContainsAny(in.APIKey, "\r\n\x00") {
		return nil, ErrModelEvaluationInvalid
	}
	switch in.APIFormat {
	case "chat_completions", "responses", "messages":
	default:
		return nil, ErrModelEvaluationInvalid
	}
	// An encrypted credential is bound to its original destination/protocol/group.
	// Do not silently forward an existing secret after changing that binding.
	if old != nil && in.APIKey == "" && (in.Endpoint != old.Endpoint || in.APIFormat != old.APIFormat || in.GroupID != old.GroupID) {
		return nil, ErrModelEvaluationCredentialRequired
	}
	if err := validateModelEvaluationEndpoint(ctx, in.Endpoint); err != nil {
		return nil, err
	}
	group, err := s.groups.GetByID(ctx, in.GroupID)
	if err != nil || group == nil || group.Status != StatusActive {
		return nil, ErrModelEvaluationInvalid
	}
	task := &ModelEvaluationTask{Name: in.Name, GroupID: in.GroupID, GroupName: group.Name, Endpoint: in.Endpoint, APIFormat: in.APIFormat, Model: in.Model, Enabled: in.Enabled, IntervalSeconds: in.IntervalSeconds, RetentionDays: in.RetentionDays, MaxRecords: in.MaxRecords}
	if old != nil {
		task.ID = old.ID
		task.APIKeyEncrypted = old.APIKeyEncrypted
		task.CreatedAt = old.CreatedAt
		task.Revision = old.Revision
		task.Published = old.Published
		task.TestStatus = old.TestStatus
		task.TestError = old.TestError
		task.LastTestedAt = old.LastTestedAt
		task.ConfigurationRevision = old.ConfigurationRevision
	}
	if in.APIKey != "" {
		encrypted, err := s.encryptor.Encrypt(in.APIKey)
		if err != nil {
			return nil, errors.New("cannot encrypt model evaluation credential")
		}
		task.APIKeyEncrypted = encrypted
	}
	if task.APIKeyEncrypted == "" {
		return nil, ErrModelEvaluationInvalid
	}
	task.HasAPIKey = true
	if old == nil {
		task.TestStatus = "untested"
		task.ConfigurationRevision = 1
	}
	return task, nil
}

func validateModelEvaluationEndpoint(ctx context.Context, endpoint string) error {
	if len(endpoint) > 2048 {
		return ErrModelEvaluationInvalid
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || isBlockedHostname(u.Hostname()) {
		return ErrModelEvaluationInvalid
	}
	// The saved URL is a complete request URL; it is never silently rewritten.
	if u.Path == "" || u.Path == "/" {
		return ErrModelEvaluationEndpointPathRequired
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (isPrivateIP(ip) || !ip.IsGlobalUnicast()) {
		return ErrModelEvaluationInvalid
	}
	dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	blocked, err := isPrivateOrLoopbackHost(dnsCtx, u.Hostname())
	if err != nil || blocked {
		return ErrModelEvaluationInvalid
	}
	return nil
}

func (s *ModelEvaluationService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	s.wg.Add(1)
	go s.loop()
}

func (s *ModelEvaluationService) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	s.client.CloseIdleConnections()
}

func (s *ModelEvaluationService) cancelActive(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for taskID, run := range s.active {
		if id == 0 || id == taskID {
			run.cancel()
		}
	}
}

func (s *ModelEvaluationService) cancelScheduled() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.active {
		if !run.isTest {
			run.cancel()
		}
	}
}

func (s *ModelEvaluationService) RunNow(ctx context.Context, id int64) error {
	cfg, _ := s.GetConfig(ctx)
	if !cfg.Enabled {
		return ErrModelEvaluationDisabled
	}
	task, err := s.repo.Claim(ctx, id, true)
	if err != nil {
		return err
	}
	if task == nil {
		return ErrModelEvaluationBusy
	}
	return s.launch(task)
}

func (s *ModelEvaluationService) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()
	lastCleanup := time.Time{}
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		cfg, _ := s.GetConfig(ctx)
		if !cfg.Enabled {
			s.cancelScheduled()
			cancel()
			continue
		}
		if time.Since(lastCleanup) > time.Hour {
			if _, err := s.repo.Cleanup(ctx, ModelEvaluationCleanupParams{}); err == nil {
				lastCleanup = time.Now()
			}
		}
		for range 2 {
			task, err := s.repo.Claim(ctx, 0, false)
			if err != nil || task == nil {
				break
			}
			if s.launch(task) != nil {
				break
			}
		}
		cancel()
	}
}

func (s *ModelEvaluationService) launch(task *ModelEvaluationTask) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.Release(ctx, task)
		return ErrModelEvaluationDisabled
	}
	ctx, cancel := context.WithTimeout(s.ctx, 180*time.Second)
	s.active[task.ID] = modelEvaluationActiveRun{token: task.LeaseToken, cancel: cancel, isTest: task.LeaseIsTest}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer func() {
			s.mu.Lock()
			if run, ok := s.active[task.ID]; ok && run.token == task.LeaseToken {
				delete(s.active, task.ID)
			}
			s.mu.Unlock()
		}()
		watchDone := make(chan struct{})
		watchExited := make(chan struct{})
		defer func() { cancel(); close(watchDone); <-watchExited }()
		go func() {
			defer close(watchExited)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-watchDone:
					return
				case <-ticker.C:
					checkCtx, done := context.WithTimeout(ctx, 5*time.Second)
					valid, err := s.repo.LeaseCurrent(checkCtx, task)
					done()
					if err != nil || !valid {
						cancel()
						return
					}
				}
			}
		}()
		current, err := s.repo.LeaseCurrent(ctx, task)
		if err == nil && current {
			result := s.execute(ctx, task)
			result.IsTest = task.LeaseIsTest
			// A timeout is a useful failed sample; shutdown/config cancellation is not.
			if !errors.Is(ctx.Err(), context.Canceled) {
				saveCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
				_, _ = s.repo.Complete(saveCtx, task, result)
				done()
			}
		}
		releaseCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.repo.Release(releaseCtx, task)
		done()
	}()
	return nil
}
