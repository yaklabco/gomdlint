# Autolink Literals

Trailing punctuation stays outside: https://example.com/path.

Balanced parentheses stay inside: https://en.wikipedia.org/wiki/Foo_(bar).

An unbalanced parenthesis stays outside (see https://example.com/a).

An email address ends before its final period: user@example.com.

Link text is not bare: [https://example.com/docs](https://example.com/docs).

Bracketed text is not bare: [https://example.com/shortcut].

Code is not bare: `https://example.com/code`.

Inline HTML content is not bare: <span>https://example.com/html</span>.

A www address is reported without a fix: www.example.com/docs.
