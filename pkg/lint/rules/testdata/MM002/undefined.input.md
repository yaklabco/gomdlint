# Test Undefined References

GitGraph with undefined branch reference:

```mermaid
gitGraph
    commit
    branch develop
    checkout develop
    commit
    checkout undefined-branch
```
