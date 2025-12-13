package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/service/cache"
)

// BuildCacheKey 构造缓存键，确保按AuthKeyID与关键参数进行隔离
// 返回ok=false表示本次请求不参与缓存
func BuildCacheKey(ctx context.Context, style string, before Before) (cache.Key, bool) {
	var empty cache.Key

	// 仅对非stream请求做缓存，避免复杂的SSE重放问题
	if before.Stream {
		return empty, false
	}

	// 必须存在AuthKeyID，确保多租户隔离
	rawAuthKeyID := ctx.Value(consts.ContextKeyAuthKeyID)
	authKeyID, ok := rawAuthKeyID.(uint)
	if !ok {
		// 管理员token可能没有AuthKeyID，暂时不缓存
		return empty, false
	}

	// 解析并规范化请求体，只包含影响输出的字段
	bodyHash, err := normalizeAndHashRequestBody(before.raw)
	if err != nil {
		return empty, false
	}

	// 确定模式标识
	mode := determineMode(style)

	key := cache.Key{
		Scope: cache.Scope{
			AuthKeyID: authKeyID,
			Style:     style,
			Model:     before.Model,
			Mode:      mode,
			Stream:    before.Stream,
		},
		BodyHash: bodyHash,
	}

	return key, true
}

// normalizeAndHashRequestBody 规范化请求体并生成哈希
func normalizeAndHashRequestBody(rawBody []byte) (string, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(rawBody, &raw); err != nil {
		return "", fmt.Errorf("failed to unmarshal request body: %w", err)
	}

	// 定义影响模型输出的关键字段（基于OpenAI、Anthropic、OpenAI Responses API）
	semanticFields := []string{
		// 基本字段
		"model",
		"messages",            // chat/messages 风格
		"input",               // responses API / vision 输入
		"stream",

		// 输出数量/长度控制
		"max_tokens",
		"max_tokens_to_sample",     // Anthropic
		"max_completion_tokens",    // OpenAI responses
		"n",                        // 返回多少条 completion
		"stop",
		"stop_sequences",

		// 采样控制
		"temperature",
		"top_p",
		"top_k",
		"seed",
		"presence_penalty",
		"frequency_penalty",

		// 结果形式/结构
		"response_format",          // 包含其中的 format/json_schema 等
		"tool_choice",
		"tool_choice_type",         // 若序列化时拆成 type
		"tools",
		"function_call",            // 旧版 openai
		"functions",                // 旧版 openai

		// logprob/置信度相关
		"logprobs",
		"top_logprobs",
		"logit_bias",

		// 角色/指令补充
		"system",
		"user",                     // responses API 里可能单独存在
		"metadata",                 // Anthropic/Responses 都允许附带
		"parallel_tool_calls",      // OpenAI responses 支持并行工具调用开关
		"reasoning_effort",         // OpenAI responses，影响深度/成本
		"modalities",               // OpenAI responses，控制输出模态
		"audio",                    // responses 模式下的音频配置
		"vision",                   // vision 相关配置字段
	}

	// 提取语义相关字段
	normalized := make(map[string]interface{})
	for _, field := range semanticFields {
		if value, exists := raw[field]; exists {
			normalized[field] = value
		}
	}

	// 确保JSON编码的稳定性：按键排序
	return hashMapStably(normalized)
}

// hashMapStably 对map进行稳定的哈希计算
func hashMapStably(data map[string]interface{}) (string, error) {
	// 按键排序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建有序的map
	ordered := make(map[string]interface{}, len(data))
	for _, k := range keys {
		ordered[k] = data[k]
	}

	// JSON编码
	jsonBytes, err := json.Marshal(ordered)
	if err != nil {
		return "", fmt.Errorf("failed to marshal normalized data: %w", err)
	}

	// SHA256哈希
	sum := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(sum[:]), nil
}

// determineMode 根据style确定模式标识
func determineMode(style string) string {
	switch style {
	case consts.StyleOpenAI:
		return "chat_completions"
	case consts.StyleOpenAIRes:
		return "responses"
	case consts.StyleAnthropic:
		return "messages"
	default:
		// 未知类型使用原始style，便于扩展
		return style
	}
}

// ValidateCacheKey 验证缓存键的有效性
func ValidateCacheKey(key cache.Key) error {
	if key.Scope.AuthKeyID == 0 {
		return fmt.Errorf("AuthKeyID cannot be zero")
	}
	if key.Scope.Style == "" {
		return fmt.Errorf("Style cannot be empty")
	}
	if key.Scope.Model == "" {
		return fmt.Errorf("Model cannot be empty")
	}
	if key.BodyHash == "" {
		return fmt.Errorf("BodyHash cannot be empty")
	}
	return nil
}