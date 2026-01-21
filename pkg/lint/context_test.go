package lint_test

import (
	"context"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

const defaultTestValue = "default"

func TestNewRuleContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	file := &mdast.FileSnapshot{
		Path:    "test.md",
		Content: []byte("# Hello"),
		Root:    mdast.NewNode(mdast.NodeDocument),
	}
	cfg := config.NewConfig()
	ruleCfg := &config.RuleConfig{
		Options: map[string]any{"key": "value"},
	}

	rc := lint.NewRuleContext(ctx, file, cfg, ruleCfg)

	if rc.Ctx != ctx {
		t.Error("Ctx mismatch")
	}
	if rc.File != file {
		t.Error("File mismatch")
	}
	if rc.Root != file.Root {
		t.Error("Root should equal File.Root")
	}
	if rc.Config != cfg {
		t.Error("Config mismatch")
	}
	if rc.RuleConfig != ruleCfg {
		t.Error("RuleConfig mismatch")
	}
	if rc.Builder == nil {
		t.Error("Builder should be initialized")
	}
}

func TestNewRuleContext_NilFile(t *testing.T) {
	t.Parallel()

	rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

	if rc.File != nil {
		t.Error("File should be nil")
	}
	if rc.Root != nil {
		t.Error("Root should be nil when File is nil")
	}
}

func TestRuleContext_Cancelled(t *testing.T) {
	t.Parallel()

	t.Run("not cancelled", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

		if rc.Cancelled() {
			t.Error("should not be cancelled")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		rc := lint.NewRuleContext(ctx, nil, nil, nil)

		if !rc.Cancelled() {
			t.Error("should be cancelled")
		}
	})
}

func TestRuleContext_Option(t *testing.T) {
	t.Parallel()

	t.Run("returns default when RuleConfig is nil", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, nil)

		result := rc.Option("key", defaultTestValue)
		if result != defaultTestValue {
			t.Errorf("got %v, want %s", result, defaultTestValue)
		}
	})

	t.Run("returns default when Options is nil", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, &config.RuleConfig{})

		result := rc.Option("key", defaultTestValue)
		if result != defaultTestValue {
			t.Errorf("got %v, want %s", result, defaultTestValue)
		}
	})

	t.Run("returns default when key not found", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, &config.RuleConfig{
			Options: map[string]any{"other": "value"},
		})

		result := rc.Option("key", defaultTestValue)
		if result != defaultTestValue {
			t.Errorf("got %v, want %s", result, defaultTestValue)
		}
	})

	t.Run("returns value when found", func(t *testing.T) {
		t.Parallel()

		rc := lint.NewRuleContext(context.Background(), nil, nil, &config.RuleConfig{
			Options: map[string]any{"key": "found"},
		})

		result := rc.Option("key", "default")
		if result != "found" {
			t.Errorf("got %v, want found", result)
		}
	})
}

func TestRuleContext_OptionInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		key     string
		def     int
		want    int
	}{
		{
			name:    "returns default when nil options",
			options: nil,
			key:     "max",
			def:     100,
			want:    100,
		},
		{
			name:    "returns int value",
			options: map[string]any{"max": 50},
			key:     "max",
			def:     100,
			want:    50,
		},
		{
			name:    "converts float64 to int",
			options: map[string]any{"max": float64(75)},
			key:     "max",
			def:     100,
			want:    75,
		},
		{
			name:    "returns default for wrong type",
			options: map[string]any{"max": "not an int"},
			key:     "max",
			def:     100,
			want:    100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}

			rc := lint.NewRuleContext(context.Background(), nil, nil, ruleCfg)
			got := rc.OptionInt(tt.key, tt.def)

			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRuleContext_OptionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		key     string
		def     string
		want    string
	}{
		{
			name:    "returns default when nil options",
			options: nil,
			key:     "style",
			def:     "default",
			want:    "default",
		},
		{
			name:    "returns string value",
			options: map[string]any{"style": "custom"},
			key:     "style",
			def:     "default",
			want:    "custom",
		},
		{
			name:    "returns default for wrong type",
			options: map[string]any{"style": 123},
			key:     "style",
			def:     "default",
			want:    "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}

			rc := lint.NewRuleContext(context.Background(), nil, nil, ruleCfg)
			got := rc.OptionString(tt.key, tt.def)

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuleContext_OptionBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		key     string
		def     bool
		want    bool
	}{
		{
			name:    "returns default when nil options",
			options: nil,
			key:     "enabled",
			def:     true,
			want:    true,
		},
		{
			name:    "returns bool value true",
			options: map[string]any{"enabled": true},
			key:     "enabled",
			def:     false,
			want:    true,
		},
		{
			name:    "returns bool value false",
			options: map[string]any{"enabled": false},
			key:     "enabled",
			def:     true,
			want:    false,
		},
		{
			name:    "returns default for wrong type",
			options: map[string]any{"enabled": "yes"},
			key:     "enabled",
			def:     true,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}

			rc := lint.NewRuleContext(context.Background(), nil, nil, ruleCfg)
			got := rc.OptionBool(tt.key, tt.def)

			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuleContext_HasRegistry(t *testing.T) {
	t.Parallel()

	reg := lint.NewRegistry()
	ctx := &lint.RuleContext{
		Registry: reg,
	}

	if ctx.Registry == nil {
		t.Error("Registry should not be nil")
	}
	if ctx.Registry != reg {
		t.Error("Registry should be the same instance")
	}
}
