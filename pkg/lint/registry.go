package lint

import (
	"cmp"
	"slices"
	"sync"
)

// Registry holds all registered lint rules.
type Registry struct {
	mu      sync.RWMutex
	byID    map[string]Rule
	byName  map[string]Rule
	aliases map[string]string // alias -> canonical ID
}

// NewRegistry creates an empty rule registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:    make(map[string]Rule),
		byName:  make(map[string]Rule),
		aliases: make(map[string]string),
	}
}

// Register adds a rule to the registry.
// If a rule with the same ID already exists, it is replaced.
func (r *Registry) Register(rule Rule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[rule.ID()] = rule
	r.byName[rule.Name()] = rule
}

// RegisterAlias maps an alias to a canonical rule ID.
// Used for legacy markdownlint compatibility (e.g., "single-h1" -> "MD041").
func (r *Registry) RegisterAlias(alias, ruleID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases[alias] = ruleID
}

// Get retrieves a rule by ID or name.
// It tries ID first, then falls back to name lookup.
//

func (r *Registry) Get(key string) (Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try ID first
	if rule, ok := r.byID[key]; ok {
		return rule, true
	}
	// Fall back to name
	if rule, ok := r.byName[key]; ok {
		return rule, true
	}
	return nil, false
}

// GetByID retrieves a rule by its ID only.
//

func (r *Registry) GetByID(id string) (Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.byID[id]
	return rule, ok
}

// GetByName retrieves a rule by its name only.
//

func (r *Registry) GetByName(name string) (Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.byName[name]
	return rule, ok
}

// Resolve returns the canonical ID and rule for a given key.
// The key can be a rule ID, name, or legacy alias.
// Returns (id, rule, found).
//

func (r *Registry) Resolve(key string) (string, Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try ID first
	if rule, ok := r.byID[key]; ok {
		return rule.ID(), rule, true
	}
	// Try name
	if rule, ok := r.byName[key]; ok {
		return rule.ID(), rule, true
	}
	// Try alias
	if targetID, ok := r.aliases[key]; ok {
		if rule, ok := r.byID[targetID]; ok {
			return rule.ID(), rule, true
		}
	}
	return "", nil, false
}

// Rules returns all registered rules.
func (r *Registry) Rules() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Rule, 0, len(r.byID))
	for _, rule := range r.byID {
		result = append(result, rule)
	}

	// Sort by rule ID for consistent, deterministic output.
	slices.SortFunc(result, func(a, b Rule) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	return result
}

// IDs returns all registered rule IDs in sorted order.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.byID))
	for id := range r.byID {
		result = append(result, id)
	}

	slices.Sort(result)
	return result
}

// DefaultRegistry is the global registry for built-in rules.
// Rules register themselves during init().
//
//nolint:gochecknoglobals // Global registry is intentional for rule registration
var DefaultRegistry = NewRegistry()
