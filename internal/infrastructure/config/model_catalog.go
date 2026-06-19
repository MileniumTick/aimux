package config

import (
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
)

// KnownModelCatalog maps model name prefixes to their metadata.
// Used as a fallback when the provider API doesn't return full model capabilities.
// ponytail: hardcoded catalog covers major providers; add API-level metadata
// parsing or a user-editable catalog if this grows beyond ~50 entries.
var KnownModelCatalog = map[string]domain.ModelMetadata{
	// ── DeepSeek V4 family ──────────────────────────────────────────────
	// All support 1M context, reasoning via DeepSeek's own thinking params.
	"deepseek-v4-pro": {
		domain.MetaContextWindow:    1_000_000,
		domain.MetaMaxTokens:        384_000,
		domain.MetaReasoning:        true,
		domain.MetaInputModalities:  []any{"text"},
		domain.MetaCost:             map[string]any{"input": 0.435, "output": 0.87, "cacheRead": 0.003625, "cacheWrite": 0.435},
		domain.MetaThinkingLevelMap: map[string]any{"minimal": nil, "low": nil, "medium": nil, "high": "high", "xhigh": "max"},
		domain.MetaCompat:           map[string]any{"supportsDeveloperRole": false, "supportsReasoningEffort": false},
	},
	"deepseek-v4-flash": {
		domain.MetaContextWindow:    1_000_000,
		domain.MetaMaxTokens:        384_000,
		domain.MetaReasoning:        true,
		domain.MetaInputModalities:  []any{"text"},
		domain.MetaCost:             map[string]any{"input": 0.14, "output": 0.28, "cacheRead": 0.0028, "cacheWrite": 0.14},
		domain.MetaThinkingLevelMap: map[string]any{"minimal": nil, "low": nil, "medium": nil, "high": "high", "xhigh": "max"},
		domain.MetaCompat:           map[string]any{"supportsDeveloperRole": false, "supportsReasoningEffort": false},
	},
	"deepseek-v4-lite": {
		domain.MetaContextWindow:    1_000_000,
		domain.MetaMaxTokens:        384_000,
		domain.MetaReasoning:        true,
		domain.MetaInputModalities:  []any{"text"},
		domain.MetaCost:             map[string]any{"input": 0.15, "output": 0.6, "cacheRead": 0.03, "cacheWrite": 0.15},
		domain.MetaThinkingLevelMap: map[string]any{"minimal": nil, "low": nil, "medium": nil, "high": "high", "xhigh": "max"},
		domain.MetaCompat:           map[string]any{"supportsDeveloperRole": false, "supportsReasoningEffort": false},
	},

	// ── DeepSeek V3 family ─────────────────────────────────────────────
	"deepseek-v3": {
		domain.MetaContextWindow:   128_000,
		domain.MetaMaxTokens:       8_192,
		domain.MetaReasoning:       false,
		domain.MetaInputModalities: []any{"text"},
		domain.MetaCost:            map[string]any{"input": 0.5, "output": 2.0, "cacheRead": 0.1, "cacheWrite": 0.5},
	},
	"deepseek-r1": {
		domain.MetaContextWindow:   128_000,
		domain.MetaMaxTokens:       8_192,
		domain.MetaReasoning:       true,
		domain.MetaInputModalities: []any{"text"},
		domain.MetaCost:            map[string]any{"input": 0.55, "output": 2.19, "cacheRead": 0.11, "cacheWrite": 0.55},
	},

	// ── Claude 4 family — 200K context ─────────────────────────────────
	"claude-sonnet-4": {
		domain.MetaContextWindow:   200_000,
		domain.MetaMaxTokens:       64_000,
		domain.MetaReasoning:       true,
		domain.MetaInputModalities: []any{"text", "image"},
		domain.MetaCost:            map[string]any{"input": 3.0, "output": 15.0, "cacheRead": 0.3, "cacheWrite": 3.0},
	},
	"claude-opus-4": {
		domain.MetaContextWindow:   200_000,
		domain.MetaMaxTokens:       64_000,
		domain.MetaReasoning:       true,
		domain.MetaInputModalities: []any{"text", "image"},
		domain.MetaCost:            map[string]any{"input": 15.0, "output": 75.0, "cacheRead": 1.5, "cacheWrite": 15.0},
	},
	"claude-haiku-4": {
		domain.MetaContextWindow:   200_000,
		domain.MetaMaxTokens:       64_000,
		domain.MetaReasoning:       false,
		domain.MetaInputModalities: []any{"text", "image"},
		domain.MetaCost:            map[string]any{"input": 0.8, "output": 4.0, "cacheRead": 0.08, "cacheWrite": 0.8},
	},

	// ── Claude 3.5 family ──────────────────────────────────────────────
	"claude-sonnet-3": {
		domain.MetaContextWindow: 200_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
		domain.MetaCost:          map[string]any{"input": 3.0, "output": 15.0, "cacheRead": 0.3, "cacheWrite": 3.0},
	},
	"claude-haiku-3": {
		domain.MetaContextWindow: 200_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
		domain.MetaCost:          map[string]any{"input": 0.25, "output": 1.25, "cacheRead": 0.03, "cacheWrite": 0.25},
	},

	// ── GPT-4o family ──────────────────────────────────────────────────
	"gpt-4o": {
		domain.MetaContextWindow:   128_000,
		domain.MetaMaxTokens:       16_384,
		domain.MetaReasoning:       true,
		domain.MetaInputModalities: []any{"text", "image"},
		domain.MetaCost:            map[string]any{"input": 2.5, "output": 10.0, "cacheRead": 1.25, "cacheWrite": 2.5},
	},
	"gpt-4o-mini": {
		domain.MetaContextWindow:   128_000,
		domain.MetaMaxTokens:       16_384,
		domain.MetaReasoning:       false,
		domain.MetaInputModalities: []any{"text", "image"},
		domain.MetaCost:            map[string]any{"input": 0.15, "output": 0.6, "cacheRead": 0.075, "cacheWrite": 0.15},
	},
	"gpt-4-turbo": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     4_096,
		domain.MetaReasoning:     false,
		domain.MetaCost:          map[string]any{"input": 10.0, "output": 30.0, "cacheRead": 10.0, "cacheWrite": 10.0},
	},

	// ── o-series reasoning ─────────────────────────────────────────────
	"o1": {
		domain.MetaContextWindow: 200_000,
		domain.MetaMaxTokens:     100_000,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 15.0, "output": 60.0, "cacheRead": 7.5, "cacheWrite": 15.0},
	},
	"o1-mini": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     65_536,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 1.1, "output": 4.4, "cacheRead": 0.55, "cacheWrite": 1.1},
	},
	"o3": {
		domain.MetaContextWindow: 200_000,
		domain.MetaMaxTokens:     100_000,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 10.0, "output": 40.0, "cacheRead": 5.0, "cacheWrite": 10.0},
	},
	"o3-mini": {
		domain.MetaContextWindow: 200_000,
		domain.MetaMaxTokens:     100_000,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 1.1, "output": 4.4, "cacheRead": 0.55, "cacheWrite": 1.1},
	},
	"o4-mini": {
		domain.MetaContextWindow: 200_000,
		domain.MetaMaxTokens:     100_000,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 1.1, "output": 4.4, "cacheRead": 0.55, "cacheWrite": 1.1},
	},

	// ── Gemini family ──────────────────────────────────────────────────
	"gemini-2.5-pro": {
		domain.MetaContextWindow: 1_000_000,
		domain.MetaMaxTokens:     65_536,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 1.25, "output": 5.0, "cacheRead": 0.06, "cacheWrite": 0.6},
	},
	"gemini-2.5-flash": {
		domain.MetaContextWindow: 1_000_000,
		domain.MetaMaxTokens:     65_536,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 0.15, "output": 0.6, "cacheRead": 0.01, "cacheWrite": 0.15},
	},
	"gemini-2.0-flash": {
		domain.MetaContextWindow: 1_000_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
		domain.MetaCost:          map[string]any{"input": 0.10, "output": 0.40, "cacheRead": 0.01, "cacheWrite": 0.10},
	},

	// ── GLM family (Zhipu AI) ──────────────────────────────────────────
	"glm-5": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     4_096,
		domain.MetaReasoning:     false,
	},
	"glm-5.1": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"glm-5.2": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},

	// ── Hunyuan (Tencent) ──────────────────────────────────────────────
	"hy3-preview": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},

	// ── Kimi (Moonshot AI) ─────────────────────────────────────────────
	"kimi-k2.5": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     16_384,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 0.6, "output": 2.4, "cacheRead": 0.3, "cacheWrite": 0.6},
	},
	"kimi-k2.6": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     16_384,
		domain.MetaReasoning:     true,
		domain.MetaCost:          map[string]any{"input": 0.6, "output": 2.4, "cacheRead": 0.3, "cacheWrite": 0.6},
	},
	"kimi-k2.7-code": {
		domain.MetaContextWindow:   128_000,
		domain.MetaMaxTokens:       16_384,
		domain.MetaReasoning:       true,
		domain.MetaInputModalities: []any{"text"},
		domain.MetaCost:            map[string]any{"input": 0.6, "output": 2.4, "cacheRead": 0.3, "cacheWrite": 0.6},
	},

	// ── Mimo family ────────────────────────────────────────────────────
	"mimo-v2-omni": {
		domain.MetaContextWindow:   128_000,
		domain.MetaMaxTokens:       8_192,
		domain.MetaReasoning:       false,
		domain.MetaInputModalities: []any{"text", "image"},
	},
	"mimo-v2-pro": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"mimo-v2.5": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"mimo-v2.5-pro": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     true,
	},

	// ── MiniMax family ─────────────────────────────────────────────────
	"minimax-m2.5": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"minimax-m2.7": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"minimax-m3": {
		domain.MetaContextWindow:    1_000_000,
		domain.MetaMaxTokens:        16_384,
		domain.MetaReasoning:        true,
		domain.MetaThinkingLevelMap: map[string]any{"minimal": nil, "low": nil, "medium": nil, "high": "high", "xhigh": "max"},
		domain.MetaCompat:           map[string]any{"supportsDeveloperRole": false, "supportsReasoningEffort": false},
	},

	// ── Qwen family (Alibaba) ──────────────────────────────────────────
	"qwen3.5-plus": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"qwen3.6-plus": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
	"qwen3.7-max": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     true,
	},
	"qwen3.7-plus": {
		domain.MetaContextWindow: 128_000,
		domain.MetaMaxTokens:     8_192,
		domain.MetaReasoning:     false,
	},
}

