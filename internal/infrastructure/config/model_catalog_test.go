package config

import (
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

// ── ContextSuffixForWindow ────────────────────────────────────────────────

func TestContextSuffixForWindow(t *testing.T) {
	tests := []struct {
		name string
		cw   int64
		want string
	}{
		{name: "1M", cw: 1_000_000, want: "[1m]"},
		{name: "over 1M", cw: 2_000_000, want: "[1m]"},
		{name: "200K", cw: 200_000, want: "[200k]"},
		{name: "199K", cw: 199_999, want: "[128k]"},
		{name: "128K", cw: 128_000, want: "[128k]"},
		{name: "127K", cw: 127_999, want: "[32k]"},
		{name: "32K", cw: 32_000, want: "[32k]"},
		{name: "31K", cw: 31_999, want: ""},
		{name: "zero", cw: 0, want: ""},
		{name: "negative", cw: -1, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContextSuffixForWindow(tt.cw)
			if got != tt.want {
				t.Errorf("ContextSuffixForWindow(%d) = %q, want %q", tt.cw, got, tt.want)
			}
		})
	}
}

// ── LookupContextSuffix ───────────────────────────────────────────────────

func TestLookupContextSuffix(t *testing.T) {
	tests := []struct {
		name string
		md   domain.ModelMetadata
		want string
	}{
		{
			name: "explicit suffix",
			md:   domain.ModelMetadata{domain.MetaContextSuffix: "[custom]"},
			want: "[custom]",
		},
		{
			name: "empty suffix ignored",
			md:   domain.ModelMetadata{domain.MetaContextSuffix: "", domain.MetaContextWindow: int64(1_000_000)},
			want: "[1m]",
		},
		{
			name: "int64 context_window",
			md:   domain.ModelMetadata{domain.MetaContextWindow: int64(200_000)},
			want: "[200k]",
		},
		{
			name: "float64 context_window",
			md:   domain.ModelMetadata{domain.MetaContextWindow: float64(128_000)},
			want: "[128k]",
		},
		{
			name: "zero context_window",
			md:   domain.ModelMetadata{domain.MetaContextWindow: int64(0)},
			want: "",
		},
		{
			name: "negative context_window",
			md:   domain.ModelMetadata{domain.MetaContextWindow: int64(-1)},
			want: "",
		},
		{
			name: "no context info",
			md:   domain.ModelMetadata{},
			want: "",
		},
		{
			name: "wrong type for context_window",
			md:   domain.ModelMetadata{domain.MetaContextWindow: "one_million"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LookupContextSuffix(tt.md)
			if got != tt.want {
				t.Errorf("LookupContextSuffix(%v) = %q, want %q", tt.md, got, tt.want)
			}
		})
	}
}

// ── StripProviderPrefix ───────────────────────────────────────────────────

func TestStripProviderPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "provider/model", in: "deepseek/model-v4", want: "model-v4"},
		{name: "no slash", in: "model-v4", want: "model-v4"},
		{name: "empty", in: "", want: ""},
		{name: "leading slash", in: "/model", want: "/model"},
		{name: "trailing slash", in: "provider/", want: "provider/"},
		{name: "just slash", in: "/", want: "/"},
		{name: "multiple slashes", in: "a/b/c", want: "b/c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripProviderPrefix(tt.in)
			if got != tt.want {
				t.Errorf("StripProviderPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ── LookupModelMetadata ───────────────────────────────────────────────────

func TestLookupModelMetadata_ExactMatch(t *testing.T) {
	md := LookupModelMetadata("deepseek-v4-flash")
	if md == nil {
		t.Fatal("LookupModelMetadata returned nil")
	}
	cw, ok := md[domain.MetaContextWindow]
	if !ok {
		t.Fatal("expected MetaContextWindow in returned metadata")
	}
	// Catalog stores int literals (not int64) — compare as int
	cwInt, ok := cw.(int)
	if !ok {
		t.Fatalf("MetaContextWindow type = %T, want int", cw)
	}
	if cwInt != 1_000_000 {
		t.Errorf("MetaContextWindow = %d, want 1000000", cwInt)
	}
	// Check another known field
	cost, ok := md[domain.MetaCost]
	if !ok {
		t.Fatal("expected MetaCost in returned metadata")
	}
	costMap, ok := cost.(map[string]any)
	if !ok {
		t.Fatalf("MetaCost type = %T, want map[string]any", cost)
	}
	if costMap["input"] != 0.14 {
		t.Errorf("expected flash input cost 0.14, got %v", costMap["input"])
	}
}

func TestLookupModelMetadata_PrefixMatch(t *testing.T) {
	md := LookupModelMetadata("deepseek-v4-flash-somevariant")
	if md == nil {
		t.Fatal("LookupModelMetadata returned nil")
	}
	cw, ok := md[domain.MetaContextWindow]
	if !ok {
		t.Fatal("expected MetaContextWindow in prefix-matched metadata")
	}
	cwInt, ok := cw.(int)
	if !ok {
		t.Fatalf("MetaContextWindow type = %T, want int", cw)
	}
	if cwInt != 1_000_000 {
		t.Errorf("MetaContextWindow = %d, want 1000000", cwInt)
	}
}

func TestLookupModelMetadata_LongestPrefixWins(t *testing.T) {
	// "deepseek-v4-flash-plus" should match "deepseek-v4-flash" (len 17) over
	// "deepseek-v4-pro" (len 15) or "deepseek-v4-lite" (len 15)
	md := LookupModelMetadata("deepseek-v4-flash-plus")
	cw := md[domain.MetaContextWindow]
	cwInt, ok := cw.(int)
	if !ok {
		t.Fatalf("MetaContextWindow type = %T, want int", cw)
	}
	if cwInt != 1_000_000 {
		t.Errorf("expected v4-flash match (cost 0.14), got context_window=%v", cwInt)
	}
	// Verify it's truly the flash metadata by checking cost
	cost, ok := md[domain.MetaCost]
	if !ok {
		t.Fatal("expected MetaCost in returned metadata")
	}
	costMap, ok := cost.(map[string]any)
	if !ok {
		t.Fatalf("MetaCost is not a map: %T", cost)
	}
	inputCost, _ := costMap["input"].(float64)
	if inputCost != 0.14 {
		t.Errorf("expected flash input cost 0.14, got %v", inputCost)
	}
}

func TestLookupModelMetadata_NoMatch(t *testing.T) {
	md := LookupModelMetadata("nonexistent-model-name-xyzzy")
	if md == nil {
		t.Fatal("LookupModelMetadata returned nil for no match, expected empty map")
	}
	if len(md) != 0 {
		t.Errorf("expected empty metadata, got %v (len=%d)", md, len(md))
	}
}

func TestLookupModelMetadata_EmptyName(t *testing.T) {
	md := LookupModelMetadata("")
	if md == nil {
		t.Fatal("LookupModelMetadata returned nil for empty name")
	}
	if len(md) != 0 {
		t.Errorf("expected empty metadata for empty name, got len=%d", len(md))
	}
}

// ── FormatCost ────────────────────────────────────────────────────────────

func TestFormatCost(t *testing.T) {
	tests := []struct {
		name string
		cost any
		want string
	}{
		{
			name: "valid cost map",
			cost: map[string]any{"input": 3.0, "output": 15.0},
			want: "$3.00/$15.00/M",
		},
		{
			name: "small values",
			cost: map[string]any{"input": 0.15, "output": 0.6},
			want: "$0.15/$0.60/M",
		},
		{
			name: "zero values",
			cost: map[string]any{"input": 0.0, "output": 0.0},
			want: "",
		},
		{
			name: "missing output key",
			cost: map[string]any{"input": 1.0},
			want: "$1.00/$0.00/M",
		},
		{
			name: "missing input key",
			cost: map[string]any{"output": 2.0},
			want: "$0.00/$2.00/M",
		},
		{
			name: "wrong type",
			cost: "not a map",
			want: "",
		},
		{
			name: "nil cost",
			cost: nil,
			want: "",
		},
		{
			name: "wrong map value type",
			cost: map[string]any{"input": "three", "output": "fifteen"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCost(tt.cost)
			if got != tt.want {
				t.Errorf("FormatCost(%v) = %q, want %q", tt.cost, got, tt.want)
			}
		})
	}
}

// ── cloneMetadata ─────────────────────────────────────────────────────────

func TestCloneMetadata(t *testing.T) {
	t.Run("shallow copy independence", func(t *testing.T) {
		src := domain.ModelMetadata{
			"key1": "value1",
			"key2": int64(42),
		}
		cp := cloneMetadata(src)
		// Modify clone — original should be untouched
		cp["key1"] = "modified"
		if src["key1"] != "value1" {
			t.Errorf("original was mutated: src['key1'] = %v, want 'value1'", src["key1"])
		}
	})

	t.Run("nested map shares reference", func(t *testing.T) {
		nested := map[string]any{"inner": "data"}
		src := domain.ModelMetadata{
			"nested": nested,
		}
		cp := cloneMetadata(src)
		// Mutate nested map through clone
		inner := cp["nested"].(map[string]any)
		inner["inner"] = "modified"
		if src["nested"].(map[string]any)["inner"] != "modified" {
			t.Error("nested map was not shared — clone is not shallow")
		}
	})

	t.Run("empty source", func(t *testing.T) {
		src := domain.ModelMetadata{}
		cp := cloneMetadata(src)
		if len(cp) != 0 {
			t.Errorf("expected empty clone, got len=%d", len(cp))
		}
	})

	t.Run("nil source", func(t *testing.T) {
		cp := cloneMetadata(nil)
		if len(cp) != 0 {
			t.Errorf("expected empty clone from nil, got len=%d", len(cp))
		}
	})
}

// ── Catalog completeness ──────────────────────────────────────────────────

func TestKnownModelCatalog_Completeness(t *testing.T) {
	for prefix, md := range KnownModelCatalog {
		t.Run(prefix, func(t *testing.T) {
			if _, ok := md[domain.MetaContextWindow]; !ok {
				t.Errorf("entry %q is missing MetaContextWindow", prefix)
			}
			if _, ok := md[domain.MetaMaxTokens]; !ok {
				t.Errorf("entry %q is missing MetaMaxTokens", prefix)
			}
		})
	}
}

func TestKnownModelCatalog_HasEntries(t *testing.T) {
	if len(KnownModelCatalog) == 0 {
		t.Fatal("KnownModelCatalog is empty")
	}
}
