# Fence Runs

The first fence sets the style:

```text
first
```

A longer closing run is rewritten whole:

~~~~~ js
let x = 1;
~~~~~~~~

A block whose content holds backtick fences keeps its tildes:

~~~markdown
```
nested
```
~~~

An info string with a backtick cannot follow a backtick fence:

~~~ a`b
info
~~~

- A fence inside a list item:

  ~~~
  in list
  ~~~
