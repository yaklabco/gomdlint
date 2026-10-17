# Dollar Signs In Any Code Block

A block with attributes in its info string:

```bash title="setup"
$ make build
$ make test
```

A PowerShell block:

```powershell
$ Get-ChildItem
```

An indented block:

    $ go test ./...

A block that shows output keeps its prompts:

```console
$ echo hello
hello
```

A dollar sign that starts a variable is not a prompt:

```sh
$HOME
```
