package fix_test

import (
	"errors"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestValidateEdits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		edits      []fix.TextEdit
		contentLen int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "empty edits",
			edits:      nil,
			contentLen: 10,
			wantErr:    false,
		},
		{
			name: "valid edits",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "hello"},
				{StartOffset: 5, EndOffset: 10, NewText: "world"},
			},
			contentLen: 10,
			wantErr:    false,
		},
		{
			name: "negative start offset",
			edits: []fix.TextEdit{
				{StartOffset: -1, EndOffset: 5, NewText: "hello"},
			},
			contentLen: 10,
			wantErr:    true,
			errMsg:     "start offset is negative",
		},
		{
			name: "end before start",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 3, NewText: "hello"},
			},
			contentLen: 10,
			wantErr:    true,
			errMsg:     "end offset is before start offset",
		},
		{
			name: "end exceeds content length",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 15, NewText: "hello"},
			},
			contentLen: 10,
			wantErr:    true,
			errMsg:     "exceeds content length",
		},
		{
			name: "zero-length edit (insertion)",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 5, NewText: "insert"},
			},
			contentLen: 10,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fix.ValidateEdits(tt.edits, tt.contentLen)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}

				var valErr *fix.ValidationError
				if !errors.As(err, &valErr) {
					t.Errorf("expected ValidationError, got %T", err)
					return
				}

				if tt.errMsg != "" && !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSortEdits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		edits []fix.TextEdit
		want  []fix.TextEdit
	}{
		{
			name:  "empty",
			edits: nil,
			want:  nil,
		},
		{
			name: "already sorted",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5},
				{StartOffset: 5, EndOffset: 10},
			},
			want: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5},
				{StartOffset: 5, EndOffset: 10},
			},
		},
		{
			name: "reverse order",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10},
				{StartOffset: 0, EndOffset: 5},
			},
			want: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5},
				{StartOffset: 5, EndOffset: 10},
			},
		},
		{
			name: "same start, different end",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10},
				{StartOffset: 0, EndOffset: 5},
			},
			want: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5},
				{StartOffset: 0, EndOffset: 10},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			edits := make([]fix.TextEdit, len(tt.edits))
			copy(edits, tt.edits)

			fix.SortEdits(edits)

			if len(edits) != len(tt.want) {
				t.Fatalf("length mismatch: got %d, want %d", len(edits), len(tt.want))
			}

			for i := range edits {
				if edits[i].StartOffset != tt.want[i].StartOffset ||
					edits[i].EndOffset != tt.want[i].EndOffset {
					t.Errorf("edit[%d]: got [%d:%d], want [%d:%d]",
						i, edits[i].StartOffset, edits[i].EndOffset,
						tt.want[i].StartOffset, tt.want[i].EndOffset)
				}
			}
		})
	}
}

func TestDetectConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		edits   []fix.TextEdit
		wantErr bool
	}{
		{
			name:    "empty",
			edits:   nil,
			wantErr: false,
		},
		{
			name: "no conflicts (adjacent)",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5},
				{StartOffset: 5, EndOffset: 10},
			},
			wantErr: false,
		},
		{
			name: "overlapping edits",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7},
				{StartOffset: 5, EndOffset: 10},
			},
			wantErr: true,
		},
		{
			name: "contained edit",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10},
				{StartOffset: 3, EndOffset: 7},
			},
			wantErr: true,
		},
		{
			name: "single edit",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fix.DetectConflicts(tt.edits)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}

				var conflictErr *fix.ConflictError
				if !errors.As(err, &conflictErr) {
					t.Errorf("expected ConflictError, got %T", err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPrepareEdits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		edits      []fix.TextEdit
		contentLen int
		wantErr    bool
		wantLen    int
	}{
		{
			name:       "empty",
			edits:      nil,
			contentLen: 10,
			wantErr:    false,
			wantLen:    0,
		},
		{
			name: "valid non-overlapping",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10},
				{StartOffset: 0, EndOffset: 5},
			},
			contentLen: 10,
			wantErr:    false,
			wantLen:    2,
		},
		{
			name: "validation error",
			edits: []fix.TextEdit{
				{StartOffset: -1, EndOffset: 5},
			},
			contentLen: 10,
			wantErr:    true,
		},
		{
			name: "conflict error",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7},
				{StartOffset: 5, EndOffset: 10},
			},
			contentLen: 10,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := fix.PrepareEdits(tt.edits, tt.contentLen)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != tt.wantLen {
				t.Errorf("result length: got %d, want %d", len(result), tt.wantLen)
			}

			// Verify sorted order.
			for i := 1; i < len(result); i++ {
				if result[i].StartOffset < result[i-1].StartOffset {
					t.Error("result not sorted")
				}
			}
		})
	}
}

func TestFilterConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		edits        []fix.TextEdit
		wantAccepted []fix.TextEdit
		wantSkipped  []fix.TextEdit
	}{
		{
			name:         "empty",
			edits:        nil,
			wantAccepted: nil,
			wantSkipped:  nil,
		},
		{
			name: "single edit",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
			},
			wantSkipped: nil,
		},
		{
			name: "no conflicts - adjacent edits",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
			},
			wantSkipped: nil,
		},
		{
			name: "no conflicts - gap between edits",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 10, EndOffset: 15, NewText: "b"},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 10, EndOffset: 15, NewText: "b"},
			},
			wantSkipped: nil,
		},
		{
			name: "overlapping edits - first wins",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "a"},
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "a"},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
			},
		},
		{
			name: "contained edit - outer wins",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: "a"},
				{StartOffset: 3, EndOffset: 7, NewText: "b"},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: "a"},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 3, EndOffset: 7, NewText: "b"},
			},
		},
		{
			name: "real case - trailing newlines conflict",
			edits: []fix.TextEdit{
				{StartOffset: 6606, EndOffset: 6608, NewText: ""}, // single-trailing-newline
				{StartOffset: 6607, EndOffset: 6608, NewText: ""}, // no-multiple-blank-lines
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 6606, EndOffset: 6608, NewText: ""},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 6607, EndOffset: 6608, NewText: ""},
			},
		},
		{
			name: "multiple conflicts - accept non-overlapping",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: "a"},
				{StartOffset: 5, EndOffset: 8, NewText: "b"},   // conflicts with first
				{StartOffset: 7, EndOffset: 12, NewText: "c"},  // conflicts with first
				{StartOffset: 15, EndOffset: 20, NewText: "d"}, // no conflict
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: "a"},
				{StartOffset: 15, EndOffset: 20, NewText: "d"},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 8, NewText: "b"},
				{StartOffset: 7, EndOffset: 12, NewText: "c"},
			},
		},
		{
			name: "chain of conflicts",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 4, EndOffset: 9, NewText: "b"},   // conflicts with a
				{StartOffset: 8, EndOffset: 13, NewText: "c"},  // would conflict with b, but b is skipped
				{StartOffset: 12, EndOffset: 17, NewText: "d"}, // conflicts with c
			},
			// After sorting: a(0-5), b(4-9), c(8-13), d(12-17)
			// Accept a, skip b (4<5), skip c (8<5? no, 8>=5, accept!), skip d (12<13)
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 8, EndOffset: 13, NewText: "c"},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 4, EndOffset: 9, NewText: "b"},
				{StartOffset: 12, EndOffset: 17, NewText: "d"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accepted, skipped := fix.FilterConflicts(tt.edits)

			// Check accepted
			if len(accepted) != len(tt.wantAccepted) {
				t.Errorf("accepted length: got %d, want %d", len(accepted), len(tt.wantAccepted))
			} else {
				for i := range accepted {
					if accepted[i] != tt.wantAccepted[i] {
						t.Errorf("accepted[%d]: got %+v, want %+v", i, accepted[i], tt.wantAccepted[i])
					}
				}
			}

			// Check skipped
			if len(skipped) != len(tt.wantSkipped) {
				t.Errorf("skipped length: got %d, want %d", len(skipped), len(tt.wantSkipped))
			} else {
				for i := range skipped {
					if skipped[i] != tt.wantSkipped[i] {
						t.Errorf("skipped[%d]: got %+v, want %+v", i, skipped[i], tt.wantSkipped[i])
					}
				}
			}
		})
	}
}

func TestMergeAndFilterConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		edits        []fix.TextEdit
		wantAccepted []fix.TextEdit
		wantSkipped  []fix.TextEdit
		wantMerged   int
	}{
		{
			name:         "empty",
			edits:        nil,
			wantAccepted: nil,
			wantSkipped:  nil,
			wantMerged:   0,
		},
		{
			name: "single edit",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
			},
			wantSkipped: nil,
			wantMerged:  0,
		},
		{
			name: "no conflicts - adjacent deletions",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
				{StartOffset: 5, EndOffset: 10, NewText: ""},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
				{StartOffset: 5, EndOffset: 10, NewText: ""},
			},
			wantSkipped: nil,
			wantMerged:  0,
		},
		{
			name: "merge overlapping deletions",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: ""},
				{StartOffset: 5, EndOffset: 10, NewText: ""},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: ""}, // merged!
			},
			wantSkipped: nil,
			wantMerged:  1,
		},
		{
			name: "merge contained deletion",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: ""},
				{StartOffset: 3, EndOffset: 7, NewText: ""},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: ""}, // outer contains inner
			},
			wantSkipped: nil,
			wantMerged:  1,
		},
		{
			name: "real case - trailing newlines (both deletions)",
			edits: []fix.TextEdit{
				{StartOffset: 6606, EndOffset: 6608, NewText: ""}, // single-trailing-newline
				{StartOffset: 6607, EndOffset: 6608, NewText: ""}, // no-multiple-blank-lines
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 6606, EndOffset: 6608, NewText: ""}, // merged (same result)
			},
			wantSkipped: nil,
			wantMerged:  1,
		},
		{
			name: "cannot merge - one has replacement text",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "foo"}, // replacement, not deletion
				{StartOffset: 5, EndOffset: 10, NewText: ""},   // deletion
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "foo"}, // first wins
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10, NewText: ""}, // skipped (can't merge)
			},
			wantMerged: 0,
		},
		{
			name: "cannot merge - both have different replacement text",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "foo"},
				{StartOffset: 5, EndOffset: 10, NewText: "bar"},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "foo"},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10, NewText: "bar"},
			},
			wantMerged: 0,
		},
		{
			name: "merge multiple overlapping deletions",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
				{StartOffset: 3, EndOffset: 8, NewText: ""},
				{StartOffset: 6, EndOffset: 12, NewText: ""},
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 12, NewText: ""}, // all merged into one
			},
			wantSkipped: nil,
			wantMerged:  2,
		},
		{
			name: "mix of merge and non-overlap",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
				{StartOffset: 3, EndOffset: 8, NewText: ""},   // merge with first
				{StartOffset: 20, EndOffset: 25, NewText: ""}, // separate, no conflict
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 8, NewText: ""},
				{StartOffset: 20, EndOffset: 25, NewText: ""},
			},
			wantSkipped: nil,
			wantMerged:  1,
		},
		{
			name: "merge some, skip others",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: ""},      // deletion
				{StartOffset: 5, EndOffset: 8, NewText: ""},       // merge (deletion)
				{StartOffset: 7, EndOffset: 15, NewText: "hello"}, // skip (replacement)
				{StartOffset: 20, EndOffset: 25, NewText: ""},     // accept (no overlap)
			},
			wantAccepted: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 10, NewText: ""},
				{StartOffset: 20, EndOffset: 25, NewText: ""},
			},
			wantSkipped: []fix.TextEdit{
				{StartOffset: 7, EndOffset: 15, NewText: "hello"},
			},
			wantMerged: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accepted, skipped, merged := fix.MergeAndFilterConflicts(tt.edits)

			// Check merged count
			if merged != tt.wantMerged {
				t.Errorf("merged count: got %d, want %d", merged, tt.wantMerged)
			}

			// Check accepted
			if len(accepted) != len(tt.wantAccepted) {
				t.Errorf("accepted length: got %d, want %d", len(accepted), len(tt.wantAccepted))
			} else {
				for i := range accepted {
					if accepted[i] != tt.wantAccepted[i] {
						t.Errorf("accepted[%d]: got %+v, want %+v", i, accepted[i], tt.wantAccepted[i])
					}
				}
			}

			// Check skipped
			if len(skipped) != len(tt.wantSkipped) {
				t.Errorf("skipped length: got %d, want %d", len(skipped), len(tt.wantSkipped))
			} else {
				for i := range skipped {
					if skipped[i] != tt.wantSkipped[i] {
						t.Errorf("skipped[%d]: got %+v, want %+v", i, skipped[i], tt.wantSkipped[i])
					}
				}
			}
		})
	}
}

func TestPrepareEditsFiltered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		edits           []fix.TextEdit
		contentLen      int
		wantAcceptedLen int
		wantSkippedLen  int
		wantErr         bool
	}{
		{
			name:            "empty",
			edits:           nil,
			contentLen:      10,
			wantAcceptedLen: 0,
			wantSkippedLen:  0,
			wantErr:         false,
		},
		{
			name: "no conflicts",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
			},
			contentLen:      10,
			wantAcceptedLen: 2,
			wantSkippedLen:  0,
			wantErr:         false,
		},
		{
			name: "with conflicts - filters instead of errors",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 7, NewText: "a"},
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
			},
			contentLen:      10,
			wantAcceptedLen: 1,
			wantSkippedLen:  1,
			wantErr:         false,
		},
		{
			name: "validation error still fails",
			edits: []fix.TextEdit{
				{StartOffset: -1, EndOffset: 5, NewText: "a"},
			},
			contentLen: 10,
			wantErr:    true,
		},
		{
			name: "out of bounds still fails",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 15, NewText: "a"},
			},
			contentLen: 10,
			wantErr:    true,
		},
		{
			name: "sorts before filtering",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 10, NewText: "b"},
				{StartOffset: 0, EndOffset: 7, NewText: "a"}, // conflicts after sorting
			},
			contentLen:      10,
			wantAcceptedLen: 1, // first by start wins: a(0-7)
			wantSkippedLen:  1, // b(5-10) conflicts
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accepted, skipped, _, err := fix.PrepareEditsFiltered(tt.edits, tt.contentLen)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(accepted) != tt.wantAcceptedLen {
				t.Errorf("accepted length: got %d, want %d", len(accepted), tt.wantAcceptedLen)
			}

			if len(skipped) != tt.wantSkippedLen {
				t.Errorf("skipped length: got %d, want %d", len(skipped), tt.wantSkippedLen)
			}

			// Verify accepted is sorted
			for i := 1; i < len(accepted); i++ {
				if accepted[i].StartOffset < accepted[i-1].StartOffset {
					t.Error("accepted not sorted")
				}
			}

			// Verify no conflicts in accepted
			for i := 1; i < len(accepted); i++ {
				if accepted[i].StartOffset < accepted[i-1].EndOffset {
					t.Error("accepted has conflicts")
				}
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsSubstringHelper(s, substr)))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
