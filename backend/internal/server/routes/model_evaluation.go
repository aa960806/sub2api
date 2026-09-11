package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// These groups inherit the existing JWT/admin authentication, compliance and
// audit middleware. Result HTML is returned as JSON, never as a same-origin page.
func registerModelEvaluationAdminRoutes(parent *gin.RouterGroup, h *handler.Handlers, limiter *middleware.PanelRateLimiter) {
	g := parent.Group("/model-evaluations")
	if limiter != nil {
		g.Use(limiter.Heavy())
	}
	s := h.Admin.ModelEvaluation
	g.GET("/config", s.GetConfig)
	g.PUT("/config", s.UpdateConfig)
	g.GET("/tasks", s.ListTasks)
	g.POST("/tasks", s.CreateTask)
	g.PUT("/tasks/:id", s.UpdateTask)
	g.DELETE("/tasks/:id", s.DeleteTask)
	g.POST("/tasks/:id/run", s.RunNow)
	g.POST("/tasks/:id/test", s.TestTask)
	g.PUT("/tasks/:id/publication", s.SetPublication)
	g.GET("/results", s.ListResults)
	g.GET("/results/:id", s.GetResult)
	g.DELETE("/results/:id", s.DeleteResult)
	g.POST("/cleanup", s.Cleanup)
}
func registerModelEvaluationUserRoutes(parent *gin.RouterGroup, h *handler.Handlers, limiter *middleware.PanelRateLimiter) {
	g := parent.Group("/model-evaluations")
	if limiter != nil {
		g.Use(limiter.Heavy())
	}
	s := h.ModelEvaluation
	g.GET("/groups", s.Groups)
	g.GET("/results", s.ListResults)
	g.GET("/results/:id", s.GetResult)
}
