# External URL Fragments

This test verifies that external URLs with fragments are not validated
against local document anchors.

## Local Heading

This is a local heading that we can link to.

## Links

External URLs with fragments should NOT trigger warnings:

- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#summary)
- [GitHub Docs](https://docs.github.com/en/get-started#next-steps)
- [MDN](https://developer.mozilla.org/en-US/docs/Web#tutorials)

Local file paths with fragments should also NOT trigger warnings
(cross-file validation is not supported):

- [README section](./README.md#installation)
- [Other doc](../docs/guide.md#usage)

Same-file fragments SHOULD be validated:

- [Valid local link](#local-heading) - this should pass
- [Invalid local link](#nonexistent-heading) - this should fail
