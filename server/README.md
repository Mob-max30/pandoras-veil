# server/ — Backend Relay

Pavan (Backend Relay Lead)

Go HTTP relay for Pandora's Veil. Stores opaque ciphertext and public-key
strings in Redis; never sees plaintext or private keys; implements no
cryptography itself. Enforces two lifecycle rules — TTL expiry and atomic
burn-after-reading — per the frozen API contract in the top-level README.

## Run locally (no Docker)

```bash
# needs a Redis instance reachable at localhost:6379 (or set PANDORA_REDIS_ADDR)
go run .
```

## Run with Docker Compose (relay + Redis, one command)

```bash
docker compose up --build
```

Relay is then reachable at `http://localhost:8080`.

## Test

```bash
go test ./...
```

Handler tests run against an in-memory fake store (`internal/api/handlers_test.go`),
including a race-style assertion that burn-after-reading pastes are gone
after exactly one read. No live Redis needed to run `go test`.

## Routes

| Method | Path | Notes |
| :--- | :--- | :--- |
| `POST` | `/keys` | `handle` optional — server generates `veil-<hex>` if omitted. `409` on collision. |
| `GET` | `/keys/{handle}` | `404` if unregistered. |
| `POST` | `/paste` | `ttl_seconds` optional (clamped to `[MIN,MAX]`, default `PANDORA_DEFAULT_TTL_SECONDS`). `ciphertext` must be base64, size-capped by `PANDORA_MAX_CIPHERTEXT_BYTES`. |
| `GET` | `/paste/{id}` | Burn-after-reading vs. TTL-only is read from the ID itself (see `internal/idgen`) — no separate flag needed on the read request. |
| `GET` | `/health` | Pings Redis; `503` if unreachable. |

## Why burn-after-reading is encoded in the ID

`GET /paste/:id` has no request body in the frozen contract, so the handler
can't be told "treat this as a burn read" at fetch time. Storing the flag
alongside the ciphertext and doing a conditional `GET` + `DEL` would reopen
a race: two concurrent readers could both pass the check before either
deletes. Instead, `idgen.New` prefixes the ID itself with `b` (burn) or `p`
(persist), so a single Redis command — `GETDEL` or `GET` — is chosen
correctly and atomically from the ID alone. See `internal/idgen/idgen.go`
and `internal/store/redis.go` for the full reasoning.

## Package layout

```
server/
├── main.go                      entrypoint: config → store → handlers → HTTP server, graceful shutdown
└── internal/
    ├── config/     env-driven config, all defaults sane for zero-config local runs
    ├── store/      Redis operations: key registration, paste TTL + atomic GETDEL
    ├── api/        HTTP handlers, router, request/response DTOs (the frozen contract)
    ├── fingerprint/  SHA-256 8-hex-char fingerprint formatting, shared by /keys handlers
    └── idgen/      share ID generation, including the burn/persist prefix encoding
```

## Coordinating changes

Per the top-level README's mandatory instructions: do not change the
`/keys` or `/paste` request/response field names or status codes without
reopening Coordination Point 1 with Pranav and Ujwal — the CLI and crypto
layers are written against these exact shapes.
