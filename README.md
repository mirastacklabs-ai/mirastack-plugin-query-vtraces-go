# MIRASTACK Plugin: Query VTraces

Go plugin for querying **VictoriaTraces** (Jaeger-compatible) from MIRASTACK workflows. Part of the core observability plugin suite.

Absorbs both Phase 1 (compact trace summaries) and Phase 2 (full span waterfall) trace querying into a single plugin with action-based dispatch.

The `v` prefix denotes Victoria-specific. Enterprise versions for other trace backends follow the same plugin contract: `query-jtraces` (Jaeger native), `query-ttraces` (Tempo), etc.

## Capabilities

| Action | Description |
|--------|-------------|
| `search` | Search traces by service, operation, tags, duration |
| `trace_by_id` | Retrieve full trace with span-level detail by trace ID |
| `services` | List all discovered services |
| `operations` | List operations for a service |
| `dependencies` | Get service dependency graph |

## Configuration

The engine pushes configuration via `ConfigUpdated()`:

| Key | Description |
|-----|-------------|
| `traces_url` | VictoriaTraces base URL (e.g., `http://victoriatraces:10428`) |

## Example Workflow Step

```yaml
- id: find-slow-traces
  type: plugin
  plugin: query_vtraces
  params:
    action: search
    service: "api-gateway"
    start: "-1h"
    end: "now"
    min_duration: "500ms"
    limit: "10"
```

## Development

```bash
go build -o mirastack-plugin-query-vtraces .
```

## Requirements

- Go 1.23+
- mirastack-sdk-go

## License

AGPL v3 — see [LICENSE](LICENSE).
