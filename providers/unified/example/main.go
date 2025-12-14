package main

import (
	"context"
	"fmt"
	"log"

	"github.com/atopos31/llmio/providers/unified"
)

func main() {
	fmt.Println("🚀 统一Provider示例 - 一个API Key，支持所有协议！")

	// 革命性功能：使用一个Anthropic配置支持OpenAI协议！
	provider, err := unified.CreateAnthropicCompatibleProvider(
		"https://api.anthropic.com",
		"sk-ant-your-api-key-here",
		"2023-06-01",
		[]unified.KeyConfig{
			{Term: "sk-ant-key1", Remark: "主要Key", Status: true},
			{Term: "sk-ant-key2", Remark: "备用Key", Status: true},
		},
	)
	if err != nil {
		log.Fatal("创建统一Provider失败:", err)
	}

	fmt.Printf("✅ 支持的协议: %v\n", provider.SupportedProtocols())

	// 示例1: 使用OpenAI协议调用Anthropic后端
	fmt.Println("\n📝 示例1: OpenAI协议 → Anthropic后端")
	openaiRequest := `{
		"model": "claude-3-opus-20240229",
		"messages": [
			{"role": "user", "content": "Hello, how are you?"}
		],
		"max_tokens": 100,
		"temperature": 0.7
	}`

	req, err := provider.BuildRequest(context.Background(), "openai", []byte(openaiRequest))
	if err != nil {
		log.Printf("构建OpenAI请求失败: %v", err)
	} else {
		fmt.Printf("✅ 成功构建OpenAI协议请求，目标URL: %s\n", req.URL.String())
	}

	// 示例2: 使用原生Anthropic协议
	fmt.Println("\n📝 示例2: Anthropic协议 → Anthropic后端")
	anthropicRequest := `{
		"model": "claude-3-opus-20240229",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Hello, how are you?"}]}
		],
		"max_tokens": 100,
		"temperature": 0.7
	}`

	req2, err := provider.BuildRequest(context.Background(), "anthropic", []byte(anthropicRequest))
	if err != nil {
		log.Printf("构建Anthropic请求失败: %v", err)
	} else {
		fmt.Printf("✅ 成功构建Anthropic协议请求，目标URL: %s\n", req2.URL.String())
	}

	fmt.Println("\n🎉 这就是统一Provider的威力：")
	fmt.Println("   - 用户只需要一个Anthropic API Key")
	fmt.Println("   - 自动支持OpenAI和Anthropic两种协议")
	fmt.Println("   - 客户端可以使用任何协议，后端统一处理")
	fmt.Println("   - 真正的协议无关化！")
}
