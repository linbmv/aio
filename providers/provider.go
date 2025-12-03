package providers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/atopos31/llmio/consts"
)

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"` // 使用 int64 存储 Unix 时间戳
	OwnedBy string `json:"owned_by"`
}

type Provider interface {
	BuildReq(ctx context.Context, header http.Header, model string, rawData []byte) (*http.Request, error)
	Models(ctx context.Context) ([]Model, error)
}

var (
	providerCache sync.Map
)

func cacheKey(providerType, config string) string {
	hash := md5.Sum([]byte(providerType + ":" + config))
	return hex.EncodeToString(hash[:])
}

func New(Type, providerConfig string) (Provider, error) {
	key := cacheKey(Type, providerConfig)
	if cached, ok := providerCache.Load(key); ok {
		return cached.(Provider), nil
	}

	var provider Provider
	var err error
	switch Type {
	case consts.StyleOpenAI:
		var openai OpenAI
		if err = json.Unmarshal([]byte(providerConfig), &openai); err != nil {
			return nil, errors.New("invalid openai config")
		}
		provider = &openai
	case consts.StyleOpenAIRes:
		var openaiRes OpenAIRes
		if err = json.Unmarshal([]byte(providerConfig), &openaiRes); err != nil {
			return nil, errors.New("invalid openai-res config")
		}
		provider = &openaiRes
	case consts.StyleAnthropic:
		var anthropic Anthropic
		if err = json.Unmarshal([]byte(providerConfig), &anthropic); err != nil {
			return nil, errors.New("invalid anthropic config")
		}
		provider = &anthropic
	default:
		return nil, errors.New("unknown provider")
	}

	actual, _ := providerCache.LoadOrStore(key, provider)
	return actual.(Provider), nil
}
