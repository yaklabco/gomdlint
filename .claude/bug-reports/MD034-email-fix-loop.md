---
rule_id: MD034
status: fixed
severity: blocking
discovered_by: golden-test-authoring
discovered_during: round-trip testing (email patterns)
affected_files:
  - pkg/lint/rules/links_extended.go
related_test_files:
  - pkg/lint/rules/links_extended_test.go
---

# MD034: Regex consumes boundary characters, causing false positives on wrapped emails

## Symptom

After wrapping a bare email in angle brackets (`user@example.com` -> `<user@example.com>`),
re-running the rule detects a false positive: `ser@example.co` inside the wrapped email.
This causes document corruption in multi-pass fix scenarios.

## Root Cause

The regex `(?:^|[^<(\[])(URL|EMAIL)(?:[^>\])]|$)` uses character-consuming groups for
boundary validation. The prefix group `[^<(\[]` consumes one character (e.g., `u` from
`user@example.com` inside `<user@example.com>`), shifting the capture group indices.
The captured "email" becomes `ser@example.co` (truncated), and the skip check
`lineContent[urlStart-1] == '<'` fails because it checks the character before `ser`,
which is `u`, not `<`.

Go's `regexp` package does not support zero-width lookbehind assertions, so the
consuming-group approach is fundamentally flawed for this use case.

## Evidence

The existing unit test `"bare email"` only tested `a@b.co` — a 1-char username with
2-char TLD, which is too short to trigger the bug. Real-world emails with 2+ char
usernames and 3+ char TLDs (i.e., virtually all emails) are affected.

Test results before fix: 7 of 8 email edge case tests failed on idempotency.
The "already wrapped email" test also failed — detecting a false positive on input
that was already correctly formatted.

## Resolution

Replaced the boundary-consuming regex with a simpler pattern that only matches
the URL/email itself: `https?://[^\s<>\[\]()]+|[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`

Moved boundary validation into code after each match:
- Check if preceded by `<`, `(`, or `[` -> skip (already in markdown syntax)
- Check if followed by `>`, `)`, or `]` -> skip (closing markdown syntax)
- Changed from `FindAllSubmatchIndex` to `FindAllIndex` (no capture groups needed)

Also pre-compiled the `isEmail` check regex as a package-level var instead of
recompiling on every call.

Added 8 email-specific edge case tests covering: start/end of line, alone on line,
multiple emails, complex domains, plus addressing, already-wrapped, and mixed with URLs.
All pass including idempotency checks.
