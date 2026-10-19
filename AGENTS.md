# Agent Instructions

This project tracks issues in GitHub Issues on
[yaklabco/gomdlint](https://github.com/yaklabco/gomdlint/issues). Use the
`gh` CLI to work with them.

## Quick Reference

```bash
gh issue list                               # Find available work
gh issue view <number>                      # View issue details
gh issue edit <number> --add-assignee @me   # Claim work
gh issue close <number>                     # Complete work
```

## Ending a Work Session

When you end a work session, take these steps:

1. **File issues for remaining work.** Create an issue with `gh issue create`
   for anything that needs follow-up.
2. **Run the quality gates** if code changed: tests, linters and builds.
3. **Update issue status.** Close finished work and update items in progress.
4. **Commit finished work on a branch.** Never commit to `main` directly.
5. **Push only when the user asks you to.** A push publishes the work, so it
   is the user's decision. Never push to `main` and never force-push.
6. **Hand off.** Say what is committed, what is pushed and what is left to
   do.
