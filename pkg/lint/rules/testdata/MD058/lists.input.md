# Tables in Lists

A table in a tight list item, directly after the item's text:

- item
  | a | b |
  | --- | --- |
  | 1 | 2 |
- next

A table in a nested tight list:

- outer
  - item
    | a | b |
    | --- | --- |
  - next

A list in a blockquote:

> - item
>   | a | b |
>   | --- | --- |
> - next

A table in a blockquote in a list item:

- item
  > | a | b |
  > | --- | --- |
- next

A table in a loose list item, directly after the item's text:

- item

  text
  | a | b |
  | --- | --- |

- next

A table that ends an item of a loose list:

- item

  | a | b |
  | --- | --- |
- next

A tight list in a loose list:

- outer

  - item
    | a | b |
    | --- | --- |
  - next

- last

A loose list in a tight list:

- outer
  - item

    text
    | a | b |
    | --- | --- |
  - next
- last

A table at the end of the last item, directly before a heading:

- first
- item
  | a | b |
  | --- | --- |
## After the List

The end.
