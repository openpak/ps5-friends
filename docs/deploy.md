# Deploy: ps5-friends

## Run

```sh
cp example.env .env              # set PS5_TOKEN_KEY, same value as ps5-auth
set -a; source .env; set +a
GOWORK=off go run ./cmd/ps5-friends
```

```sh
GOWORK=off go vet ./... && GOWORK=off go test ./...
docker build -t ps5-friends .
```

## Configuration

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `PS5_ADDR` | no | `:8080` | HTTP listen address |
| `PS5_TOKEN_KEY` | yes | | HMAC key for bearer tokens (>= 32 bytes), shared with ps5-auth |
| `PS5_LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |
| `PS5_DATABASE_URL` | yes | | Postgres URL (3-connection pool); the service exits without it unless `PS5_DEV_MEMORY=1`. Tables are created at start |
| `PS5_DEV_MEMORY` | no | | `1`: keep state in memory, lost on restart (local development only) |

The other `PS5_*` variables of ps5-common are read but unused (no database, no account core yet).

## Shared host: m.np.playstation.com

`m.np.playstation.com` is one DNS name for ps5-profile, ps5-friends, ps5-trophy and ps5-messaging, so Traefik
routes by path prefix. This service takes (priority above ps5-profile's catch-alls is not needed because the
prefixes are disjoint):

```yaml
http:
  routers:
    ps5-friends:
      rule: >-
        Host(`m.np.playstation.com`) && (
        PathRegexp(`^/api/userProfile/v1/internal/users/[^/]+/(friends|blocks|basicPresences)(/.*)?$`))
      service: ps5-friends
      tls: {}
  services:
    ps5-friends:
      loadBalancer:
        servers: [{url: "http://openpak-ps5-friends:8080"}]
```

ps5-profile owns `/api/userProfile/v1/internal/users/me/profiles` and `/users/{id}/profiles` (and v2); keep its
rule to `.../profiles` so nothing overlaps. The regex above must not be widened to the whole `userProfile` tree.

## Storage

Friends, pending requests and blocks live in Postgres (`PS5_DATABASE_URL`; tables `ps5_friends`, `ps5_friend_requests`,
`ps5_blocks`, created at start), so one database backup covers them. Presence is transient and stays in memory (the
console re-announces it). The service exits without a database unless `PS5_DEV_MEMORY=1`.

Why not the core Social service: it keys friendships by core account id and has no decline/cancel of a pending
request and no block-severs-friendship, which the PSN userProfile API relies on. Moving the graph onto it is a
later step (`prds/universal-social-prd.md`); the `social.Store` interface is the seam.

## Release

Tag `v*.*.*` publishes `ghcr.io/openpak/ps5-friends`; deployed hosts auto-update from it.

## Account erasure

`DELETE /internal/accounts/{account_id}/data` with `Authorization: Bearer <ACCOUNT_ERASE_KEY>` erases everything this service
holds for a core account (UUID). It listens on `ERASE_ADDR` (default `:9099`, container network only, never published) and is
only served when `ACCOUNT_ERASE_KEY` is set. The website finaliser calls `http://openpak-ps5-friends:9099`; the reply is
`{"service":"ps5-friends","rows":N}`. The ids searched are the core account id and, when the core (`PS5_ACCOUNT_CORE_URL`) has an
active `ps5` link for it, the link subject. If the core is configured but unreachable the call fails (500) rather than
skipping data.

Erased: friend rows (both directions), requests sent and received, blocks made and blocks against the account, and in-memory presence.
Kept: nothing per account. Other people lose the friendship/block that referenced the erased account; their own other rows are untouched.
