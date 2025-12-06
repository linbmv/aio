#!/bin/bash

SERVER="https://llmio.150129.xyz"
TOKEN="sk-LinHome-wo20Fang13145204eVer"

echo "=========================================="
echo "诊断：查看实际配置"
echo "=========================================="
echo ""

echo "1. 所有 Provider"
echo "------------------------------------------"
curl -s "$SERVER/api/providers" -H "Authorization: Bearer $TOKEN" | jq -r '.data[] | "名称: \(.name) | 类型: \(.type) | 启用: \(.enabled)"'
echo ""

echo "2. 所有模型及其 Provider 映射"
echo "------------------------------------------"
curl -s "$SERVER/api/models" -H "Authorization: Bearer $TOKEN" | jq -r '.data[] | "模型: \(.name) | Providers: \(.providers | join(", "))"'
echo ""

echo "3. 测试实际可用的模型"
echo "------------------------------------------"
echo "请从上面的列表中选择一个已配置的模型进行测试"
echo ""
