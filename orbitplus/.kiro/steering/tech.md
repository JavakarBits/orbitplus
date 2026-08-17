# Technology

- Go module: `orbitplusmaster`; Go 1.22.
- HTTP uses the Go standard library (`net/http`) and `http.ServeMux`; there are no third-party Go dependencies.
- JSON handling uses `encoding/json`; process logs use `log`.
- Container builds use a multi-stage Go 1.22 image and run the static binary as a non-root distroless user.
- Runtime configuration: `APP_ENV` (default `development`) and `MASTER_API_PORT` (default `8082`).

## Common commands

Run these from the repository root:

```powershell
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/orbitplusmaster
$env:APP_ENV="development"; $env:MASTER_API_PORT="8082"; go run ./cmd/orbitplusmaster
docker compose up --build
```

Run `go fmt` and relevant tests after Go changes. Prefer standard-library solutions unless a new dependency is explicitly justified and approved.
