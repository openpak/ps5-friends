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

## Storage and the universal social service

The store is in memory (`internal/social.Memory`), lost on restart, behind the `social.Store` interface. To make
the graph real, add a `Store` that maps onto the OpenPak universal social service (`prds/universal-social-prd.md`):
the `account` core is the only friend graph (`friend_requested`, `friend_accepted`, `friend_removed` events),
`platforms/core/infra/chat` carries notifications, and presence is a core fact with the adapter shaping the
payload. PS5 account ids would resolve through the `ps5` namespace of `ps5-common/accountcore`. Not built.

## Release

Tag `v*.*.*` publishes `ghcr.io/openpak/ps5-friends`; deployed hosts auto-update from it.
