# Relay

Real-time chat backend in Go. WebSocket messaging, Redis caching, JWT auth, PostgreSQL persistence.

Built to explore distributed systems patterns at a practical level — caching, pub/sub fanout, idempotency, rate limiting — while keeping the codebase small enough to reason about.

## What it does

- WebSocket-based real-time messaging with per-conversation subscriptions
- Distributed message delivery via Redis Pub/Sub (or in-memory fallback for single-node)
- Redis-cached participant membership for fast authorization on every message send
- Client-side message deduplication (idempotency keys) to handle retries safely
- Presence tracking with TTL-based heartbeats
- Cursor-based pagination for message history
- Per-endpoint rate limiting (sliding window, Redis-backed)
- JWT auth with bcrypt password hashing

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

Migrations run on startup. If Redis isn't available, the app starts anyway using in-memory alternatives.

Docker:
```bash
docker compose up --build
```

## Environment

```env
PORT=8080
ENV=development
DB_SOURCE=postgres://user:pass@localhost:5432/relay?sslmode=disable
JWT_SECRET=change_this
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
WS_ALLOWED_ORIGINS=http://localhost:5173
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
| GET | `/api/conversation/messages` | Message history (cursor pagination) |
| GET | `/api/users/search` | Search users by username |
| GET | `/api/ws` | WebSocket upgrade |
| GET | `/api/presence` | Online status |

## WebSocket protocol

Messages from client:
```json
{"type": "subscribe", "conversation_id": "uuid"}
{"type": "unsubscribe", "conversation_id": "uuid"}
{"type": "message", "conversation_id": "uuid", "content": "text", "username": "name", "client_id": "uuid"}
```

The `client_id` field enables idempotency — if the client retries with the same ID within 5 minutes, the server returns a duplicate error instead of inserting again. This makes at-least-once delivery safe on unreliable connections.

Messages from server:
```json
{"type": "message", "id": "uuid", "sender_id": "uuid", "sender_username": "name", "conversation_id": "uuid", "content": "text", "created_at": "timestamp"}
{"type": "subscribe_ack", "conversation_id": "uuid"}
{"type": "unsubscribe_ack", "conversation_id": "uuid"}
```

## Project layout

```
main.go                     Entry point + wiring
config/                     Env loading
db/                         Postgres connection
  redis/                    Redis connection + presence store
handler/                    HTTP handlers
internal/
  cache/                    Message cache (Redis ZSET + memory fallback)
  realtime/                 WebSocket hub, read/write pumps, rate limiter
middleware/                 JWT, CORS, rate limiting
repository/                 SQL queries (pgx)
service/
  message_service.go        Message creation, idempotency, participant check
  participant_cache.go      Redis SET-based membership cache
  cached_message_service.go Write-through message cache with singleflight
  redis_publisher.go        Redis Pub/Sub event bus
helper/                     JWT, bcrypt, migrations
logger/                     Structured logging (slog)
```

## Tests

```bash
go test ./...
go test -v ./internal/realtime/   # WebSocket integration
go test -v -run TestE2E           # end-to-end
```

## Design notes

**Why cache participant checks?** `IsParticipant` is called on every single message send. That's a `SELECT EXISTS` against PostgreSQL for every frame. With Redis SISMEMBER it's a sub-millisecond O(1) lookup. Cache is populated on first miss and kept in sync incrementally (SADD on join, SREM on leave).

**Why idempotency keys?** WebSocket connections drop. Clients retry. Without dedup, you get duplicate messages in the DB and double-delivered to other users. A simple `SETNX` with a 5-minute TTL solves this cheaply.

**Why Redis Pub/Sub instead of a message queue?** For a chat app, fire-and-forget fanout is fine. If a subscriber is offline, they'll fetch history on reconnect via the REST endpoint. No need for guaranteed delivery at the transport layer — persistence handles durability, Pub/Sub handles speed.

**Fail-open Redis** — If Redis goes down, the app continues with in-memory local bus, local participant checks (direct DB), and memory cache. Single-node still works. You lose multi-instance fanout and caching performance, but nothing breaks.

## Stack

Go 1.25 · PostgreSQL · Redis · gorilla/websocket · pgx/v5 · go-redis/v9 · Docker
