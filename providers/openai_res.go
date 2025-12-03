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

	"github.com/tidwall/sjson"
)

// openai responses api
type OpenAIRes struct {
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key,omitempty"`
	APIKeys []string `json:"api_keys,omitempty"`
	cursor  uint64   `json:"-"`
}

func (o *OpenAIRes) BuildReq(ctx context.Context, header http.Header, model string, rawBody []byte) (*http.Request, error) {
	key, err := o.pickKey()
	if err != nil {
		return nil, err
	}
	body, err := sjson.SetBytes(rawBody, "model", model)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/responses", o.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if header != nil {
		req.Header = header
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))

	return req, nil
}

func (o *OpenAIRes) Models(ctx context.Context) ([]Model, error) {
	key, err := o.pickKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/models", o.BaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}

	var modelList ModelList
	if err := json.NewDecoder(res.Body).Decode(&modelList); err != nil {
		return nil, err
	}
	return modelList.Data, nil
}

func (o *OpenAIRes) pickKey() (string, error) {
	filtered := make([]string, 0, len(o.APIKeys))
	for _, k := range o.APIKeys {
		if s := strings.TrimSpace(k); s != "" {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) > 0 {
		idx := atomic.AddUint64(&o.cursor, 1)
		return filtered[(idx-1)%uint64(len(filtered))], nil
	}
	if strings.TrimSpace(o.APIKey) != "" {
		return strings.TrimSpace(o.APIKey), nil
	}
	return "", errors.New("no api key configured for openai-res provider")
}
