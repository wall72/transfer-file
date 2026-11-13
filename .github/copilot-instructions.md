# Copilot instructions for transfer-file

## What this is

A single-binary WebDAV server (Go, `golang.org/x/net/webdav`) for ad-hoc
PC-to-PC file transfer on a trusted LAN. No auth, no TLS — by design, not
an oversight. All logic lives in `main.go`; there is no package layout to
navigate.

## Build & run

```powershell
go build -o transfer-file.exe .
./transfer-file.exe -dir ./shared -port 8081 [-readonly]
```

- `go vet ./...` — static checks (no test suite exists yet; add `_test.go`
  files and use `go test ./... -run TestName` for a single test if tests
  are introduced).
- `go fmt ./...` before committing.
- Requires Go 1.21+ (uses `golang.org/x/net/webdav`, pulled via `go.mod`).

## Architecture

- `main()` wires three pieces: the `webdav.Handler` (serves `-dir` as the
  DAV root via `webdav.Dir`), an optional read-only wrapper
  (`withReadOnly`), and startup diagnostics (`lanURLs`) that print every
  non-loopback IPv4 address so the user knows what URL to type on the
  *other* PC.
- `withReadOnly` is a `http.Handler` middleware that blocks the WebDAV
  write verbs (`PUT`, `DELETE`, `MKCOL`, `COPY`, `MOVE`, `PROPPATCH`)
  before delegating to the real handler — it does not touch the
  filesystem layer.
- The shared directory is created with `os.MkdirAll` if missing and
  resolved to an absolute path before being handed to `webdav.Dir`.

## Conventions

- Keep it to a single `main.go` unless a feature genuinely needs its own
  file — this project intentionally avoids internal packages.
- Flags only (`-dir`, `-port`, `-readonly`); don't add a config file
  format unless requested.
- Any new write-restricting behavior must be added to the
  `writeMethods` set in `withReadOnly`, not as a separate handler.
- `./shared/` (the default data dir) and built binaries are gitignored —
  never commit transferred files or `transfer-file.exe`.