// ContextSuffixForWindow returns a context window indicator suffix
// for Claude Code-style model identifiers (e.g. "[1m]", "[200k]", "[32k]").
func ContextSuffixForWindow(cw int64) string {
	switch {
	case cw >= 1_000_000:
		return "[1m]"
	case cw >= 200_000:
		return "[200k]"
	case cw >= 128_000:
		return "[128k]"
	case cw >= 32_000:
		return "[32k]"
	default:
		return ""
	}
}

// LookupContextSuffix finds the context suffix for a model from its metadata.
// If metadata has context_suffix, use it. Otherwise derive from context_window.
func LookupContextSuffix(md domain.ModelMetadata) string {
	if s, ok := md[domain.MetaContextSuffix].(string); ok && s != "" {
		return s
	}
	if cw, ok := md[domain.MetaContextWindow].(int64); ok && cw > 0 {
		return ContextSuffixForWindow(cw)
	}
	// float64 from JSON unmarshalling
	if cw, ok := md[domain.MetaContextWindow].(float64); ok && cw > 0 {
		return ContextSuffixForWindow(int64(cw))
	}
	return ""
}

// StripProviderPrefix strips a Bifrost-style Provider/ prefix from a model name.
func StripProviderPrefix(modelName string) string {
	idx := strings.IndexByte(modelName, '/')
	if idx > 0 && idx < len(modelName)-1 {
		return modelName[idx+1:]
	}
	return modelName
}

