package service

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Extract the single complete document without parsing/re-serializing its DOM.
// Tokenizer offsets preserve the model's original bytes (including scripts,
// whitespace and case) and avoid treating an HTML string inside a script or
// comment as another document. Explanations around that document are discarded.
func extractModelEvaluationHTML(content string) (string, string) {
	if len(content) > ModelEvaluationMaxResponseBytes {
		return "", fmt.Sprintf("上游响应超过 %d MiB 限制", ModelEvaluationMaxResponseBytes/(1024*1024))
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return "", "HTML 编码无效"
	}
	segments, valid := modelEvaluationHTMLSegments(content)
	if !valid {
		return "", "响应不包含完整 HTML 文档"
	}
	var document string
	for _, segment := range segments {
		tokenizer := html.NewTokenizer(strings.NewReader(segment))
		offset, start := 0, -1
		opened := false
	tokens:
		for {
			kind := tokenizer.Next()
			rawStart := offset
			offset += len(tokenizer.Raw())
			if kind == html.ErrorToken {
				if tokenizer.Err() != io.EOF || start >= 0 {
					return "", "响应不包含完整 HTML 文档"
				}
				break tokens
			}
			token := tokenizer.Token()
			if !strings.EqualFold(token.Data, "html") {
				continue
			}
			switch kind {
			case html.DoctypeToken:
				if start >= 0 {
					return "", "响应包含多个或不完整的 HTML 文档"
				}
				start = rawStart
			case html.StartTagToken:
				if opened {
					return "", "响应包含多个或不完整的 HTML 文档"
				}
				if start < 0 {
					start = rawStart
				}
				opened = true
			case html.SelfClosingTagToken:
				return "", "响应不包含完整 HTML 文档"
			case html.EndTagToken:
				if !opened || start < 0 {
					return "", "响应不包含完整 HTML 文档"
				}
				if document != "" {
					return "", "响应包含多个 HTML 文档，请只生成一个"
				}
				if offset-start > ModelEvaluationMaxHTMLBytes {
					return "", "HTML 超过 512 KiB 限制"
				}
				document = segment[start:offset]
				start = -1
				opened = false
			}
		}
	}
	if document == "" {
		return "", "响应不包含完整 HTML 文档"
	}
	return document, ""
}

// Only raw text and HTML/unspecified fenced blocks can supply a document. Other
// code fences are skipped; an unterminated fence is rejected as truncated output.
func modelEvaluationHTMLSegments(content string) ([]string, bool) {
	var segments []string
	protected := modelEvaluationHTMLDocumentRanges(content)
	protectedIndex := 0
	outsideStart, bodyStart := 0, 0
	var fence byte
	fenceLength := 0
	eligible := false
	for offset := 0; offset < len(content); {
		lineEnd := strings.IndexByte(content[offset:], '\n')
		next := len(content)
		if lineEnd < 0 {
			lineEnd = len(content)
		} else {
			lineEnd += offset
			next = lineEnd + 1
		}
		line := strings.TrimSpace(content[offset:lineEnd])
		for protectedIndex < len(protected) && offset >= protected[protectedIndex][1] {
			protectedIndex++
		}
		insideDocument := protectedIndex < len(protected) && offset >= protected[protectedIndex][0] && offset < protected[protectedIndex][1]
		if !insideDocument && len(line) >= 3 && (line[0] == '`' || line[0] == '~') {
			count := 1
			for count < len(line) && line[count] == line[0] {
				count++
			}
			if count >= 3 {
				label := strings.TrimSpace(line[count:])
				if fence == 0 {
					segments = append(segments, content[outsideStart:offset])
					fence = line[0]
					fenceLength = count
					bodyStart = next
					eligible = label == "" || strings.EqualFold(label, "html")
				} else if line[0] == fence && count >= fenceLength && label == "" {
					if eligible {
						segments = append(segments, content[bodyStart:offset])
					}
					fence = 0
					outsideStart = next
				}
			}
		}
		offset = next
	}
	if fence != 0 {
		return nil, false
	}
	segments = append(segments, content[outsideStart:])
	return segments, true
}

// Markdown-looking lines inside the HTML itself (for example JavaScript template
// strings containing backticks) are page source, not an enclosing response fence.
func modelEvaluationHTMLDocumentRanges(content string) [][2]int {
	var ranges [][2]int
	tokenizer := html.NewTokenizer(strings.NewReader(content))
	offset, start := 0, -1
	for {
		kind := tokenizer.Next()
		rawStart := offset
		offset += len(tokenizer.Raw())
		if kind == html.ErrorToken {
			if start >= 0 {
				ranges = append(ranges, [2]int{start, len(content)})
			}
			return ranges
		}
		token := tokenizer.Token()
		if !strings.EqualFold(token.Data, "html") {
			continue
		}
		if (kind == html.StartTagToken || kind == html.DoctypeToken) && start < 0 {
			start = rawStart
		}
		if kind == html.EndTagToken && start >= 0 {
			ranges = append(ranges, [2]int{start, offset})
			start = -1
		}
	}
}
