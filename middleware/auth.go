package middleware

import (
	"net/http"
	"strings"

	"github.com/atopos31/llmio/common"
	"github.com/gin-gonic/gin"
)

func Auth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 不设置token，则不进行验证
		if token == "" {
			c.Next()
			return
		}

		// 只从 Authorization Bearer 提取网关 TOKEN，避免与上游 Provider 的 X-Api-Key 混淆
		auth := c.GetHeader("Authorization")
		if auth == "" {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "API key is missing")
			c.Abort()
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "Invalid authorization format")
			c.Abort()
			return
		}

		extractedKey := strings.TrimPrefix(auth, "Bearer ")
		if extractedKey != token {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "Invalid token")
			c.Abort()
			return
		}
		c.Next()
	}
}

func AuthAnthropic(koken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 不设置token，则不进行验证
		if koken == "" {
			c.Next()
			return
		}

		// Anthropic 支持两种认证方式：x-api-key 优先，兼容 Authorization Bearer
		apiKey := c.GetHeader("x-api-key")
		if apiKey == "" {
			auth := c.GetHeader("Authorization")
			if auth != "" && strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey == "" {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "API key is missing")
			c.Abort()
			return
		}

		if apiKey != koken {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "Invalid token")
			c.Abort()
			return
		}
		c.Next()
	}
}

// AuthEither 同时支持 Authorization Bearer 和 x-api-key（用于兼容路由）
// 优先检查 x-api-key，避免错误 Bearer 覆盖正确 x-api-key 的情况
func AuthEither(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}

		// 优先检查 x-api-key（Anthropic 风格）
		apiKey := c.GetHeader("x-api-key")
		if apiKey == "" {
			// 再检查 Authorization Bearer（OpenAI 风格）
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey == "" {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "API key is missing")
			c.Abort()
			return
		}
		if apiKey != token {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "Invalid token")
			c.Abort()
			return
		}
		c.Next()
	}
}