// LookupModelMetadata finds metadata for a model name.
// Tries exact match first, then prefix match (longest wins), then empty.
// Auto-derives context_suffix from context_window if not already set.
func LookupModelMetadata(modelName string) domain.ModelMetadata {
	var md domain.ModelMetadata

	if m, ok := KnownModelCatalog[modelName]; ok {
		md = cloneMetadata(m)
	} else {
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
			md = cloneMetadata(best)
		} else {
			md = domain.ModelMetadata{}
		}
	}

	// Auto-derive context_suffix if not set and context_window is available
	if _, has := md[domain.MetaContextSuffix]; !has {
		if suffix := LookupContextSuffix(md); suffix != "" {
			md[domain.MetaContextSuffix] = suffix
		}
	}

	return md
}

// cloneMetadata deep-copies a ModelMetadata.
func cloneMetadata(src domain.ModelMetadata) domain.ModelMetadata {
	cp := make(domain.ModelMetadata, len(src))
	for k, v := range src {
		cp[k] = v
	}
	return cp
}

// ApplyModelOverrides applies user overrides on top of catalog metadata.
// overrides is a map of modelID -> {key: value} — merges, does not replace.
func ApplyModelOverrides(base domain.ModelMetadata, overrides map[string]any) domain.ModelMetadata {
	if len(overrides) == 0 {
		return base
	}
	// If base is nil, start fresh
	result := base
	if result == nil {
		result = make(domain.ModelMetadata)
	}
	for k, v := range overrides {
		// ponytail: simple merge, no deep merge for nested maps yet
		result[k] = v
	}
	return result
}

// FormatCost returns a human-readable cost string for display.
// ponytail: simple formatting, tolerates missing fields.
func FormatCost(cost any) string {
	m, ok := cost.(map[string]any)
	if !ok {
		return ""
	}
	input, _ := m["input"].(float64)
	output, _ := m["output"].(float64)
	if input == 0 && output == 0 {
		return ""
	}
	return fmt.Sprintf("$%.2f/$%.2f/M", input, output)
}
