package lint

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

type inlineDirective struct {
	line   int
	offset int
	action string
	rules  []string
}

var directivePattern = regexp.MustCompile(`(?i)<!--\s*markdownlint-(disable-next-line|disable-line|disable-file|enable-file|disable|enable|capture|restore)\b([^<>]*?)-->`)

func collectDirectives(file *mdast.FileSnapshot, registry *Registry) []inlineDirective {
	var directives []inlineDirective
	for _, kind := range []mdast.NodeKind{mdast.NodeHTMLBlock, mdast.NodeHTMLInline} {
		for _, node := range mdast.FindByKind(file.Root, kind) {
			rng := node.SourceRange()
			for _, match := range directivePattern.FindAllSubmatchIndex(node.Text(), -1) {
				content := node.Text()
				offset := rng.StartOffset + match[0]
				line, _ := file.LineAt(offset)
				directives = append(directives, inlineDirective{
					line: line, offset: offset,
					action: strings.ToLower(string(content[match[2]:match[3]])),
					rules:  directiveRuleIDs(string(content[match[4]:match[5]]), registry),
				})
			}
		}
	}
	slices.SortFunc(directives, func(a, b inlineDirective) int { return a.offset - b.offset })
	return directives
}

func directiveRuleIDs(parameter string, registry *Registry) []string {
	items := strings.Fields(strings.ToLower(parameter))
	if len(items) == 0 {
		return registry.IDs()
	}
	ids := make(map[string]bool)
	for _, item := range items {
		key := item
		if strings.HasPrefix(item, "md") {
			key = strings.ToUpper(item)
		}
		if id, _, ok := registry.Resolve(key); ok {
			ids[id] = true
		}
		for _, rule := range registry.Rules() {
			for _, tag := range rule.Tags() {
				if strings.EqualFold(tag, item) {
					ids[rule.ID()] = true
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(ids))
}

// directiveStates computes rule enablement by original source line. File directives
// apply first, persistent directives second, and line overrides last.
func directiveStates(file *mdast.FileSnapshot, registry *Registry, cfg *config.Config) map[string][]bool {
	directives := collectDirectives(file, registry)
	state := make(map[string]bool)
	states := make(map[string][]bool)
	for _, rule := range registry.Rules() {
		state[rule.ID()] = resolveRule(rule, cfg).Enabled
		states[rule.ID()] = make([]bool, len(file.Lines)+1)
	}
	set := func(directive inlineDirective) {
		for _, id := range directive.rules {
			state[id] = strings.HasPrefix(directive.action, "enable")
		}
	}
	for _, directive := range directives {
		if directive.action == "disable-file" || directive.action == "enable-file" {
			set(directive)
		}
	}
	captured := maps.Clone(state)
	next := 0
	for line := 1; line <= len(file.Lines); line++ {
		for next < len(directives) && directives[next].line == line {
			directive := directives[next]
			switch directive.action {
			case "disable", "enable":
				set(directive)
			case "capture":
				captured = maps.Clone(state)
			case "restore":
				state = maps.Clone(captured)
			}
			next++
		}
		for id, enabled := range state {
			states[id][line] = enabled && line > file.FrontMatterLines
		}
	}
	for _, directive := range directives {
		line := directive.line
		switch directive.action {
		case "disable-next-line":
			line++
		case "disable-line":
		default:
			continue
		}
		if line <= len(file.Lines) {
			for _, id := range directive.rules {
				states[id][line] = false
			}
		}
	}
	return states
}
