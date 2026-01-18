# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` holds entrypoints: `webserver/`, `worker/`, and `extractor/` binaries.
- `internal/` contains core packages (API handlers, adapters, extractors, config, models, validators).
- `proto/` defines API specs (`specs.proto`); generated Go/TS code lives under `internal/api/http/pb/` and `frontend/wizard-vue/src/urlmaker/proto/`.
- `frontend/wizard-vue/` is the Vue + Vite wizard UI.
- `deploy/` provides Dockerfiles, `docker-compose.yml`, and runtime config.
- `presets/` contains shared feed presets and docs.

## Build, Test, and Development Commands
- `go mod download` installs Go deps.
- `go test ./...` runs Go unit tests (see `*_test.go`).
- `make proto` regenerates Go and TS protobufs after editing `proto/specs.proto`.
- `make update_adblock` refreshes blocklists (requires network).
- `cd frontend/wizard-vue && npm install` installs UI deps.
- `cd frontend/wizard-vue && npm run dev` starts the Vite dev server.
- `cd deploy && docker-compose up -d` runs the full stack locally.

## Coding Style & Naming Conventions
- Go: standard `gofmt` formatting; package names are lowercase (`internal/limiter`).
- Tests: Go test files use `*_test.go`.
- Frontend: TypeScript/Vue; lint with `npm run lint`, format with `npm run format`.
- File naming: keep modules descriptive and lowercase; new CLI entrypoints belong in `cmd/<name>/`.

## Testing Guidelines
- Primary tests are Go unit tests under `internal/**`.
- Run `go test ./...` before submitting backend changes.
- No dedicated frontend test runner is configured; validate UI changes with `npm run dev` and manual checks.

## Commit & Pull Request Guidelines
- Commit messages are short, lowercase, and descriptive (e.g., "small refactoring", "updated dependencies").
- PRs should explain scope, link relevant issues, and include steps to verify.
- Include screenshots or recordings for UI changes in `frontend/wizard-vue/`.

## Security & Configuration Notes
- Runtime config is via environment variables; see `internal/config/config.go` and `deploy/.env`.
- Dev requires Go 1.23, Node.js 20, NATS (JetStream), and Redis.
