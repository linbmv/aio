package authx

import (
	"net/http"
	"strings"
)

// ExtractAPIKey 从请求中提取 API Key，支持多种方式
// 优先级：Authorization Bearer > X-Api-Key > X-Goog-Api-Key > Query key
func ExtractAPIKey(r *http.Request) string {
	// 1. Authorization: Bearer
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// 2. X-Api-Key (Anthropic)
	if key := r.Header.Get("X-Api-Key"); key != "" {
		return key
	}

	// 3. X-Goog-Api-Key (Gemini)
	if key := r.Header.Get("X-Goog-Api-Key"); key != "" {
		return key
	}

	// 4. Query parameter
	if key := r.URL.Query().Get("key"); key != "" {
		return key
	}

	return ""
}

// DetectProviderFromPath 根据路径自动识别 Provider 类型
func DetectProviderFromPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		return "anthropic"
	case strings.HasPrefix(path, "/v1/responses"):
		return "openai-res"
	case strings.HasPrefix(path, "/v1beta/"):
		return "gemini"
	case strings.HasPrefix(path, "/v1/chat/completions"), strings.HasPrefix(path, "/v1/completions"):
		return "openai"
	default:
		return "openai" // 默认 OpenAI
	}
}
