#!/bin/bash
# 应用 Codex 的修复方案

# 1. 添加 normalizeProviderStyle 函数
sed -i '16a\n// normalizeProviderStyle 将 openai-res Provider 视为 openai，避免把客户端格式误当 Provider 类型\nfunc normalizeProviderStyle(style string) string {\n\tif style == consts.StyleOpenAIRes {\n\t\treturn consts.StyleOpenAI\n\t}\n\treturn style\n}' service/formatx/converter.go

# 2. 修改 ConvertRequest
sed -i 's/func ConvertRequest(raw \[\]byte, from, to string) ([]byte, error) {/func ConvertRequest(raw []byte, from, to string) ([]byte, error) {\n\tprovider := normalizeProviderStyle(to)/' service/formatx/converter.go
sed -i 's/if from == to {/if from == provider {/' service/formatx/converter.go

# 3. 修改 ConvertResponse  
sed -i '/func ConvertResponse/,/if from == to {/s/if from == to {/provider := normalizeProviderStyle(from)\n\tif provider == to {/' service/formatx/converter.go

# 4. 修改 ConvertStream
sed -i '/func ConvertStream/,/streamReader := r/s/streamReader := r/provider := normalizeProviderStyle(from)\n\n\tstreamReader := r/' service/formatx/converter.go

echo "修复已应用"
