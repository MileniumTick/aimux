package config

import (
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
)

// KnownModelCatalog maps model name prefixes to their metadata.
// Used as a fallback when the provider API doesn't return full model capabilities.
// ponytail: hardcoded catalog covers major providers; add API-level metadata
// parsing or a user-editable catalog if this grows beyond ~50 entries.
var KnownModelCatalog = map[string]domain.ModelMetadata{
	// DeepSeek V4 family — all support 1M context, reasoning
	"deepseek-v4-pro": {
		"context_window":   1_000_000,
		"max_tokens":       384_000,
		"reasoning":        true,
		"input_modalities": []any{"text"},
	},
	"deepseek-v4-flash": {
		"context_window":   1_000_000,
		"max_tokens":       384_000,
		"reasoning":        true,
		"input_modalities": []any{"text"},
	},
	"deepseek-v4-lite": {
		"context_window":   1_000_000,
		"max_tokens":       384_000,
		"reasoning":        true,
		"input_modalities": []any{"text"},
	},

	// DeepSeek V3 family
	"deepseek-v3": {
		"context_window":   128_000,
		"max_tokens":       8_192,
		"reasoning":        false,
		"input_modalities": []any{"text"},
	},
	"deepseek-r1": {
		"context_window":   128_000,
		"max_tokens":       8_192,
		"reasoning":        true,
		"input_modalities": []any{"text"},
	},

	// Claude 4 family — 200K context
	"claude-sonnet-4": {
		"context_window":   200_000,
		"max_tokens":       64_000,
		"reasoning":        true,
		"input_modalities": []any{"text", "image"},
	},
	"claude-opus-4": {
		"context_window":   200_000,
		"max_tokens":       64_000,
		"reasoning":        true,
		"input_modalities": []any{"text", "image"},
	},
	"claude-haiku-4": {
		"context_window":   200_000,
		"max_tokens":       64_000,
		"reasoning":        false,
		"input_modalities": []any{"text", "image"},
	},

	// Claude 3.5 family
	"claude-sonnet-3": {
		"context_window": 200_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"claude-haiku-3": {
		"context_window": 200_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},

	// GPT-4o family
	"gpt-4o": {
		"context_window":   128_000,
		"max_tokens":       16_384,
		"reasoning":        true,
		"input_modalities": []any{"text", "image"},
	},
	"gpt-4o-mini": {
		"context_window":   128_000,
		"max_tokens":       16_384,
		"reasoning":        false,
		"input_modalities": []any{"text", "image"},
	},
	"gpt-4-turbo": {
		"context_window": 128_000,
		"max_tokens":     4_096,
		"reasoning":      false,
	},

	// o-series reasoning models
	"o1": {
		"context_window": 200_000,
		"max_tokens":     100_000,
		"reasoning":      true,
	},
	"o1-mini": {
		"context_window": 128_000,
		"max_tokens":     65_536,
		"reasoning":      true,
	},
	"o3": {
		"context_window": 200_000,
		"max_tokens":     100_000,
		"reasoning":      true,
	},
	"o3-mini": {
		"context_window": 200_000,
		"max_tokens":     100_000,
		"reasoning":      true,
	},
	"o4-mini": {
		"context_window": 200_000,
		"max_tokens":     100_000,
		"reasoning":      true,
	},

	// Gemini family
	"gemini-2.5-pro": {
		"context_window": 1_000_000,
		"max_tokens":     65_536,
		"reasoning":      true,
	},
	"gemini-2.5-flash": {
		"context_window": 1_000_000,
		"max_tokens":     65_536,
		"reasoning":      true,
	},
	"gemini-2.0-flash": {
		"context_window": 1_000_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},

	// GLM family (Zhipu AI)
	"glm-5": {
		"context_window": 128_000,
		"max_tokens":     4_096,
		"reasoning":      false,
	},
	"glm-5.1": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"glm-5.2": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},

	// Hunyuan family (Tencent)
	"hy3-preview": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},

	// Kimi family (Moonshot AI)
	"kimi-k2.5": {
		"context_window": 128_000,
		"max_tokens":     16_384,
		"reasoning":      true,
	},
	"kimi-k2.6": {
		"context_window": 128_000,
		"max_tokens":     16_384,
		"reasoning":      true,
	},
	"kimi-k2.7-code": {
		"context_window":   128_000,
		"max_tokens":       16_384,
		"reasoning":        true,
		"input_modalities": []any{"text"},
	},

	// Mimo family
	"mimo-v2-omni": {
		"context_window":   128_000,
		"max_tokens":       8_192,
		"reasoning":        false,
		"input_modalities": []any{"text", "image"},
	},
	"mimo-v2-pro": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"mimo-v2.5": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"mimo-v2.5-pro": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      true,
	},

	// MiniMax family
	"minimax-m2.5": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"minimax-m2.7": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"minimax-m3": {
		"context_window": 1_000_000,
		"max_tokens":     16_384,
		"reasoning":      true,
	},

	// Qwen family (Alibaba)
	"qwen3.5-plus": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"qwen3.6-plus": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
	"qwen3.7-max": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      true,
	},
	"qwen3.7-plus": {
		"context_window": 128_000,
		"max_tokens":     8_192,
		"reasoning":      false,
	},
}

// StripProviderPrefix strips a Bifrost-style Provider/ prefix from a model name.
// Bifrost returns model IDs as "Opencode/deepseek-v4-flash" when aggregating
// multiple upstreams. Returns the model name unchanged if no slash prefix.
func StripProviderPrefix(modelName string) string {
	idx := strings.IndexByte(modelName, '/')
	if idx > 0 && idx < len(modelName)-1 {
		return modelName[idx+1:]
	}
	return modelName
}

// LookupModelMetadata finds metadata for a model name.
// Tries exact match first, then prefix match (longest wins), then empty.
func LookupModelMetadata(modelName string) domain.ModelMetadata {
	if m, ok := KnownModelCatalog[modelName]; ok {
		// Clone to avoid mutation
		cp := make(domain.ModelMetadata, len(m))
		for k, v := range m {
			cp[k] = v
		}
		return cp
	}
	// Prefix match: "deepseek-v4-pro-20250601" should match "deepseek-v4-pro"
	var best domain.ModelMetadata
	bestLen := 0
	for prefix, m := range KnownModelCatalog {
		if len(prefix) > bestLen && len(prefix) <= len(modelName) &&
			modelName[:len(prefix)] == prefix {
			bestLen = len(prefix)
			best = m
		}
	}
	if best != nil {
		cp := make(domain.ModelMetadata, len(best))
		for k, v := range best {
			cp[k] = v
		}
		return cp
	}
	return domain.ModelMetadata{}
}
