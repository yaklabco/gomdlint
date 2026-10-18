# Link Text As Written

Code inside emphasis still counts as text: [*`here`*](https://example.com/a).

Code directly in the link text is skipped: [`here`](https://example.com/b).

HTML directly in the link text is skipped: [here <b>now</b>](https://example.com/c).

The source of an image is part of the link text: [![here](a.png)](https://example.com/d).

Strong text is compared without its markers: [**more**](https://example.com/e).

An autolink is never checked: <https://example.com/here>.

> A link in a blockquote: [link](https://example.com/f).
