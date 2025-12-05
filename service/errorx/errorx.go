package errorx

import (
	"encoding/json"
	"net/http"
	"strings"
)

type ErrorLevel int

const (
	ErrorNone ErrorLevel = iota
	ErrorClient
	ErrorKey
	ErrorChannel
)

type ClassifiedError struct {
	Level      ErrorLevel
	HTTPStatus int
	Code       string
	Message    string
	Retryable  bool
}

// ClassifyHTTPError 分类 HTTP 错误
func ClassifyHTTPError(status int, body []byte, header http.Header) ClassifiedError {
	if status >= 200 && status < 300 {
		return ClassifiedError{Level: ErrorNone, HTTPStatus: status}
	}

	bodyStr := strings.ToLower(string(body))
	pErr := parseProviderError(body)

	switch status {
	case 400, 422:
		// 先检查是否为key失效错误
		if pErr.Code == "invalid_api_key" || pErr.Type == "authentication_error" ||
			strings.Contains(bodyStr, "invalid") || strings.Contains(bodyStr, "expired") {
			return ClassifiedError{
				Level:      ErrorKey,
				HTTPStatus: status,
				Code:       "key_invalid",
				Retryable:  true,
			}
		}
		return ClassifiedError{
			Level:      ErrorClient,
			HTTPStatus: status,
			Code:       "bad_request",
			Retryable:  false,
		}
	case 404:
		return ClassifiedError{
			Level:      ErrorClient,
			HTTPStatus: status,
			Code:       "not_found",
			Retryable:  false,
		}
	case 401, 403:
		return ClassifiedError{
			Level:      ErrorKey,
			HTTPStatus: status,
			Code:       "auth_invalid",
			Retryable:  false,
		}
	case 429:
		if pErr.Code == "insufficient_quota" || pErr.Type == "insufficient_quota" || isGlobalRateLimit(bodyStr, header) {
			return ClassifiedError{
				Level:      ErrorChannel,
				HTTPStatus: status,
				Code:       "rate_limit_global",
				Retryable:  true,
			}
		}
		if pErr.Type == "rate_limit_error" || strings.Contains(bodyStr, "rate limit") {
			return ClassifiedError{
				Level:      ErrorKey,
				HTTPStatus: status,
				Code:       "rate_limit_key",
				Retryable:  true,
			}
		}
		return ClassifiedError{
			Level:      ErrorKey,
			HTTPStatus: status,
			Code:       "rate_limit_key",
			Retryable:  true,
		}
	case 413:
		return ClassifiedError{
			Level:      ErrorClient,
			HTTPStatus: status,
			Code:       "payload_too_large",
			Retryable:  false,
		}
	case 500, 502, 503, 504, 520, 521, 524:
		return ClassifiedError{
			Level:      ErrorChannel,
			HTTPStatus: status,
			Code:       "upstream_unavailable",
			Retryable:  true,
		}
	default:
		// 先检查key失效
		if pErr.Code == "invalid_api_key" || pErr.Type == "authentication_error" ||
			strings.Contains(bodyStr, "invalid") || strings.Contains(bodyStr, "expired") {
			return ClassifiedError{
				Level:      ErrorKey,
				HTTPStatus: status,
				Code:       "key_invalid",
				Retryable:  true,
			}
		}
		// 再判断客户端错误
		if status >= 400 && status < 500 {
			return ClassifiedError{
				Level:      ErrorClient,
				HTTPStatus: status,
				Code:       "client_error",
				Retryable:  false,
			}
		}
		return ClassifiedError{
			Level:      ErrorChannel,
			HTTPStatus: status,
			Code:       "upstream_error",
			Retryable:  true,
		}
	}
}

func isGlobalRateLimit(body string, header http.Header) bool {
	scope := header.Get("X-RateLimit-Scope")
	if scope == "account" || scope == "organization" {
		return true
	}
	if header.Get("Retry-After") != "" {
		return true
	}
	if strings.Contains(body, "account") && strings.Contains(body, "rate limit") {
		return true
	}
	return false
}

type providerError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func parseProviderError(body []byte) providerError {
	var wrapper struct {
		Error providerError `json:"error"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil {
		wrapper.Error.Type = strings.ToLower(wrapper.Error.Type)
		wrapper.Error.Code = strings.ToLower(wrapper.Error.Code)
		return wrapper.Error
	}
	return providerError{}
}
