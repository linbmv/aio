package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/sjson"
)

type Anthropic struct {
	BaseURL     string   `json:"base_url"`
	APIKey      string   `json:"api_key,omitempty"`
	APIKeys     []string `json:"api_keys,omitempty"`
	Version     string   `json:"version"`
	KeyStrategy string   `json:"key_strategy,omitempty"` // sequential | round_robin
	ProviderID  uint     `json:"-"`
	cursor      uint64   `json:"-"`
}

func (a *Anthropic) BuildReq(ctx context.Context, header http.Header, model string, rawBody []byte) (*http.Request, string, error) {
	key, err := a.pickKey()
	if err != nil {
		return nil, "", err
	}
	body, err := sjson.SetBytes(rawBody, "model", model)
	if err != nil {
		return nil, key, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/messages", a.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, key, err
	}
	if header != nil {
		req.Header = header
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", a.Version)
	return req, key, nil
}

type AnthropicModelsResponse struct {
	Data    []AnthropicModel `json:"data"`
	FirstID string           `json:"first_id"`
	HasMore bool             `json:"has_more"`
	LastID  string           `json:"last_id"`
}

type AnthropicModel struct {
	CreatedAt   time.Time `json:"created_at"`
	DisplayName string    `json:"display_name"`
	ID          string    `json:"id"`
	Type        string    `json:"type"`
}

func (a *Anthropic) Models(ctx context.Context) ([]Model, error) {
	key, err := a.pickKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/models", a.BaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", a.Version)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}
	var anthropicModels AnthropicModelsResponse
	if err := json.NewDecoder(res.Body).Decode(&anthropicModels); err != nil {
		return nil, err
	}

	var modelList ModelList
	for _, model := range anthropicModels.Data {
		modelList.Data = append(modelList.Data, Model{
			ID:      model.ID,
			Created: model.CreatedAt.Unix(),
		})
	}
	return modelList.Data, nil
}

func (a *Anthropic) pickKey() (string, error) {
	strategy := strings.TrimSpace(a.KeyStrategy)
	if strategy == "" {
		strategy = "sequential"
	}

	filtered := make([]string, 0, len(a.APIKeys))
	for _, k := range a.APIKeys {
		if s := strings.TrimSpace(k); s != "" {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) > 0 {
		switch strategy {
		case "round_robin":
			idx := atomic.AddUint64(&a.cursor, 1)
			for range filtered {
				key := filtered[(idx-1)%uint64(len(filtered))]
				if IsKeyCoolingDown(a.ProviderID, key) {
					idx++
					continue
				}
				return key, nil
			}
			return "", errors.New("all api keys are cooling down")
		case "sequential":
			fallthrough
		default:
			for _, key := range filtered {
				if IsKeyCoolingDown(a.ProviderID, key) {
					continue
				}
				return key, nil
			}
			return "", errors.New("all api keys are cooling down")
		}
	}
	if key := strings.TrimSpace(a.APIKey); key != "" {
		if IsKeyCoolingDown(a.ProviderID, key) {
			return "", errors.New("api key is cooling down")
		}
		return key, nil
	}
	return "", errors.New("no api key configured for anthropic provider")
}
