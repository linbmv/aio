package handler

import (
	"net/http"
	"strconv"

	"github.com/atopos31/llmio/common"
	"github.com/gin-gonic/gin"
)

// GetCacheStats 获取缓存统计信息
func GetCacheStats(c *gin.Context) {
	if chatCache == nil {
		common.ErrorWithHttpStatus(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "cache not enabled")
		return
	}

	stats := chatCache.Stats()
	common.Success(c, stats)
}

// ClearCacheByAuthKey 按AuthKeyID清空缓存
func ClearCacheByAuthKey(c *gin.Context) {
	if chatCache == nil {
		common.ErrorWithHttpStatus(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "cache not enabled")
		return
	}

	authKeyIDStr := c.Param("authKeyId")
	authKeyID, err := strconv.ParseUint(authKeyIDStr, 10, 32)
	if err != nil {
		common.ErrorWithHttpStatus(c, http.StatusBadRequest, http.StatusBadRequest, "invalid auth key id")
		return
	}

	ctx := c.Request.Context()
	if err := chatCache.DeleteByAuthKey(ctx, uint(authKeyID)); err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	common.Success(c, gin.H{"message": "cache cleared successfully"})
}

// ClearCacheByStyle 按API风格清空缓存
func ClearCacheByStyle(c *gin.Context) {
	if chatCache == nil {
		common.ErrorWithHttpStatus(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "cache not enabled")
		return
	}

	style := c.Param("style")
	if style == "" {
		common.ErrorWithHttpStatus(c, http.StatusBadRequest, http.StatusBadRequest, "style cannot be empty")
		return
	}

	ctx := c.Request.Context()
	if err := chatCache.DeleteByStyle(ctx, style); err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	common.Success(c, gin.H{"message": "cache cleared successfully"})
}