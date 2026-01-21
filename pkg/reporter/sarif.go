package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/runner"
)

// SARIF version used by this reporter.
const sarifVersion = "2.1.0"

// SARIF schema URI.
const sarifSchemaURI = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"

// SARIFOutput represents the root SARIF document.
type SARIFOutput struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun represents a single analysis run.
type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

// SARIFTool describes the analysis tool.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver contains tool metadata and rules.
type SARIFDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []SARIFRule `json:"rules"`
}

// SARIFRule describes a rule (linter check).
type SARIFRule struct {
	ID               string               `json:"id"`
	Name             string               `json:"name,omitempty"`
	ShortDescription SARIFMultiformatText `json:"shortDescription,omitempty"`
	DefaultConfig    *SARIFRuleConfig     `json:"defaultConfiguration,omitempty"`
	Properties       map[string]any       `json:"properties,omitempty"`
}

// SARIFMultiformatText contains text in multiple formats.
type SARIFMultiformatText struct {
	Text string `json:"text"`
}

// SARIFRuleConfig contains rule configuration.
type SARIFRuleConfig struct {
	Level string `json:"level"`
}

// SARIFResult represents a single diagnostic result.
type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations"`
	Fixes     []SARIFFix      `json:"fixes,omitempty"`
}

// SARIFMessage contains the result message.
type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFLocation describes a code location.
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

// SARIFPhysicalLocation contains file path and region.
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

// SARIFArtifactLocation contains the file URI.
type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

// SARIFRegion describes the affected text region.
type SARIFRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIFFix represents a proposed fix.
type SARIFFix struct {
	Description     SARIFMessage          `json:"description"`
	ArtifactChanges []SARIFArtifactChange `json:"artifactChanges"`
}

// SARIFArtifactChange describes changes to a file.
type SARIFArtifactChange struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Replacements     []SARIFReplacement    `json:"replacements"`
}

// SARIFReplacement describes a text replacement.
type SARIFReplacement struct {
	DeletedRegion   SARIFRegion           `json:"deletedRegion"`
	InsertedContent *SARIFInsertedContent `json:"insertedContent,omitempty"`
}

// SARIFInsertedContent contains the replacement text.
type SARIFInsertedContent struct {
	Text string `json:"text"`
}

// SARIFReporter formats results as SARIF.
type SARIFReporter struct {
	opts Options
	out  io.Writer
}

// NewSARIFReporter creates a new SARIF reporter.
func NewSARIFReporter(opts Options) *SARIFReporter {
	return &SARIFReporter{
		opts: opts,
		out:  opts.Writer,
	}
}

// Report implements Reporter.
func (r *SARIFReporter) Report(_ context.Context, result *runner.Result) (int, error) {
	output := r.buildOutput(result)

	encoder := json.NewEncoder(r.out)
	if !r.opts.Compact {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(output); err != nil {
		return 0, fmt.Errorf("encode SARIF: %w", err)
	}

	return len(output.Runs[0].Results), nil
}

func (r *SARIFReporter) buildOutput(result *runner.Result) *SARIFOutput {
	output := &SARIFOutput{
		Schema:  sarifSchemaURI,
		Version: sarifVersion,
		Runs: []SARIFRun{{
			Tool: SARIFTool{
				Driver: SARIFDriver{
					Name:           "gomdlint",
					Version:        "0.1.0",
					InformationURI: "https://github.com/jamesainslie/gomdlint",
					Rules:          make([]SARIFRule, 0),
				},
			},
			Results: make([]SARIFResult, 0),
		}},
	}

	if result == nil {
		return output
	}

	// Track rules we've already added
	rulesSeen := make(map[string]bool)

	for _, file := range result.Files {
		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}

		for _, diag := range file.Result.Diagnostics {
			// Add rule if not already seen
			if !rulesSeen[diag.RuleID] {
				rule := SARIFRule{
					ID:   diag.RuleID,
					Name: diag.RuleName,
					ShortDescription: SARIFMultiformatText{
						Text: diag.Message,
					},
					DefaultConfig: &SARIFRuleConfig{
						Level: severityToSARIFLevel(diag.Severity),
					},
				}
				output.Runs[0].Tool.Driver.Rules = append(output.Runs[0].Tool.Driver.Rules, rule)
				rulesSeen[diag.RuleID] = true
			}

			// Create result
			sarifResult := SARIFResult{
				RuleID: diag.RuleID,
				Level:  severityToSARIFLevel(diag.Severity),
				Message: SARIFMessage{
					Text: diag.Message,
				},
				Locations: []SARIFLocation{{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI: diag.FilePath,
						},
						Region: SARIFRegion{
							StartLine:   diag.StartLine,
							StartColumn: diag.StartColumn,
							EndLine:     diag.EndLine,
							EndColumn:   diag.EndColumn,
						},
					},
				}},
			}

			// Add fixes if available
			if len(diag.FixEdits) > 0 && diag.Suggestion != "" {
				fix := SARIFFix{
					Description: SARIFMessage{
						Text: diag.Suggestion,
					},
					ArtifactChanges: make([]SARIFArtifactChange, 0, len(diag.FixEdits)),
				}

				for _, edit := range diag.FixEdits {
					change := SARIFArtifactChange{
						ArtifactLocation: SARIFArtifactLocation{
							URI: diag.FilePath,
						},
						Replacements: []SARIFReplacement{{
							DeletedRegion: SARIFRegion{
								// Note: SARIF uses byte offsets differently
								// This is a simplified representation
								StartLine: diag.StartLine,
							},
							InsertedContent: &SARIFInsertedContent{
								Text: edit.NewText,
							},
						}},
					}
					fix.ArtifactChanges = append(fix.ArtifactChanges, change)
				}

				sarifResult.Fixes = append(sarifResult.Fixes, fix)
			}

			output.Runs[0].Results = append(output.Runs[0].Results, sarifResult)
		}
	}

	return output
}

// severityToSARIFLevel converts gomdlint severity to SARIF level.
func severityToSARIFLevel(severity config.Severity) string {
	switch severity {
	case config.SeverityError:
		return "error"
	case config.SeverityWarning:
		return "warning"
	case config.SeverityInfo:
		return "note"
	default:
		return "warning"
	}
}
