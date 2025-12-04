package middleware

import (
	"net/http"

	"github.com/atopos31/llmio/common"
	"github.com/atopos31/llmio/middleware/authx"
	"github.com/gin-gonic/gin"
)

func Auth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 不设置token，则不进行验证
		if token == "" {
			return
		}

		// 使用增强的认证提取，支持多种方式
		extractedKey := authx.ExtractAPIKey(c.Request)
		if extractedKey == "" {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "API key is missing")
			c.Abort()
			return
		}

		if extractedKey != token {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "Invalid token")
			c.Abort()
			return
		}
	}
}

func AuthAnthropic(koken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 不设置token，则不进行验证
		if koken == "" {
			return
		}

		// 使用增强的认证提取，支持多种方式
		extractedKey := authx.ExtractAPIKey(c.Request)
		if extractedKey == "" {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "API key is missing")
			c.Abort()
			return
		}

		if extractedKey != koken {
			common.ErrorWithHttpStatus(c, http.StatusUnauthorized, http.StatusUnauthorized, "Invalid token")
			c.Abort()
			return
		}
	}
}
