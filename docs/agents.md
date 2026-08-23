# Agent memory

After any change to `cliamp-clify`, always rebuild the latest binary and update
the repository `bin/` artifact before handing the work back:

```sh
cd cliamp-clify
GOCACHE=/tmp/clify-go-cache go build -trimpath \
  -ldflags="-s -w -X main.version=dj-mode-dev" \
  -o bin/cliamp-clify .
```

Verify it with:

```sh
./bin/cliamp-clify --version
```

When the user asks to test via `cliamp`, also update the user-local binary
`~/.local/bin/cliamp` from `bin/cliamp-clify` and verify `cliamp --version`.
