# Code Block Immunity

Lines inside code blocks should not trigger MD013 even if they are very long.

```go
func exampleFunction() string { return "this line is intentionally made very long to exceed the default line length limit of one hundred and twenty characters for testing" }
```

```bash
echo "another long line inside a code block that also exceeds one hundred and twenty characters to verify that code blocks are properly excluded from line length checks"
```

Short line after code blocks.
