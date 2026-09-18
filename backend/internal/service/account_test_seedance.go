package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *AccountTestService) testSeedanceAccountConnection(c *gin.Context, account *Account) error {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()
	s.sendEvent(c, TestEvent{Type: "test_start", Model: "seedance"})
	if err := validateSeedanceAccount(account.Platform, account.Type, account.Credentials); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	if _, err := s.validateUpstreamBaseURL(account.GetSeedanceBaseURL()); err != nil {
		return s.sendErrorAndEnd(c, "Seedance upstream URL is not allowed")
	}
	prober := seedanceProberFor(s.httpUpstream)
	if prober == nil || s.httpUpstream == nil {
		return s.sendErrorAndEnd(c, "Seedance credential probe is not configured")
	}
	if err := prober.ProbeCredential(c.Request.Context(), account); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	s.sendEvent(c, TestEvent{Type: "content", Text: "Seedance 凭证探测成功。此测试不生成视频、不验证完整出片；正式开放前仍需完成一次真实生成、查询和下载验收。"})
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
