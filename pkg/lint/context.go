package lint

import (
	"context"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint/refs"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// RuleContext provides all context needed by a rule to perform linting.
//
// Design note: RuleContext stores context.Context as a field (Ctx) rather than
// passing it as a method parameter. This is acceptable because RuleContext is
// a short-lived parameter object created per-rule-invocation, not a long-lived
// struct. This design simplifies the Rule interface (single Apply method) while
// still providing cancellation support via the Cancelled() helper.
type RuleContext struct {
	// Ctx is the context for cancellation and timeouts.
	Ctx context.Context

	// File is the parsed FileSnapshot.
	File *mdast.FileSnapshot

	// Root is the AST root node (convenience alias for File.Root).
	Root *mdast.Node

	// Config is the resolved configuration.
	Config *config.Config

	// RuleConfig is the rule-specific configuration (may be nil).
	RuleConfig *config.RuleConfig

	// Builder accumulates text edits for auto-fix.
	Builder *fix.EditBuilder

	// Registry provides access to the rule registry for name lookups.
	Registry *Registry

	// refCtx is the cached reference context, lazily initialized.
	refCtx *refs.Context
}

// NewRuleContext creates a RuleContext for the given file and configuration.
func NewRuleContext(
	ctx context.Context,
	file *mdast.FileSnapshot,
	cfg *config.Config,
	ruleCfg *config.RuleConfig,
) *RuleContext {
	var root *mdast.Node
	if file != nil {
		root = file.Root
	}

	return &RuleContext{
		Ctx:        ctx,
		File:       file,
		Root:       root,
		Config:     cfg,
		RuleConfig: ruleCfg,
		Builder:    fix.NewEditBuilder(),
	}
}

// Cancelled returns true if the context has been cancelled.
func (rc *RuleContext) Cancelled() bool {
	select {
	case <-rc.Ctx.Done():
		return true
	default:
		return false
	}
}

// Option returns a rule-specific option value, or the default if not set.
func (rc *RuleContext) Option(key string, defaultValue any) any {
	if rc.RuleConfig == nil || rc.RuleConfig.Options == nil {
		return defaultValue
	}
	if v, ok := rc.RuleConfig.Options[key]; ok {
		return v
	}
	return defaultValue
}

// OptionInt returns a rule-specific integer option, or the default.
func (rc *RuleContext) OptionInt(key string, defaultValue int) int {
	v := rc.Option(key, defaultValue)
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return defaultValue
	}
}

// OptionString returns a rule-specific string option, or the default.
func (rc *RuleContext) OptionString(key string, defaultValue string) string {
	v := rc.Option(key, defaultValue)
	if s, ok := v.(string); ok {
		return s
	}
	return defaultValue
}

// OptionBool returns a rule-specific boolean option, or the default.
func (rc *RuleContext) OptionBool(key string, defaultValue bool) bool {
	v := rc.Option(key, defaultValue)
	if b, ok := v.(bool); ok {
		return b
	}
	return defaultValue
}

// OptionStringSlice returns a rule-specific string slice option, or the default.
func (rc *RuleContext) OptionStringSlice(key string, defaultValue []string) []string {
	v := rc.Option(key, defaultValue)
	if slice, ok := v.([]string); ok {
		return slice
	}
	// Handle []interface{} from YAML/JSON parsing
	if iface, ok := v.([]interface{}); ok {
		result := make([]string, 0, len(iface))
		for _, item := range iface {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// RefContext returns the reference context for this file, building it lazily.
// The reference context contains all link/image usages, reference definitions,
// and document anchors needed by reference-tracking rules (MD051-MD054).
func (rc *RuleContext) RefContext() *refs.Context {
	if rc.refCtx == nil {
		rc.refCtx = refs.Collect(rc.Root, rc.File)
	}
	return rc.refCtx
}
