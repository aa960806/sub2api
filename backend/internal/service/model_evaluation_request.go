package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	modelEvaluationTransientStreamFailure = "__MODEL_EVALUATION_TRANSIENT_STREAM_FAILURE__"
	modelEvaluationTransientStreamTimeout = "__MODEL_EVALUATION_TRANSIENT_STREAM_TIMEOUT__"
	modelEvaluationStreamFailureMessage   = "读取上游流式响应失败，请检查网络与上游超时设置"
)

func isModelEvaluationTransientStreamFailure(reason string) bool {
	return reason == modelEvaluationTransientStreamFailure || reason == modelEvaluationTransientStreamTimeout
}

func modelEvaluationTransientStreamFailureMessage(reason string) string {
	if reason == modelEvaluationTransientStreamTimeout {
		return fmt.Sprintf("等待上游响应超时（单次请求最多 %d 秒），请检查模型生成耗时与代理超时设置", ModelEvaluationRequestTimeoutSeconds)
	}
	return modelEvaluationStreamFailureMessage
}

func (s *ModelEvaluationService) execute(ctx context.Context, task *ModelEvaluationTask) *ModelEvaluationResult {
	started := time.Now()
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Duration(ModelEvaluationTotalTimeoutSeconds)*time.Second)
	defer cancel()
	attempts := 0
	var last *ModelEvaluationResult
	for attempts < modelEvaluationMaxAttempts {
		result := s.executeOnce(ctx, task, &attempts)
		last = result
		result.DurationMS = time.Since(started).Milliseconds()
		result.CreatedAt = started
		if !isModelEvaluationTransientStreamFailure(result.transientStreamFailure) {
			return result
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) <= time.Minute {
			result.ErrorMessage = modelEvaluationTransientStreamFailureMessage(result.transientStreamFailure)
			result.transientStreamFailure = ""
			return result
		}
	}
	if last != nil && isModelEvaluationTransientStreamFailure(last.transientStreamFailure) {
		last.ErrorMessage = modelEvaluationTransientStreamFailureMessage(last.transientStreamFailure)
		last.transientStreamFailure = ""
		return last
	}
	return &ModelEvaluationResult{Status: "error", ErrorMessage: modelEvaluationStreamFailureMessage, CreatedAt: started, DurationMS: time.Since(started).Milliseconds()}
}

