package middleware

import (
	"net/http"

	"github.com/atopos31/llmio/common"
	"github.com/gin-gonic/gin"
)

// DefaultMaxBodyBytes 默认聊天请求体大小上限（字节）
// 目前固定为 10MB，如需调整可后续通过配置/环境变量注入。
const DefaultMaxBodyBytes int64 = 10 * 1024 * 1024

// LimitRequestBody 限制请求体大小，超限直接返回 413 Payload Too Large。
//
// - 若 maxBytes <= 0，则不做任何限制，直接放行；
// - 若请求头 Content-Length 已超过限制，则直接返回错误，避免继续读取 body；
// - 否则使用 http.MaxBytesReader 包裹 Body，后续 ReadAll 若超过限制会返回 http.ErrBodyTooLarge。
func LimitRequestBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 {
			c.Next()
			return
		}

		if c.Request.ContentLength > maxBytes {
			if c.Request.Body != nil {
				c.Request.Body.Close()
			}
			common.PayloadTooLarge(c, "request body too large")
			c.Abort()
			return
		}

		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}

		c.Next()
	}
}
