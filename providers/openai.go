package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/tidwall/sjson"
)

type OpenAI struct {
	BaseURL string      `json:"base_url"`
	APIKey  string      `json:"api_key"`
	Keys    []KeyConfig `json:"keys"`
}

// pickKey 随机抽取状态有效的 key，兼容旧 api_key 配置
func (o *OpenAI) pickKey() string {
	if len(o.Keys) > 0 {
		type activeKey struct {
			term   string
			remark string
		}
		active := make([]activeKey, 0, len(o.Keys))
		for _, key := range o.Keys {
			if key.Status && key.Term != "" {
				active = append(active, activeKey{term: key.Term, remark: key.Remark})
			}
		}
		if len(active) > 0 {
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			selected := active[rng.Intn(len(active))]
			keyHint := selected.term[len(selected.term)-4:]
			if selected.remark != "" {
				keyHint = selected.remark
			}
			fmt.Printf("[OpenAI] using key: %s\n", keyHint)
			return selected.term
		}
	}
	return o.APIKey
}

func (o *OpenAI) BuildReqWithKey(ctx context.Context, header http.Header, model string, rawBody []byte, key string, keyID uint) (*http.Request, uint, error) {
	body, err := sjson.SetBytes(rawBody, "model", model)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", o.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if header != nil {
		req.Header = header
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LLMIO-Gateway/1.0")
	apiKey := key
	usedKeyID := keyID
	if apiKey == "" {
		apiKey = o.pickKey()
		usedKeyID = 0
	}
	if apiKey == "" {
		return nil, 0, fmt.Errorf("no openai api key available")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	return req, usedKeyID, nil
}

func (o *OpenAI) BuildReq(ctx context.Context, header http.Header, model string, rawBody []byte) (*http.Request, error) {
	req, _, err := o.BuildReqWithKey(ctx, header, model, rawBody, "", 0)
	return req, err
}

func (o *OpenAI) Models(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/models", o.BaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.pickKey()))
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