func (s *ModelEvaluationService) executeOnce(ctx context.Context, task *ModelEvaluationTask, attempts *int) *ModelEvaluationResult {
	started := time.Now()
	result := &ModelEvaluationResult{TaskID: task.ID, GroupID: task.GroupID, GroupName: task.GroupName, TaskName: task.Name, Model: task.Model, Status: "error", CreatedAt: started}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	key, err := s.encryptor.Decrypt(task.APIKeyEncrypted)
	if err != nil || key == "" {
		result.ErrorMessage = "凭据解密失败，请重新填写 API Key"
		return result
	}
	// Incremental responses let upstream proxies flush data/heartbeats while a
	// long animation is generated, instead of waiting for the entire document.
	// Keep legacy `ultra` values readable in existing task rows, but send the
	// highest value accepted by the upstream Responses API (`max`).
	reasoningEffort := normalizeModelEvaluationOutboundEffort(task.ReasoningEffort)
	payload := map[string]any{"model": task.Model, "stream": true}
	switch task.APIFormat {
	case "chat_completions":
		payload["messages"] = []map[string]string{{"role": "user", "content": ModelEvaluationPrompt}}
		payload["max_tokens"] = 16384
		if reasoningEffort != "" {
			payload["reasoning_effort"] = reasoningEffort
		}
	case "responses":
		payload["input"] = ModelEvaluationPrompt
		payload["max_output_tokens"] = 16384
		if reasoningEffort != "" {
			// Responses counts reasoning and visible output in this budget; leave
			// sufficient room for the generated document at non-default efforts.
			payload["max_output_tokens"] = 32768
			payload["reasoning"] = map[string]string{"effort": reasoningEffort}
		}
	case "messages":
		payload["messages"] = []map[string]string{{"role": "user", "content": ModelEvaluationPrompt}}
		// Anthropic counts thinking tokens against max_tokens. Leave room for the
		// generated document when the selected effort uses a large budget.
		payload["max_tokens"] = 32768
		if budget := modelEvaluationThinkingBudget(reasoningEffort); budget > 0 {
			payload["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
		}
	default:
		result.ErrorMessage = "请求协议配置无效"
		return result
	}
	body, err := json.Marshal(payload)
	if err != nil {
		result.ErrorMessage = "无法生成请求"
		return result
	}
	var req *http.Request
	var resp *http.Response
	var rawStatusBody []byte
	var pendingRetryAfter string
	var pendingRetryStatus int
	var pendingRetryBody []byte
	for attempt := 0; attempt < modelEvaluationMaxAttempts && *attempts < modelEvaluationMaxAttempts; attempt++ {
		if attempt > 0 {
			if err := waitModelEvaluationRetry(ctx, attempt, pendingRetryAfter); err != nil {
				if pendingRetryStatus != 0 && errors.Is(err, errModelEvaluationRetryNotAllowed) {
					result.ErrorMessage = modelEvaluationHTTPError(pendingRetryStatus, pendingRetryBody, []byte(key))
					return result
				}
				result.ErrorMessage = modelEvaluationRequestError(ctx, err)
				return result
			}
			pendingRetryAfter = ""
			pendingRetryStatus = 0
			pendingRetryBody = nil
		}
		// Recreate the request so every retry has an independent body reader.
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, task.Endpoint, bytes.NewReader(body))
		if err != nil {
			result.ErrorMessage = "请求地址无效"
			return result
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream, application/json")
		if task.APIFormat == "messages" {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		*attempts = *attempts + 1
		resp, err = s.client.Do(req)
		if err != nil {
			if ctx.Err() != nil || !isRetryableModelEvaluationNetworkError(err) || attempt+1 >= modelEvaluationMaxAttempts {
				result.ErrorMessage = modelEvaluationRequestError(ctx, err)
				return result
			}
			continue
		}
		rawStatusBody = nil
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			retryAfter := resp.Header.Get("Retry-After")
			rawStatusBody, _ = io.ReadAll(io.LimitReader(resp.Body, modelEvaluationErrorBodyLimit+1))
			resp.Body.Close()
			if !isRetryableModelEvaluationStatus(resp.StatusCode) || attempt+1 >= modelEvaluationMaxAttempts || *attempts >= modelEvaluationMaxAttempts {
				result.ErrorMessage = modelEvaluationHTTPError(resp.StatusCode, rawStatusBody, []byte(key))
				return result
			}
			pendingRetryAfter = retryAfter
			pendingRetryStatus = resp.StatusCode
			pendingRetryBody = rawStatusBody
			continue
		}
		break
	}
	if resp == nil {
		result.ErrorMessage = "请求或读取响应失败，请检查接口地址、网络与上游服务状态"
		return result
	}
	defer resp.Body.Close()
	var content, reason string
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		content, reason = readModelEvaluationStream(task.APIFormat, resp.Body)
	} else {
		// Some compatible providers still return a completed JSON response.
		var raw []byte
		raw, err = io.ReadAll(io.LimitReader(resp.Body, ModelEvaluationMaxResponseBytes+1))
		if err != nil {
			reason = modelEvaluationRequestError(ctx, err)
		} else if len(raw) > ModelEvaluationMaxResponseBytes {
			reason = fmt.Sprintf("上游响应超过 %d MiB 限制", ModelEvaluationMaxResponseBytes/(1024*1024))
		} else {
			content, reason = parseModelEvaluationResponse(task.APIFormat, raw)
		}
	}
	if ctx.Err() != nil {
		reason = modelEvaluationRequestError(ctx, ctx.Err())
	}
	if reason != "" {
		if isModelEvaluationTransientStreamFailure(reason) {
			result.transientStreamFailure = reason
			return result
		}
		result.ErrorMessage = reason
		return result
	}
	html, reason := extractModelEvaluationHTML(content)
	if reason != "" {
		result.ErrorMessage = reason
		return result
	}
	if strings.Contains(html, key) {
		result.ErrorMessage = "响应包含敏感凭据，已拒绝保存"
		return result
	}
	result.HTML = html
	result.Status = "success"
	return result
}

// Anthropic Messages exposes a token budget rather than named efforts. Keep
// the same selector useful across protocols while omitting the field for
// "none" (and for the historical empty default).
func modelEvaluationThinkingBudget(effort string) int {
	switch effort {
	case "minimal":
		return 1024
	case "low":
		return 2048
	case "medium":
		return 4096
	case "high":
		return 8192
	case "xhigh", "max", "ultra":
		return 16384
	default:
		return 0
	}
}

const (
	modelEvaluationMaxAttempts    = 2
	modelEvaluationErrorBodyLimit = 16 * 1024
)

var errModelEvaluationRetryNotAllowed = errors.New("retry-after exceeds remaining evaluation budget")

func normalizeModelEvaluationOutboundEffort(effort string) string {
	if effort == "ultra" {
		return "max"
	}
	return effort
}

func isRetryableModelEvaluationStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func isRetryableModelEvaluationNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func waitModelEvaluationRetry(ctx context.Context, attempt int, retryAfter string) error {
	d := time.Duration(250*(1<<(attempt-1))) * time.Millisecond
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		d = time.Duration(seconds) * time.Second
	} else if when, err := http.ParseTime(strings.TrimSpace(retryAfter)); err == nil {
		d = time.Until(when)
	}
	if d > time.Duration(ModelEvaluationRequestTimeoutSeconds)*time.Second {
		return errModelEvaluationRetryNotAllowed
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= d {
		return errModelEvaluationRetryNotAllowed
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func modelEvaluationRequestError(ctx context.Context, err error) string {
	var timeout net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return fmt.Sprintf("等待上游响应超时（单次请求最多 %d 秒），请检查模型生成耗时与代理超时设置", ModelEvaluationRequestTimeoutSeconds)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "请求已取消"
	}
	return "请求或读取响应失败，请检查接口地址、网络与上游服务状态"
}

// Fixed messages keep credentials and untrusted provider bodies out of history.
func modelEvaluationHTTPError(status int, bodyAndKey ...[]byte) string {
	var detail string
	if len(bodyAndKey) > 0 {
		key := ""
		if len(bodyAndKey) > 1 {
			key = string(bodyAndKey[1])
		}
		detail = modelEvaluationProviderErrorDetail(bodyAndKey[0], key)
	}
	base := ""
	switch status {
	case 524:
		base = "上游返回 HTTP 524：代理等待模型响应超时，请检查上游耗时与代理超时设置"
	case 408, 504:
		base = fmt.Sprintf("上游返回 HTTP %d：请求超时，请检查上游服务与代理超时设置", status)
	case 401, 403:
		base = fmt.Sprintf("上游返回 HTTP %d：鉴权或访问被拒绝，请检查 Key、模型权限与访问策略", status)
	case 429:
		base = "上游返回 HTTP 429：请求被限流或额度受限，请检查上游限制"
	default:
		base = fmt.Sprintf("上游返回 HTTP %d，请检查接口协议、模型配置与服务状态", status)
	}
	if detail != "" {
		return base + "（" + detail + "）"
	}
	return base
}

func modelEvaluationProviderErrorDetail(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Param   string `json:"param"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return ""
	}
	code, param, kind, message := envelope.Error.Code, envelope.Error.Param, envelope.Error.Type, envelope.Error.Message
	if message == "" {
		message = envelope.Message
	}
	message = redactModelEvaluationErrorText(message, key)
	parts := make([]string, 0, 4)
	if code != "" {
		parts = append(parts, "code="+redactModelEvaluationErrorText(code, key))
	}
	if param != "" {
		parts = append(parts, "param="+redactModelEvaluationErrorText(param, key))
	}
	if kind != "" {
		parts = append(parts, "type="+redactModelEvaluationErrorText(kind, key))
	}
	if message != "" {
		parts = append(parts, "message="+message)
	}
	return strings.Join(parts, ", ")
}

func redactModelEvaluationErrorText(value, key string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if key != "" {
		value = strings.ReplaceAll(value, key, "[REDACTED]")
	}
	value = modelEvaluationSecretPattern.ReplaceAllString(value, "$1[REDACTED]")
	value = strings.ReplaceAll(value, ModelEvaluationPrompt, "[REQUEST_REDACTED]")
	if runes := []rune(value); len(runes) > 384 {
		value = string(runes[:384]) + "…"
	}
	return value
}

var modelEvaluationSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|x-api-key\s*[:=]\s*|api[_-]?key\s*[:=]\s*|token\s*[:=]\s*)[A-Za-z0-9._~+/=-]+`)

func parseModelEvaluationResponse(format string, raw []byte) (string, string) {
	var response struct {
		Error      json.RawMessage `json:"error"`
		Status     string          `json:"status"`
		StopReason string          `json:"stop_reason"`
		Choices    []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return "", "上游响应不是有效的 JSON"
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return "", "上游返回错误，请检查配置与服务状态"
	}
	switch format {
	case "chat_completions":
		if len(response.Choices) == 0 {
			return "", "上游响应没有文本结果"
		}
		if response.Choices[0].FinishReason != "stop" {
			return "", "生成未完整结束，结果未保存"
		}
		return response.Choices[0].Message.Content, ""
	case "responses":
		if response.Status != "completed" {
			return "", "生成未完整结束，结果未保存"
		}
		var parts []string
		for _, output := range response.Output {
			if output.Type == "message" && (output.Role == "assistant" || output.Role == "") {
				for _, c := range output.Content {
					if c.Type == "output_text" {
						parts = append(parts, c.Text)
					}
				}
			}
		}
		return strings.Join(parts, "\n"), ""
	case "messages":
		if response.StopReason != "end_turn" && response.StopReason != "stop_sequence" {
			return "", "生成未完整结束，结果未保存"
		}
		var parts []string
		for _, c := range response.Content {
			if c.Type == "text" {
				parts = append(parts, c.Text)
			}
		}
		return strings.Join(parts, "\n"), ""
	default:
		return "", "请求协议配置无效"
	}
}
