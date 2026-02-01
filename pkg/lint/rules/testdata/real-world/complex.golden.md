# Complex Markdown Test Document

This document exercises multiple markdown rules for comprehensive testing.

## Table of Contents

1. [Headings](#headings)
2. [Lists](#lists)
3. [Code Blocks](#code-blocks)
4. [Links and Images](#links-and-images)
5. [Emphasis and Formatting](#emphasis-and-formatting)
6. [Tables](#tables)
7. [Blockquotes](#blockquotes)

## Headings

### Third Level Heading

This section demonstrates heading hierarchy.

#### Fourth Level Heading

Going deeper into the heading structure.

##### Fifth Level Heading

Nearly at the bottom of the heading hierarchy.

###### Sixth Level Heading

The deepest heading level in markdown.

## Lists

### Unordered Lists

- First item
- Second item
  - Nested item one
  - Nested item two
    - Deeply nested item
- Third item

Alternative markers:

- Asterisk item one
- Asterisk item two

### Ordered Lists

1. First numbered item
2. Second numbered item
   1. Nested numbered item
   2. Another nested item
3. Third numbered item

### Mixed Lists

1. Start with numbered
   - Then unordered nested
   - Another bullet
2. Back to numbered
   1. Nested ordered
      - Mixed with bullets

### Task Lists

- [x] Completed task
- [ ] Incomplete task
- [x] Another done item
- [ ] Still to do

## Code Blocks

### Inline Code

Use `backticks` for inline code like `const x = 42` or `func main()`.

### Fenced Code Blocks

Basic Go code:

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
```

JavaScript example:

```javascript
const greet = (name) => {
    console.log(`Hello, ${name}!`);
};

greet("World");
```

Shell commands:

```bash
echo "Running tests..."
go test ./...
make build
```

Code without language specification:

```
This is plain text in a fenced block
with multiple lines
and no syntax highlighting.
```

### Indented Code Block

The following is an indented code block (4 spaces):

    func indented() {
        // This is an indented code block
        return true
    }

## Links and Images

### Inline Links

Visit [Google](https://www.google.com) for search.

Here is a link to [GitHub](https://github.com "GitHub Homepage") with a title.

### Reference Links

This is a [reference link][reflink] that points elsewhere.

You can also use [implicit reference links][].

[reflink]: <https://example.com> "Reference Link"
[implicit reference links]: <https://example.org>

### Autolinks

Automatic URL linking: <https://www.example.com>

Email autolink: <user@example.com>

### Images

![Alt text for image](https://via.placeholder.com/150 "Image Title")

Reference style image:

![Reference image][imgref]

[imgref]: <https://via.placeholder.com/200> "Reference Image"

## Emphasis and Formatting

### Basic Emphasis

This text has *single asterisk emphasis* and _single underscore emphasis_.

This text has **double asterisk strong** and __double underscore strong__.

### Combined Emphasis

This is ***bold and italic*** together.

This is also ___bold and italic___ with underscores.

And this is **_mixed markers_** for emphasis.

### Strikethrough

This text has ~~strikethrough~~ applied.

### Horizontal Rules

Below is a horizontal rule:

---

Another style:

---

And another:

---

## Tables

### Simple Table

| Column 1 | Column 2 | Column 3 |
|----------|----------|----------|
| Cell 1   | Cell 2   | Cell 3   |
| Cell 4   | Cell 5   | Cell 6   |

### Aligned Table

| Left     | Center   | Right    |
|:---------|:--------:|---------:|
| L1       | C1       | R1       |
| L2       | C2       | R2       |
| L3       | C3       | R3       |

### Complex Table

| Feature | Description | Supported |
|---------|-------------|:---------:|
| Tables | Data in rows and columns | Yes |
| Alignment | Left, center, right | Yes |
| Multi-line | Content spanning lines | No |
| Nested | Tables within tables | No |

## Blockquotes

### Simple Blockquote

> This is a simple blockquote.
> It can span multiple lines.

### Nested Blockquotes

> Level one of the blockquote.
>
> > Level two is nested inside.
> >
> > > Level three goes even deeper.

### Blockquote with Other Elements

> ### Heading in Blockquote

>
> A paragraph with **bold** and *italic* text.
>
> - List item one
> - List item two
>

> ```go
> // Code in blockquote
> fmt.Println("Quoted code")
> ```

## Special Characters and Escaping

Backslash escapes: \* \_ \# \[ \] \( \) \` \~

HTML entities: &amp; &lt; &gt; &copy;

Unicode: snowman is here

## Definition Lists (Extended Syntax)

Term 1
:   Definition for term 1

Term 2
:   Definition for term 2
:   Another definition for term 2

## Footnotes (Extended Syntax)

Here is a sentence with a footnote[^1].

And another with a different note[^note].

[^1]: This is the footnote content.
[^note]: This is a named footnote.

## Conclusion

This document demonstrates a wide variety of markdown features for comprehensive linting coverage.
