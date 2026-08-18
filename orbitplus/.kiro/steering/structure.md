# Project Structure

- `cmd/orbitplusmaster/`: executable entry point and dependency wiring. Keep startup, configuration loading, router construction, and server lifecycle here.
- `internal/application/master/`: application use cases and runtime configuration. HTTP-agnostic business orchestration belongs here.
- `internal/domain/`: domain-facing data models. Use lossless representations such as `json.RawMessage` where the input schema must remain flexible.
- `internal/infrastructure/http/`: HTTP router, handlers, request parsing, response serialization, and transport-specific errors. Handlers should delegate successful application work to `internal/application`.
- `configs/`, `data/`, `logs/`, `docs/`, `scripts/`, `test/`, `ui/`: reserved operational, documentation, test, and UI locations; do not introduce functionality into them without a clear need.
- `postman/`: manual API collection. Update it when externally visible endpoint behavior changes.

Follow the dependency direction: `cmd` → `infrastructure`/`application` → `domain`. Do not import HTTP concerns into domain models or application services. Use package documentation and exported-symbol comments in the established Go style; keep response helpers and route registration centralized in the HTTP package.

## Code organization

- Use concise, responsibility-based file names. Keep an established name unless it is misleading; avoid unnecessary renames.
- Split non-trivial orchestration into focused, descriptively named methods. Keep related logic together and preserve readable blank-line spacing; format Go code with `go fmt`.
- For flexible external payloads, add only validation needed for the HTTP contract, safe persistence keys, or explicitly approved behavior. Do not add schema-specific or semantic validation without a requirement.
