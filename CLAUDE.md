# CLAUDE.md

## Backend command location

When working with the Go backend, run module-related commands from:

```bash
E:\code\feed-videos-system\backend
```

Use that directory for:

- `go mod tidy`
- `gofmt -w ./cmd ./internal`
- `go test ./...`
- `go run ./cmd/api`

Do not run these from the repository root unless the command explicitly targets the root-level files.

## API verification preference

When manual API verification is needed and no test file is being used, provide Apifox request setup instead of curl-first terminal commands.

Prefer giving the user:

- Request method and URL.
- Required headers.
- Body type and JSON payload.
- Auth setup, especially `Authorization: Bearer <token>`.
- Suggested Apifox environment variables such as `baseUrl` and `token`.
- The expected response shape or key fields to check.

Use curl only when the user asks for terminal commands, when a scriptable reproduction is necessary, or when comparing command-line output matters.
