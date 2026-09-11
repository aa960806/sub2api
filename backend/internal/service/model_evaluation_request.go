package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	payload := map[string]any{"model": task.Model, "stream": false}
	switch task.APIFormat {
	case "chat_completions":
		payload["messages"] = []map[string]string{{"role": "user", "content": ModelEvaluationPrompt}}
		payload["max_tokens"] = 16384
	case "responses":
		payload["input"] = ModelEvaluationPrompt
		payload["max_output_tokens"] = 16384
	case "messages":
		payload["messages"] = []map[string]string{{"role": "user", "content": ModelEvaluationPrompt}}
		payload["max_tokens"] = 16384
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
	if task.APIFormat == "messages" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			result.ErrorMessage = "请求超时或已取消"
		} else {
			result.ErrorMessage = "请求失败，请检查地址、凭据与服务状态"
		}
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorMessage = fmt.Sprintf("上游返回 HTTP %d，请检查配置或额度", resp.StatusCode)
		return result
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, ModelEvaluationMaxResponseBytes+1))
	if err != nil {
		result.ErrorMessage = "读取上游响应失败"
		return result
	}
	if len(raw) > ModelEvaluationMaxResponseBytes {
		result.ErrorMessage = "上游响应超过 2 MiB 限制"
		return result
	}
	content, reason := parseModelEvaluationResponse(task.APIFormat, raw)
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
