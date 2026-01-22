# Reference Definitions in Code Blocks

This test verifies that reference-definition-like patterns inside code blocks
are not mistakenly flagged as markdown reference definitions.

## JavaScript Example

```javascript
import { Resource } from '@opentelemetry/resources';
import { SemanticResourceAttributes } from '@opentelemetry/semantic-conventions';

const resource = Resource.default().merge(new Resource({
  [SemanticResourceAttributes.SERVICE_NAME]: 'my-app',
  [SemanticResourceAttributes.SERVICE_VERSION]: '1.0.0',
}));
```

## Python Example

```python
config = {
    [key]: value
    for key, value in items.items()
}
```

## Valid Reference Usage

Here is a [link to example][example-ref].

[example-ref]: https://example.com

## Unused Reference (should be flagged)

[unused-ref]: https://unused.example.com
