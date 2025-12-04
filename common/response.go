package common

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// Success 成功响应
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	})
}

// Success 成功响应(原始数据)
func SuccessRaw(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// SuccessWithMessage 带消息的成功响应
func SuccessWithMessage(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: message,
		Data:    data,
	})
}

// Error 错误响应
func Error(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
	})
}

// ErrorWithHttpStatus 带HTTP状态码的错误响应
func ErrorWithHttpStatus(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
	})
}

// InternalServerError 内部服务器错误
func InternalServerError(c *gin.Context, message string) {
	// 记录详细错误到日志
	slog.Error("internal server error", "path", c.Request.URL.Path, "error", message)

	// 对外只返回通用错误信息
	c.JSON(http.StatusInternalServerError, Response{
		Code:    500,
		Error:   "Internal server error",
		Message: "An internal error occurred. Please try again later.",
	})
}

// BadRequest 请求参数错误
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusBadRequest,
		Message: message,
	})
}

// NotFound 资源未找到
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    404,
		Message: message,
	})
}

// Unauthorized 未授权
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    401,
		Message: message,
	})
}

// Forbidden 禁止访问
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:    403,
		Message: message,
	})
}

// PayloadTooLarge 请求体过大
func PayloadTooLarge(c *gin.Context, message string) {
	c.JSON(http.StatusRequestEntityTooLarge, Response{
		Code:    http.StatusRequestEntityTooLarge,
		Message: message,
	})
}
