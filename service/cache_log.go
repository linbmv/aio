package service

import (
	"context"

	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/service/cache"
)

// RecordCacheHit 记录缓存命中的审计日志
func RecordCacheHit(ctx context.Context, cacheKey cache.Key, cached *cache.Value, reqMeta models.ReqMeta) {
	// 异步记录，不阻塞响应
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 记录日志失败不应影响主流程，仅记录错误
				// TODO: 可以添加日志记录
			}
		}()

		// 从上下文获取AuthKeyID
		authKeyID, _ := ctx.Value(consts.ContextKeyAuthKeyID).(uint)

		// 构建缓存命中日志
		log := models.ChatLog{
			Name:          cacheKey.Scope.Model,
			ProviderModel: cached.ProviderModel,
			ProviderName:  cached.ProviderName,
			Status:        "success",
			Style:         cacheKey.Scope.Style,
			UserAgent:     reqMeta.UserAgent,
			RemoteIP:      reqMeta.RemoteIP,
			AuthKeyID:     authKeyID,
			ChatIO:        false, // 缓存命中不记录IO
			Size:          len(cached.Body),
			Cached:        true,
			CachedFromLogID: &cached.SourceLogID,
		}

		// 复制Usage信息（如果存在）
		if cached.Usage != nil {
			if usage, ok := cached.Usage.(models.Usage); ok {
				log.Usage = usage
			}
		}

		// 保存到数据库
		if err := models.DB.Create(&log).Error; err != nil {
			// TODO: 可以添加错误日志记录
		}
	}()
}

// RecordCacheWrite 在写入缓存时记录相关信息到缓存值中
func RecordCacheWrite(logID uint, usage models.Usage, providerName, providerModel string) cache.Value {
	return cache.Value{
		SourceLogID:   logID,
		Usage:         usage,
		ProviderName:  providerName,
		ProviderModel: providerModel,
	}
}