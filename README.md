# Relay

Real-time chat backend in Go. WebSocket messaging, Redis caching, JWT auth, PostgreSQL persistence, Prometheus metrics.

Built to explore distributed systems patterns at a practical level — caching, pub/sub fanout, idempotency, rate limiting, observability — while keeping the codebase small enough to reason about.

## What it does

- WebSocket-based real-time messaging with per-conversation subscriptions
- Distributed message delivery via Redis Pub/Sub (or in-memory fallback for single-node)
- Redis-cached participant membership for fast authorization on every message send
- Client-side message deduplication (idempotency keys) to handle retries safely
- Presence tracking with TTL-based heartbeats
- Cursor-based pagination for message history
- Per-endpoint rate limiting (sliding window, Redis-backed)
- JWT auth with bcrypt password hashing
- Prometheus metrics for HTTP, WebSocket, DB, cache, and rate-limiter layers

## How messages flow

```
Client sends WebSocket frame
  → readPump parses + validates
  → idempotency check (Redis SETNX, 5min TTL)
  → participant check (Redis SISMEMBER, cached SET per conversation)
  → DB insert (PostgreSQL)
  → publish to Redis Pub/Sub channel
  → all subscribers' writePumps push to connected clients
```

On cache miss for participant check, the system loads the full member list from the DB, populates the Redis SET, and serves subsequent checks from cache (1h TTL). Cache is updated incrementally on join/leave.

## Running it

Prerequisites: Go 1.25+, PostgreSQL, Redis (optional).

```bash
cp .env.example .env  # fill in your values
go run ./cmd/server
```

Migrations run on startup via Ent. If Redis isn't available, the app starts anyway using in-memory alternatives.

### Local development (full stack)

```bash
docker-compose -f docker-compose.local.yml up --build
```

This runs:
- **App** on :8080
- **PostgreSQL** on :5432
- **Redis** on :6379
- **Prometheus** on :9090 (metrics UI)
- **Grafana** on :3000 (dashboards, login: admin/admin)

### Production (single container)

```bash
docker compose up --build
```

Uses `Dockerfile` with embedded Redis for single-service deployments (e.g., Render free tier).

## Environment

```env
PORT=8080
ENV=development
DB_SOURCE=postgres://user:pass@localhost:5432/relay?sslmode=disable
JWT_SECRET=change_this_minimum_32_characters
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
WS_ALLOWED_ORIGINS=http://localhost:5173
TRUSTED_PROXIES=0
```

## API

| Method | Path | What |
|--------|------|------|
| POST | `/api/signup` | Register |
| POST | `/api/login` | Login (sets JWT cookie) |
| POST | `/api/logout` | Logout |
| GET | `/api/me` | Current user |
| POST | `/api/conversation/create` | Create group or private conversation |
| GET | `/api/conversation/list` | User's conversations |
| POST | `/api/conversation/join` | Join group conversation |
| POST | `/api/conversation/leave` | Leave conversation |
| GET | `/api/conversation/members` | Conversation participants |
| GET | `/api/conversation/messages` | Message history (cursor pagination) |
| GET | `/api/users/search` | Search users by username |
| GET | `/api/presence` | Online status |
| GET | `/api/ws` | WebSocket upgrade |
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |

## WebSocket protocol

Messages from client:
```json
{"type": "subscribe", "conversation_id": "uuid"}
{"type": "unsubscribe", "conversation_id": "uuid"}
{"type": "message", "conversation_id": "uuid", "content": "text", "username": "name", "client_id": "uuid"}
```

The `client_id` field enables idempotency — if the client retries with the same ID within 5 minutes, the server returns a duplicate error instead of inserting again.

Messages from server:
```json
{"type": "message", "id": "uuid", "sender_id": "uuid", "sender_username": "name", "conversation_id": "uuid", "content": "text", "created_at": "timestamp"}
{"type": "subscribe_ack", "conversation_id": "uuid"}
{"type": "unsubscribe_ack", "conversation_id": "uuid"}
{"type": "error", "error": "description"}
```

## Observability

Prometheus metrics exposed at `/metrics`:

| Metric | Type | Labels | What |
|--------|------|--------|------|
| `relay_http_total_requests` | Counter | method, path, status_code | HTTP request count |
| `relay_http_request_duration_seconds` | Histogram | method, path | HTTP latency |
| `relay_db_queries_total` | Counter | operation, status | DB query count |
| `relay_db_query_duration_seconds` | Histogram | operation | DB latency |
| `relay_cache_hits_total` | Counter | cache | Cache hits |
| `relay_cache_misses_total` | Counter | cache | Cache misses |
| `relay_cache_operations_duration_seconds` | Histogram | cache, operation | Cache latency |
| `relay_ws_connections_active` | Gauge | — | Active WebSocket connections |
| `relay_ws_messages_total` | Counter | direction | WS messages in/out |
| `relay_ws_message_processing_duration_seconds` | Histogram | type | WS message handling time |
| `relay_rate_limit_hits_total` | Counter | limiter | Rate limit rejections |

Example Grafana queries:
```promql
# p95 HTTP latency by endpoint
histogram_quantile(0.95, sum(rate(relay_http_request_duration_seconds_bucket[5m])) by (path, le))

# DB query p95 by operation
histogram_quantile(0.95, sum(rate(relay_db_query_duration_seconds_bucket[5m])) by (operation, le))

# Cache hit ratio
rate(relay_cache_hits_total[5m]) / (rate(relay_cache_hits_total[5m]) + rate(relay_cache_misses_total[5m]))

# Active WebSocket connections
relay_ws_connections_active
```

## Project layout

```
cmd/
  server/main.go              Entry point + route wiring
internal/
  auth/                       JWT creation/validation, password hashing, middleware
  config/                     Env loading + validation
  constants/                  App constants
  conversation/               Conversation handler, repository, participant cache
  domain/ent/                 Ent ORM schema + generated code
  message/                    Message handler, repository, service, cache
  metrics/                    Prometheus metric definitions + helpers
  middleware/                 CORS, rate limiting, Prometheus instrumentation
  pkg/
    httpx/                    HTTP response helpers
    logger/                   Structured logging (slog)
    validator/                Request validation with translated messages
  realtime/                   WebSocket hub, client, read/write pumps, publisher
  storage/
    postgres/                 DB connection
    redis/                    Redis connection + presence store
  testutil/                   Test containers setup
  user/                       User handler + repository
```

## Tests

```bash
go test ./...
go test -v ./internal/realtime/   # WebSocket tests
go test -v ./internal/auth/       # Auth tests
```

Integration tests use testcontainers for Postgres and Redis.

## Design notes

**Why cache participant checks?** `IsParticipant` is called on every message send. With Redis SISMEMBER it's a sub-millisecond O(1) lookup vs a DB query. Cache is populated on first miss and kept in sync incrementally.

**Why idempotency keys?** WebSocket connections drop. Clients retry. A simple `SETNX` with 5-minute TTL prevents duplicate messages.

**Why Redis Pub/Sub instead of a message queue?** For chat, fire-and-forget fanout is fine. Offline users fetch history on reconnect. Persistence handles durability, Pub/Sub handles speed.

**Fail-open Redis** — If Redis goes down, the app continues with in-memory fallback. Single-node still works; you lose multi-instance fanout and caching performance.

**Go 1.22+ route patterns** — Uses `METHOD /path` syntax for proper `r.Pattern` population in metrics middleware.

## Stack

Go 1.25 · PostgreSQL · Redis · Ent ORM · gorilla/websocket · pgx/v5 · go-redis/v9 · Prometheus · Docker
