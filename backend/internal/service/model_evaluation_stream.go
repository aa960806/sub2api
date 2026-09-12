package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Only completed output is returned. Partial HTML, provider errors and reasoning
// tokens never become a successful sample. Wire bytes and assembled text are
// bounded independently, including SSE comments and otherwise ignored events.
func readModelEvaluationStream(format string, body io.Reader) (string, string) {
	limited := &io.LimitedReader{R: body, N: ModelEvaluationMaxResponseBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), ModelEvaluationMaxResponseBytes+1)
	var data strings.Builder
	var output strings.Builder
	eventName, finish := "", ""
	appendText := func(text string) bool {
		if output.Len()+len(text) > ModelEvaluationMaxHTMLBytes {
			return false
		}
		output.WriteString(text)
		return true
	}
	process := func() (bool, string, string) {
		payload := strings.TrimSpace(data.String())
		data.Reset()
		name := eventName
		eventName = ""
		if payload == "" {
			return false, "", ""
		}
		if payload == "[DONE]" {
			if format == "chat_completions" && finish == "stop" {
				return true, output.String(), ""
			}
			return true, "", "生成未完整结束，结果未保存"
		}
		var event struct {
			Type     string          `json:"type"`
			Error    json.RawMessage `json:"error"`
			Response json.RawMessage `json:"response"`
			Delta    json.RawMessage `json:"delta"`
			Choices  []struct {
				Index        int    `json:"index"`
				FinishReason string `json:"finish_reason"`
				Delta        struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
		}
		if json.Unmarshal([]byte(payload), &event) != nil {
			return true, "", "上游流式响应格式无效"
		}
		if event.Type == "" {
			event.Type = name
		}
		if (len(event.Error) > 0 && string(event.Error) != "null") || event.Type == "error" || event.Type == "response.failed" {
			return true, "", "上游流式响应返回错误，请检查模型配置与服务状态"
		}
		switch format {
		case "responses":
			switch event.Type {
			case "response.output_text.delta":
				var delta string
				if json.Unmarshal(event.Delta, &delta) != nil {
					return true, "", "上游流式响应格式无效"
				}
				if !appendText(delta) {
					return true, "", "生成内容超过 512 KiB 限制"
				}
			case "response.completed":
				// The completed response is authoritative, including its status,
				// output item boundaries and any refusal/error information.
				content, reason := parseModelEvaluationResponse(format, event.Response)
				return true, content, reason
			case "response.incomplete":
				return true, "", "生成未完整结束，结果未保存"
			}
		case "chat_completions":
			for _, choice := range event.Choices {
				if choice.Index != 0 {
					continue
				}
				if !appendText(choice.Delta.Content) {
					return true, "", "生成内容超过 512 KiB 限制"
				}
				if choice.FinishReason != "" {
					finish = choice.FinishReason
					if finish != "stop" {
						return true, "", "生成未完整结束，结果未保存"
					}
				}
			}
		case "messages":
			switch event.Type {
			case "content_block_start":
				if event.ContentBlock.Type == "text" && !appendText(event.ContentBlock.Text) {
					return true, "", "生成内容超过 512 KiB 限制"
				}
			case "content_block_delta", "message_delta":
				var delta struct {
					Type       string `json:"type"`
					Text       string `json:"text"`
					StopReason string `json:"stop_reason"`
				}
				if json.Unmarshal(event.Delta, &delta) != nil {
					return true, "", "上游流式响应格式无效"
				}
				if delta.Type == "text_delta" && !appendText(delta.Text) {
					return true, "", "生成内容超过 512 KiB 限制"
				}
				if delta.StopReason != "" {
					finish = delta.StopReason
				}
			case "message_stop":
				if finish != "end_turn" && finish != "stop_sequence" {
					return true, "", "生成未完整结束，结果未保存"
				}
				return true, output.String(), ""
			}
		default:
			return true, "", "请求协议配置无效"
		}
		return false, "", ""
	}
	for scanner.Scan() {
		if limited.N <= 0 {
			return "", fmt.Sprintf("上游响应超过 %d MiB 限制", ModelEvaluationMaxResponseBytes/(1024*1024))
		}
		line := scanner.Text()
		if line == "" {
			if done, content, reason := process(); done {
				return content, reason
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		} else if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
	}
	if limited.N <= 0 {
		return "", fmt.Sprintf("上游响应超过 %d MiB 限制", ModelEvaluationMaxResponseBytes/(1024*1024))
	}
	if scanner.Err() != nil {
		if errors.Is(scanner.Err(), context.DeadlineExceeded) || strings.Contains(strings.ToLower(scanner.Err().Error()), "timeout") {
			return "", modelEvaluationTransientStreamTimeout
		}
		return "", modelEvaluationTransientStreamFailure
	}
	// Tolerate a missing final blank line only when a complete terminal event is
	// present; EOF alone never confirms a successful generation.
	if done, content, reason := process(); done {
		return content, reason
	}
	return "", modelEvaluationTransientStreamFailure
}
