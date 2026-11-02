# CenterCDP

CenterCDP is a local-first customer data platform exposed through the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/). It uses SQLite for storage and gives MCP clients typed tools for customer profiles, events, segments, campaigns, and automated journeys.

The same server also exposes three general-purpose SQLite tools for schema inspection, read-only queries, and database administration.

## What it provides

- Isolated customer databases for separate businesses or brands
- Versioned data dictionaries for contact attributes and event parameters
- Identity-aware contact upserts, consent tracking, and idempotent event ingestion
- Rule-based and constrained read-only SQL segments
- Cross-database segment previews with per-database results
- Email, SMS, and push templates and campaigns
- Durable customer journeys with triggers, delays, conditions, and message actions
- A transactional message outbox for integration with delivery providers
- Streamable HTTP MCP transport and a health endpoint

## Quick start

### Prerequisites

- Go 1.26 or later
- [Task](https://taskfile.dev/) and the `sqlite3` CLI for the task-based workflow
- `golangci-lint` for `task lint` and `task check`
- Docker, optionally

### Build and run

From the repository root:

```bash
task build
./build/sqlite-mcp --database ./build/example.db
```

`task build` creates an example SQLite database and writes the server binary to `build/sqlite-mcp`.

To build without Task:

```bash
mkdir -p build
go build -o ./build/sqlite-mcp ./cmd/server
./build/sqlite-mcp --database ./build/centercdp.db
```

The server listens on `http://0.0.0.0:3000/mcp` by default. Check readiness at:

```bash
curl http://localhost:3000/health
```

### Connect an MCP client

Start the server, then configure a client that supports Streamable HTTP:

```json
{
  "mcpServers": {
    "centercdp": {
      "url": "http://localhost:3000/mcp"
    }
  }
}
```

The exact configuration file and shape depend on the MCP client.

## Server options

```text
--database, -d  Path to the default SQLite database (required)
--data-dir      Directory for additional managed customer databases
--address       Listen address (default: ${HOST:-0.0.0.0}:${PORT:-3000})
--endpoint      MCP endpoint path (default: /mcp)
--debug         Enable debug logging
```

The parent directory of `--database` must already exist. The database file itself is created when necessary.

If `--data-dir` is omitted, managed customer databases are stored next to the default database in `<database-name>-customer-databases/`. The registry is stored at `<database-path>.registry.db`.

## MCP tools

### Customer data platform

| Area | Tools |
| --- | --- |
| System | `get_code_version` |
| Databases | `create_customer_database`, `list_customer_databases`, `gen_demo_data`, `reset_demo_data`, `create_hotel_sample_data` |
| Data dictionary | `get_data_dictionary`, `define_contact_attribute`, `define_event` |
| Contacts and events | `upsert_contact`, `get_contact`, `list_contacts`, `search_contacts`, `track_event` |
| Segments | `create_segment`, `list_segments`, `preview_segment`, `preview_segments_across_databases` |
| Messaging | `save_message_template`, `list_message_templates`, `create_campaign`, `list_campaigns`, `launch_campaign`, `list_message_outbox` |
| Journeys | `create_journey`, `list_journeys`, `publish_journey`, `pause_journey` |

Every CDP tool is explicitly scoped by `database_id`. The database supplied with `--database` is registered as `default`; databases created through `create_customer_database` receive their own ID and SQLite file.

Collection tools return structured data and are bound to a shared MCP Apps table resource. Compatible clients therefore render contacts, contact searches, segments, templates, campaigns, journeys, databases, and outbox messages as deterministic tables instead of asking the model to choose a presentation. Paginated list tools accept `page` and `page_size` (20 rows by default, 100 maximum). Each response exposes at most three pages to the model. Previous/Next navigation stays inside that local buffer and loads the adjacent three-page block only when the user crosses a buffer boundary.

`get_code_version` reports the semantic feature version, the exact Git revision embedded by the Go toolchain, and a `modified` flag for builds made from uncommitted changes. The semantic version lives in `internal/appversion/version.go`; bump its minor component for every feature and its patch component for fixes.

Contact attributes and event parameters must be defined before data is ingested. Supported data types are `string`, `integer`, `number`, `boolean`, `date`, `datetime`, `enum`, `object`, and `array`.

`gen_demo_data` creates a deterministic sample dataset in the selected database, including 300 varied contacts, behavioral events, segments, templates, a completed campaign, and a draft journey. It first replaces any prior generated demo dataset, so repeated calls do not create duplicates. `reset_demo_data` deletes records in the reserved `demo_` ID/key namespace and leaves non-demo data intact. Both tools return per-table counts.

### Hotel sample data through MCP

Create an empty hotel database with `create_customer_database`:

```json
{"name":"CenterBKK Hotel"}
```

Then call **`create_hotel_sample_data`** using the returned database ID:

```json
{"database_id":"<returned database ID>","as_of":"2026-11-03T18:00:00+07:00"}
```

`as_of` is optional and defaults to the current time. The tool atomically adds the dictionary, 80 synthetic Center BKK Hotel guest contacts, 80 `check_in`, 20 `check_out`, and 47 `restaurant_order` events. Of the 80 guests, 60 are in-house and 20 have checked out. No messages are sent or marketing consent granted. Existing contacts, events, dictionary definitions, or segments cause an error; repeated calls never overwrite existing data.

The response includes three validated SQL segments and ready-to-call `search_contacts` arguments:

| Attribute key | Audience | Count |
| --- | --- | --- |
| `first_night` | Checked in on the snapshot date in Asia/Bangkok and still in-house | 20 |
| `no_restaurant` | Still in-house without a restaurant order for this stay | 25 |
| `recent_checkout` | Checked out in (snapshot minus 24 hours, snapshot] | 15 |

To display one audience as the CDP table, call `search_contacts`:

```json
{"database_id":"<returned database ID>","attribute_key":"no_restaurant","value":true,"page":1,"page_size":100}
```

Use `first_night` or `recent_checkout` for the other lists, and `list_contacts` for all 80 guests. Restaurant usage is correlated by `stay_id`. Both audience flags and SQL segment parameters are frozen at `as_of`; they do not update as time passes. Use a new empty database for a new snapshot.

### SQLite administration

| Tool | Purpose |
| --- | --- |
| `get_schema` | Return tables, columns, constraints, indexes, and foreign keys |
| `query` | Run a statement beginning with `SELECT`, `WITH`, or `EXPLAIN` |
| `execute` | Run DDL or DML such as `INSERT`, `UPDATE`, `DELETE`, `CREATE`, or `ALTER` |

The raw SQL tools operate on the database passed with `--database`. Prefer the typed CDP tools for customer data workflows.

## Typical CDP workflow

1. Use `database_id: "default"` or create an isolated database for a brand.
2. Define contact attributes and events in the database's data dictionary.
3. Upsert contacts and consent state, then track customer events.
4. Create and preview a rule or SQL segment.
5. Save an active message template.
6. Create and launch a campaign, or create and publish a journey.
7. Consume pending records from `list_message_outbox` with a delivery adapter.

### Segments

Rule segments validate referenced fields and events against the data dictionary. A contact field must be marked filterable before it can be used in a rule.

SQL segments are an advanced escape hatch. They:

- run through a separate read-only SQLite connection;
- accept named parameters;
- must contain one `SELECT` or `WITH` statement;
- must return exactly one column named `contact_id`;
- have a five-second evaluation timeout; and
- are limited to 100,000 contacts.

### Campaigns and delivery

Launching a campaign materializes an audience snapshot and checks channel-specific marketing consent, such as `email_marketing`, `sms_marketing`, or `push_marketing`. Eligible messages are added to `message_outbox`.

CenterCDP does not send messages directly. A delivery adapter should consume the outbox and update delivery state in the database.

### Journeys

Published journeys run as durable state machines. The current journey model supports:

- `contact_created` and defined-event triggers;
- `send_message` actions;
- delays in minutes, hours, or days;
- `event_occurred` conditions; and
- end steps.

The background worker checks due enrollments every two seconds. Delay deadlines are persisted, so scheduled work survives a server restart. A published journey version is immutable; pausing it stops new enrollments.

## Docker

Build and run the image with the bundled example database:

```bash
task docker-build
docker run --rm -p 127.0.0.1:3000:3000 sqlite-mcp-server
```

To initialize the bundled database with an additional schema file:

```bash
docker run --rm \
  -p 127.0.0.1:3000:3000 \
  -v "/path/to/schema.sql:/data/schema.sql:ro" \
  sqlite-mcp-server
```

The container stores its default database inside the container. Mount a host directory and pass `--database` and `--data-dir` when persistence is required:

```bash
docker run --rm \
  -p 127.0.0.1:3000:3000 \
  -v "$PWD/data:/state" \
  sqlite-mcp-server \
  --database /state/centercdp.db \
  --data-dir /state/customer-databases
```

## Development

List all available tasks:

```bash
task --list
```

Common commands:

```bash
task fmt           # Tidy modules and format Go code
task lint          # Run golangci-lint
task test          # Run unit tests
task check         # Run formatting, linting, and tests
task run-dev       # Run from source with the example database
task build         # Build the example database and binary
task docker-build  # Build the local Docker image
```

Tests can also be run directly:

```bash
go test ./...
```

## Security and deployment

The HTTP server does not provide authentication or TLS, and the `execute` tool can modify or destroy data. Keep the service bound to localhost for local development.

For shared or remote deployments:

- place the server behind authenticated TLS;
- authorize access to each `database_id` before forwarding requests;
- restrict or remove the raw `execute` tool if clients do not need administration access;
- protect database files and backups as sensitive customer data; and
- run only one trusted delivery adapter against the message outbox.
