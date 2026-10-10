# Repository Guidelines

## Project Structure & Module Organization
- `cmd/cc-gateway/`: Go service entrypoint.
- `internal/`: core backend modules (`gateway/`, `upstream/`, `settings/`, `scheduler/`, etc.).
- `tests/`: all Go tests, organized by module (for example `tests/gateway/`, `tests/upstream/`).
- `web/admin/`: Vue + Vite admin frontend.
- `scripts/`: local run/build helpers and adapter examples (`scripts/script-adapters/`).
- `docs/`: contributor/user documentation. Keep long-form docs here, not in `scripts/`.
- `configs/`: example runtime and upstream configs.

## Build, Test, and Development Commands
- `go test ./...`: run full backend test suite.
- `bash scripts/build_run_gateway.sh`: build (UI + backend) and start gateway.
- `bash scripts/build_run_gateway.sh --no-ui --port 18080`: backend-only local run.
- `source <(bash scripts/use-glm-local-env.sh)`: load local GLM routing env vars.
- Frontend only:
  - `cd web/admin && npm run dev`
  - `cd web/admin && npm run build`

## Coding Style & Naming Conventions
- Go version: `1.22` (`go.mod`).
- Format Go code with `gofmt` before committing.
- Use clear package boundaries under `internal/`; avoid cross-module leakage.
- Naming:
  - files/packages: lowercase, concise (`modelmap`, `toolruntime`).
  - exported Go identifiers: `CamelCase`; internal helpers: `camelCase`.
- Keep docs and examples aligned with actual endpoints and scripts.

## Testing Guidelines
- Test framework: Go standard `testing` package.
- Put all test files under `tests/` using `*_test.go`.
- Prefer module-scoped tests mirroring `internal/` layout.
- For feature changes, add/adjust tests in the matching folder (for example gateway logic -> `tests/gateway/`).

## Commit & Pull Request Guidelines
- Current history uses short, timestamped commit messages; prefer clearer messages going forward.
- Recommended format: `<scope>: <imperative summary>` (example: `gateway: add admin upstream update endpoint`).
- PRs should include:
  - change summary and motivation,
  - affected paths/endpoints,
  - test evidence (`go test ./...`),
  - screenshots for `web/admin` UI changes.

## Security & Configuration Tips
- Never commit real API keys; use env vars and local shell exports.
- Admin auth uses `ADMIN_TOKEN`; do not use default token in production.
- Runtime logs are ignored via `.gitignore`; keep `logs/.gitkeep` so `logs/` exists.
