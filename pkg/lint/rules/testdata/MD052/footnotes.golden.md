# Footnotes With Shortcut Syntax

A footnote call is not a reference[^note], and neither is a call to a
footnote defined right after another definition[^next].

A call to an undefined footnote is[^missing]. So is a call to a footnote
defined in a list item[^listed]: MD052 counts only the definitions that
micromark is certain to read, and goldmark reads the call as a reference.

[^note]: The definition holds text, so it is no link reference definition.
[^next]: A definition right after another one counts too.

- An item.

  [^listed]: A footnote defined in a list item.
