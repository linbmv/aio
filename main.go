package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/atopos31/llmio/handler"
	"github.com/atopos31/llmio/middleware"
	"github.com/atopos31/llmio/models"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/crypto/x509roots/fallback"
)

func init() {
	ctx := context.Background()
	models.Init(ctx, "./db/llmio.db")
	slog.Info("TZ", "time.Local", time.Local.String())

	token := os.Getenv("TOKEN")

	// 强制要求 TOKEN，无论任何模式
	if token == "" {
		slog.Error("TOKEN is required - set TOKEN environment variable to secure your API")
		panic("TOKEN environment variable must be set")
	}
}

func main() {
	router := gin.Default()

	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/openai", "/anthropic", "/v1"})))

	token := os.Getenv("TOKEN")

	authOpenAI := middleware.Auth(token)
	authAnthropic := middleware.AuthAnthropic(token)
	authCompat := middleware.AuthEither(token)
	bodyLimit := middleware.LimitRequestBody(middleware.DefaultMaxBodyBytes)

	openai := router.Group("/openai/v1", authOpenAI)
	{
		openai.GET("/models", handler.OpenAIModelsHandler)
		openai.POST("/chat/completions", bodyLimit, handler.ChatCompletionsHandler)
		openai.POST("/responses", bodyLimit, handler.ResponsesHandler)
	}

	anthropic := router.Group("/anthropic/v1", authAnthropic)
	{
		anthropic.GET("/models", handler.AnthropicModelsHandler)
		anthropic.POST("/messages", bodyLimit, handler.Messages)
		// TODO
		anthropic.POST("/messages/count_tokens", bodyLimit)
	}

	// 兼容性保留
	v1 := router.Group("/v1", authCompat)
	{
		v1.GET("/models", handler.OpenAIModelsHandler)
		v1.POST("/chat/completions", bodyLimit, handler.ChatCompletionsHandler)
		v1.POST("/responses", bodyLimit, handler.ResponsesHandler)
		v1.POST("/messages", bodyLimit, handler.Messages)
		// TODO
		v1.POST("/messages/count_tokens", bodyLimit)
	}

	api := router.Group("/api")
	{
		api.Use(middleware.Auth(token))
		api.GET("/metrics/use/:days", handler.Metrics)
		api.GET("/metrics/counts", handler.Counts)
		// Provider management
		api.GET("/providers/template", handler.GetProviderTemplates)
		api.GET("/providers", handler.GetProviders)
		api.GET("/providers/models/:id", handler.GetProviderModels)
		api.POST("/providers", bodyLimit, handler.CreateProvider)
		api.PUT("/providers/:id", bodyLimit, handler.UpdateProvider)
		api.DELETE("/providers/:id", handler.DeleteProvider)

		// Model management
		api.GET("/models", handler.GetModels)
		api.POST("/models", bodyLimit, handler.CreateModel)
		api.PUT("/models/:id", bodyLimit, handler.UpdateModel)
		api.DELETE("/models/:id", handler.DeleteModel)

		// Model-provider association management
		api.GET("/model-providers", handler.GetModelProviders)
		api.GET("/model-providers/status", handler.GetModelProviderStatus)
		api.POST("/model-providers", bodyLimit, handler.CreateModelProvider)
		api.PUT("/model-providers/:id", bodyLimit, handler.UpdateModelProvider)
		api.PATCH("/model-providers/:id/status", bodyLimit, handler.UpdateModelProviderStatus)
		api.DELETE("/model-providers/:id", handler.DeleteModelProvider)

		// System status and monitoring
		api.GET("/logs", handler.GetRequestLogs)
		api.GET("/logs/:id/chat-io", handler.GetChatIO)
		api.GET("/user-agents", handler.GetUserAgents)

		// System configuration
		api.GET("/config", handler.GetSystemConfig)
		api.PUT("/config", bodyLimit, handler.UpdateSystemConfig)

		// Provider connectivity test
		api.GET("/test/:id", handler.ProviderTestHandler)
		api.GET("/test/react/:id", handler.TestReactHandler)
	}
	setwebui(router)
	router.Run(":7070")
}

//go:embed webui/dist
var distFiles embed.FS

//go:embed webui/dist/index.html
var indexHTML []byte

func setwebui(r *gin.Engine) {
	subFS, err := fs.Sub(distFiles, "webui/dist/assets")
	if err != nil {
		panic(err)
	}

	r.StaticFS("/assets", http.FS(subFS))

	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && !strings.HasPrefix(c.Request.URL.Path, "/api/") && !strings.HasPrefix(c.Request.URL.Path, "/v1/") {
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
			return
		}
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte("404 Not Found"))
	})
}
