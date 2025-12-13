package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("🎯 统一Provider使用场景演示")
	fmt.Println("=====================================")

	// 用户配置：只需要一个Anthropic渠道
	fmt.Println("📋 用户配置（一次性）：")
	fmt.Println(`{
  "name": "我的万能渠道",
  "type": "anthropic",
  "keys": ["sk-ant-key1", "sk-ant-key2", "sk-ant-key3"],
  "protocols": ["anthropic", "openai"]
}`)

	fmt.Println("\n🔄 系统自动支持的调用场景：")

	// 场景1：Chat客户端调用（OpenAI协议）
	fmt.Println("\n1️⃣ Chat客户端调用 /v1/chat/completions")
	fmt.Println("   请求协议: OpenAI")
	fmt.Println("   系统处理: OpenAI格式 → 转换 → Anthropic API")
	fmt.Println("   使用Key: sk-ant-key1 (轮询选择)")
	fmt.Println("   ✅ 成功响应OpenAI格式")

	// 场景2：Claude Code调用（Anthropic协议）
	fmt.Println("\n2️⃣ Claude Code调用 /v1/messages")
	fmt.Println("   请求协议: Anthropic")
	fmt.Println("   系统处理: Anthropic格式 → 直接调用 → Anthropic API")
	fmt.Println("   使用Key: sk-ant-key2 (轮询选择)")
	fmt.Println("   ✅ 成功响应Anthropic格式")

	// 场景3：第三方工具调用
	fmt.Println("\n3️⃣ 第三方工具调用任意协议")
	fmt.Println("   请求协议: 任意支持的协议")
	fmt.Println("   系统处理: 自动适配 → Anthropic API")
	fmt.Println("   使用Key: sk-ant-key3 (轮询选择)")
	fmt.Println("   ✅ 成功响应对应格式")

	fmt.Println("\n🎉 核心价值：")
	fmt.Println("   ✅ 用户只需要一个供应商的API Key")
	fmt.Println("   ✅ 支持所有协议的客户端调用")
	fmt.Println("   ✅ 自动负载均衡和容错")
	fmt.Println("   ✅ 真正的协议无关化")

	fmt.Println("\n💰 成本对比：")
	fmt.Println("   传统方案: OpenAI Key + Anthropic Key = 双重成本")
	fmt.Println("   统一方案: 只需要 Anthropic Key = 50% 成本节省")

	fmt.Println("\n🚀 这就是统一Provider的革命性价值！")
}