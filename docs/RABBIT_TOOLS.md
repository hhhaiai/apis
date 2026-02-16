# Rabbit Tools (Tool Bridge)

CC Gateway now includes built-in Rabbit tool handlers in the local tool runtime:

- `rabbit_publish`
- `rabbit_get`
- `rabbit_rpc`

These handlers call RabbitMQ Management HTTP API (`/api/...`) so you can provide tool capability even when upstream LLM APIs do not natively support tools.

## 1) Environment Variables

If inputs do not provide connection fields, the handlers fallback to env vars:

- `RABBIT_HTTP_API` (default: `http://127.0.0.1:15672/api`)
- `RABBIT_VHOST` (default: `/`)
- `RABBIT_USERNAME` (default: `guest`)
- `RABBIT_PASSWORD` (default: `guest`)

## 2) Tool Inputs

### `rabbit_publish`

Required:
- `routing_key`
- `payload`

Optional:
- `management_url`, `username`, `password`, `vhost`, `exchange`, `properties`, `timeout_ms`

### `rabbit_get`

Required:
- `queue`

Optional:
- `management_url`, `username`, `password`, `vhost`
- `count` (default 1), `ackmode` (default `ack_requeue_false`)
- `encoding` (default `auto`), `truncate` (default 65536)

### `rabbit_rpc`

Required:
- `request_queue`
- `payload`

Optional:
- `reply_queue` (if empty, gateway creates ephemeral queue)
- `management_url`, `username`, `password`, `vhost`, `exchange`
- `properties`, `correlation_id`
- `timeout_ms` (default 10000), `poll_interval_ms` (default 200)

## 3) Alias Mapping (Client Compatibility)

Use runtime setting `tool_aliases` to map client-facing names to implemented tools:

```json
{
  "tool_aliases": {
    "read_file": "file_read",
    "write_file": "file_write",
    "list_files": "file_list"
  }
}
```

When alias matches, gateway emits `tool.alias_applied` event.

## 4) Gap Suggestions

Call:

- `GET /admin/tools/gaps?include_suggestions=true`

Response includes:
- `replacement_candidates`
- `unresolved_tools`
- `mcp_tools`
- `tool_aliases`

This is used to inventory unknown/unsupported tools and migrate them gradually to MCP or local implementations.
