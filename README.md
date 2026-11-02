# PS5 Friends Server

OpenPak's PS5 Friends Server.

| | |
| --- | --- |
| Network | PlayStation 5 |
| Role | Friends, friend requests, blocks and basic presence of the PSN userProfile API on `m.np.playstation.com` |
| Used by | PlayStation 5 (jailbroken, pointed at OpenPak), PlayStation app clients |
| Image | `ghcr.io/openpak/ps5-friends` |
| Licence | AGPL-3.0-only |

## What it serves

| Endpoint (prefix `/api/userProfile/v1/internal/users/{id}`, `{id}` is `me` or an account id) | What |
| --- | --- |
| `GET /friends` | Friend account ids, paged |
| `GET /friends/receivedRequests`, `GET /friends/requests` | Incoming friend requests |
| `GET /friends/sentRequests` | Outgoing friend requests (OpenPak extension) |
| `PUT /friends/{accountId}` | Send a friend request, or accept an incoming one |
| `DELETE /friends/{accountId}` | Unfriend, cancel or decline |
| `GET /friends/{accountId}/summary` | Relation to that account |
| `GET /blocks`, `PUT`/`DELETE /blocks/{accountId}` | Block list |
| `GET /basicPresences`, `PUT /basicPresences` | Basic presence (the PUT is an OpenPak extension) |

## Run

```sh
export PS5_TOKEN_KEY=$(head -c 32 /dev/urandom | base64)
GOWORK=off go run ./cmd/ps5-friends          # listens on :8080
```

## Docs

- [Protocol](docs/protocol.md)
- [Deploy](docs/deploy.md)

## Licence

AGPL-3.0-only. See [LICENSE](LICENSE).
