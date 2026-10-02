# Contributing

Thanks for helping improve Yanki.

## Requirements

- Go 1.24 or newer.
- The [Anki desktop application](https://apps.ankiweb.net) with the
  [AnkiConnect](https://ankiweb.net/shared/info/2055492159) add-on, if you want
  to exercise a real sync.

## Development

```sh
go build ./...              # build everything
go test ./...               # run the test suite
go vet ./...                # static checks
gofmt -l ./cmd ./internal   # formatting check
```

The sync tests use an in-memory fake AnkiConnect server, so they run without
Anki. When you change behavior that talks to Anki, prefer extending that fake
over requiring a desktop install.

## Layout

| Path               | Purpose                                                        |
| ------------------ | -------------------------------------------------------------- |
| `cmd/yanki`        | The CLI entry point.                                           |
| `internal/anki`    | Typed AnkiConnect client.                                      |
| `internal/note`    | Note models, frontmatter, and namespace rules.                 |
| `internal/markdown`| Markdown → note conversion and HTML rendering.                 |
| `internal/vault`   | File discovery and deck inference.                             |
| `internal/config`  | Defaults, TOML loading, and flag overrides.                    |
| `internal/sync`    | Reconciliation of local notes with Anki.                       |
| `internal/cli`     | Command definitions and output formatting.                     |

## Pull requests

- Keep changes focused, and add or update tests for behavior changes.
- Run `gofmt`, `go vet`, and `go test ./...` before opening a pull request.
- Match the surrounding code style: small packages, explicit types, and
  comments that explain why rather than what.
