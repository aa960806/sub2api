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
	"strings"
	"time"
)

func (s *ModelEvaluationService) execute(ctx context.Context, task *ModelEvaluationTask) *ModelEvaluationResult {
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
	payload := map[string]any{"model": task.Model, "stream": true}
	switch task.APIFormat {
	case "chat_completions":
		payload["messages"] = []map[string]string{{"role": "user", "content": ModelEvaluationPrompt}}
		payload["max_tokens"] = 16384
		if task.ReasoningEffort != "" {
			payload["reasoning_effort"] = task.ReasoningEffort
		}
	case "responses":
		payload["input"] = ModelEvaluationPrompt
		payload["max_output_tokens"] = 16384
		if task.ReasoningEffort != "" {
			payload["reasoning"] = map[string]string{"effort": task.ReasoningEffort}
		}
	case "messages":
		payload["messages"] = []map[string]string{{"role": "user", "content": ModelEvaluationPrompt}}
		payload["max_tokens"] = 16384
		if budget := modelEvaluationThinkingBudget(task.ReasoningEffort); budget > 0 {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, task.Endpoint, bytes.NewReader(body))
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
	resp, err := s.client.Do(req)
	if err != nil {
		result.ErrorMessage = modelEvaluationRequestError(ctx, err)
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorMessage = modelEvaluationHTTPError(resp.StatusCode)
		return result
	}
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
			reason = "上游响应超过 2 MiB 限制"
		} else {
			content, reason = parseModelEvaluationResponse(task.APIFormat, raw)
		}
	}
	if ctx.Err() != nil {
		reason = modelEvaluationRequestError(ctx, ctx.Err())
	}
	if reason != "" {
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

func modelEvaluationRequestError(ctx context.Context, err error) string {
	var timeout net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return "等待上游响应超时（单次请求最多 180 秒），请检查模型生成耗时与代理超时设置"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "请求已取消"
	}
	return "请求或读取响应失败，请检查接口地址、网络与上游服务状态"
}

// Fixed messages keep credentials and untrusted provider bodies out of history.
func modelEvaluationHTTPError(status int) string {
	switch status {
	case 524:
		return "上游返回 HTTP 524：代理等待模型响应超时，请检查上游耗时与代理超时设置"
	case 408, 504:
		return fmt.Sprintf("上游返回 HTTP %d：请求超时，请检查上游服务与代理超时设置", status)
	case 401, 403:
		return fmt.Sprintf("上游返回 HTTP %d：鉴权或访问被拒绝，请检查 Key、模型权限与访问策略", status)
	case 429:
		return "上游返回 HTTP 429：请求被限流或额度受限，请检查上游限制"
	default:
		return fmt.Sprintf("上游返回 HTTP %d，请检查接口协议、模型配置与服务状态", status)
	}
}

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
