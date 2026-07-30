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

## Git commit attribution

Do not add `Co-Authored-By: Claude <noreply@anthropic.com>` or any other Claude co-author trailer to commit messages in this project.

When suggesting commits, use only the user's configured Git author and a normal commit subject/body.

Do not run `git add` or `git commit` in this project. The user will stage and commit changes themselves. When the user is ready to commit, provide only the suggested commit message, for example `feat: add feed cache and ranking`.
