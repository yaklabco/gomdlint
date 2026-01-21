package configloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConvertMarkdownlintConfig_JSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdownlint JSON config
	configContent := `{
  "MD001": true,
  "MD009": false,
  "MD013": {
    "line_length": 120,
    "tables": false
  },
  "heading-increment": true
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	if result.Config == nil {
		t.Fatal("result.Config is nil")
	}

	// Check MD001 is enabled
	md001, ok := result.Config.Rules["MD001"]
	if !ok {
		t.Fatal("MD001 rule not found in config")
	}
	if md001.Enabled == nil || !*md001.Enabled {
		t.Error("expected MD001 to be enabled")
	}

	// Check MD009 is disabled
	md009, ok := result.Config.Rules["MD009"]
	if !ok {
		t.Fatal("MD009 rule not found in config")
	}
	if md009.Enabled == nil || *md009.Enabled {
		t.Error("expected MD009 to be disabled")
	}

	// Check MD013 has options
	md013, ok := result.Config.Rules["MD013"]
	if !ok {
		t.Fatal("MD013 rule not found in config")
	}
	if md013.Options == nil {
		t.Fatal("MD013 options is nil")
	}
	if lineLen, ok := md013.Options["line_length"].(float64); !ok || lineLen != 120 {
		t.Errorf("expected line_length 120, got %v", md013.Options["line_length"])
	}
}

func TestConvertMarkdownlintConfig_YAML(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdownlint YAML config
	configContent := `
default: true
MD001: true
MD009: false
MD013:
  line_length: 100
`
	configPath := filepath.Join(tmpDir, ".markdownlint.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	if result.Config == nil {
		t.Fatal("result.Config is nil")
	}

	// Check MD013 options
	md013, ok := result.Config.Rules["MD013"]
	if !ok {
		t.Fatal("MD013 rule not found in config")
	}
	if md013.Options == nil {
		t.Fatal("MD013 options is nil")
	}
}

func TestConvertMarkdownlintConfig_Aliases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config using aliases
	configContent := `{
  "heading-increment": true,
  "no-trailing-spaces": false,
  "line-length": {
    "line_length": 80
  }
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// Aliases should be normalized to rule IDs
	if _, ok := result.Config.Rules["MD001"]; !ok {
		t.Error("heading-increment should be normalized to MD001")
	}

	if _, ok := result.Config.Rules["MD009"]; !ok {
		t.Error("no-trailing-spaces should be normalized to MD009")
	}

	if _, ok := result.Config.Rules["MD013"]; !ok {
		t.Error("line-length should be normalized to MD013")
	}
}

func TestConvertMarkdownlintConfig_Tags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config using tag-based disabling
	configContent := `{
  "whitespace": false
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// All whitespace rules should be disabled
	whitespaceRules := GetTagRules("whitespace")
	for _, ruleID := range whitespaceRules {
		rule, ok := result.Config.Rules[ruleID]
		if !ok {
			t.Errorf("expected %s to be in config (from whitespace tag)", ruleID)
			continue
		}
		if rule.Enabled == nil || *rule.Enabled {
			t.Errorf("expected %s to be disabled (from whitespace tag)", ruleID)
		}
	}
}

func TestConvertMarkdownlintConfig_SpecialKeys(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config with special keys
	configContent := `{
  "$schema": "https://example.com/schema.json",
  "default": false,
  "extends": "some-preset",
  "MD001": true
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// Should have warnings about special keys
	if len(result.Warnings) == 0 {
		t.Error("expected warnings about default and extends")
	}

	// MD001 should still be processed
	if _, ok := result.Config.Rules["MD001"]; !ok {
		t.Error("MD001 should be in config")
	}
}

func TestConvertMarkdownlintConfig_JavaScript(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a JavaScript config file
	configPath := filepath.Join(tmpDir, ".markdownlint.cjs")
	if err := os.WriteFile(configPath, []byte("module.exports = {}"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ConvertMarkdownlintConfig(configPath)
	if err == nil {
		t.Fatal("expected error for JavaScript config file")
	}
}

func TestConvertMarkdownlintConfig_InvalidJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create an invalid JSON config
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte("{ invalid json }"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ConvertMarkdownlintConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConvertMarkdownlintConfig_JSONC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a JSONC config with comments
	configContent := `{
  // This is a comment
  "MD001": true,
  /* Multi-line
     comment */
  "MD009": false
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.jsonc")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// Check rules were parsed correctly
	if _, ok := result.Config.Rules["MD001"]; !ok {
		t.Error("MD001 should be in config")
	}
	if _, ok := result.Config.Rules["MD009"]; !ok {
		t.Error("MD009 should be in config")
	}
}

func TestCanMigrate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{".markdownlint.json", true},
		{".markdownlint.jsonc", true},
		{".markdownlint.yaml", true},
		{".markdownlint.yml", true},
		{".markdownlint.cjs", false},
		{".markdownlint.mjs", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			result := CanMigrate(tt.path)
			if result != tt.expected {
				t.Errorf("CanMigrate(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsJavaScriptConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{".markdownlint.cjs", true},
		{".markdownlint.mjs", true},
		{".markdownlint.json", false},
		{".markdownlint.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			result := IsJavaScriptConfig(tt.path)
			if result != tt.expected {
				t.Errorf("IsJavaScriptConfig(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
