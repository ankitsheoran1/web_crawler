# Web crawler service

An HTTP service that accepts a seed URL and crawls same-host HTML pages with configurable depth, worker concurrency, and per-URL cooldown. Crawls run **asynchronously**: submitting a job returns a UUID and `kind`; results are available later via `GET /v1/jobs/{id}`.

## Requirements

- [Go](https://go.dev/dl/) 1.22 or newer (for `http.ServeMux` route patterns and `PathValue`)

## Quick start

1. **Clone and enter the repository**

   ```bash
   cd web_crawler
   ```

2. **Configure** (optional)

   Edit `config.yaml` or copy it and point the app at your file:

   ```bash
   export CRAWLER_CONFIG=/absolute/path/to/config.yaml
   ```

   Environment variables prefixed with `CRAWLER_` still **override** values from the YAML file after it is loaded. See [Configuration](#configuration).

3. **Build and run**

   Using Make:

   ```bash
   make build
   make run
   ```

   Or without Make:

   ```bash
   go build -o bin/server ./cmd/server
   CRAWLER_CONFIG=config.yaml ./bin/server
   ```

   The server listens on the address in `config.yaml` → `server.addr` (default `:8080`).

## Docker

Build:

```bash
docker build -t web_crawler .
```

Run:

```bash
docker run --rm -p 8080:8080 web_crawler
```

Notes:
- `use_browser=true` uses Chromium inside the container (installed via apt).
- You can override config with `-e CRAWLER_CONFIG=/path` and bind-mount a file.

## Makefile

| Target    | Description |
|-----------|-------------|
| `make build` | Compiles `./cmd/server` to `bin/server` |
| `make run`   | Builds then runs with `CRAWLER_CONFIG=config.yaml` (override with `make run CONFIG=...`) |
| `make tidy`  | `go mod tidy` |
| `make vet`   | `go vet ./...` |
| `make fmt`   | `go fmt ./...` |
| `make clean` | Removes the `bin/` directory |
| `make help`  | Prints available targets |

Examples:

```bash
make run CONFIG=./config.yaml
BINARY=bin/crawler make build
```

## Configuration

### YAML (`config.yaml`)

Default path: **`config.yaml`** in the current working directory. Set a different file with:

```bash
export CRAWLER_CONFIG=/path/to/config.yaml
```

Example structure:

```yaml
crawler:
  max_depth: 3          # 0 = only the seed URL
  workers: 10           # max concurrent HTTP fetches
  url_cooldown: 500ms   # min time between GETs to the same normalized URL
  http_timeout: 60s    # per-request HTTP client timeout
  request_timeout: 300s # max time for one background crawl job

server:
  addr: ":8080"
```

Durations use the same strings as Go’s [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) (e.g. `500ms`, `60s`, `5m`).

If `max_depth` or `workers` are omitted from YAML, defaults are **3** and **10**. If duration fields are omitted, built-in defaults match the example above.

### Environment overrides

After YAML is loaded, these variables may override values (all optional):

| Variable | Effect |
|----------|--------|
| `CRAWLER_CONFIG` | Path to YAML file (default: `config.yaml`) |
| `CRAWLER_MAX_DEPTH` | Integer ≥ 0 |
| `CRAWLER_WORKERS` | Integer ≥ 1 |
| `CRAWLER_URL_COOLDOWN` | Duration string |
| `CRAWLER_HTTP_TIMEOUT` | Duration string |
| `CRAWLER_SERVER_ADDR` | Listen address (e.g. `:9090`) |
| `CRAWLER_REQUEST_TIMEOUT` | Per-job deadline duration |

Invalid env values are ignored (YAML value is kept).

## HTTP API

### Submit crawl (async)

**`POST /v1/crawl`**

Request body (JSON):

| Field | Required | Description |
|-------|----------|-------------|
| `url` | yes | Seed URL (`http` / `https`, with host) |
| `max_depth` | no | Overrides default from config |
| `workers` | no | Overrides default from config |
| `url_cooldown` | no | Duration string; overrides default |

**Response** `202 Accepted`:

```json
{
  "id": "<uuid>",
  "kind": "crawl",
  "status": "pending"
}
```

Example:

```bash
curl -s -X POST http://localhost:8080/v1/crawl \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com","max_depth":2,"workers":8,"url_cooldown":"1s"}'
```

### Get job status and result

**`GET /v1/jobs/{id}`**

Returns the full job document: `status` (`pending`, `running`, `completed`, `failed`, `cancelled`), `params`, optional `result` with `urls` and `errors`, timestamps, and `error` on failure.

```bash
curl -s http://localhost:8080/v1/jobs/<id-from-submit>
```

## Project layout

```
cmd/server/main.go     # process entry: config, server, graceful shutdown
internal/api/          # HTTP handlers (crawl submit, job lookup)
internal/config/       # YAML + env merge and validation
internal/crawler/      # depth-limited crawl, workers, URL cooldown
internal/repo/         # in-memory job store
internal/service/      # async job execution
internal/web/          # URL helpers and HTTP client helper
config.yaml            # default configuration file
docs/                  # design docs (Parts 2+)
Makefile
```

## Docs

- `docs/design-doc.md`: Part 2+3 system design and POC plan

## License

See [LICENSE](LICENSE) in the repository.
