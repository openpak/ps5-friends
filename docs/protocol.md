# Protocol: ps5-friends

The friends part of the NP web API 2 `userProfile` service, `https://m.np.playstation.com/api/...`, as the
PlayStation app and psn-api/PSNAWP use it. Every request carries `Authorization: Bearer <token>`; a missing or
invalid token is `401` with a PSN-shaped error body (`{"error":{"referenceId","code","message"}}`).
`{id}` is `me` (the token's account) or a decimal account id. Source of the shapes: `research/ps5/psn-endpoints.md`
section 1.3 (public client code, not captured from a console).

Prefix: `/api/userProfile/v1/internal/users/{id}`. Disjoint from ps5-profile, which owns `.../profiles`.

| Request | Answer |
| --- | --- |
| `GET /friends?limit&offset` | `200 {"friends":["<accountId>",...],"totalItemCount":N,"nextOffset":N?,"previousOffset":N?}`. Own list, or a friend's; otherwise `403 Not permitted by access control` |
| `GET /friends/receivedRequests` (alias `/friends/requests`) | same shape, incoming requests; own only |
| `GET /friends/sentRequests` | same shape, outgoing requests; OpenPak extension |
| `PUT /friends/{accountId}` | `204`. Sends a request; if the other already sent one, accepts it. `400` for yourself, `403` if either side blocked |
| `DELETE /friends/{accountId}` | `204`; removes a friendship, cancels or declines a request; `404` if there was none |
| `GET /friends/{accountId}/summary` | `200 {"friendRelation":"friend\|requested\|requesting\|no","accountId"}` |
| `GET /blocks?limit&offset` | `200 {"blockList":[...],"totalItemCount":N,"nextOffset","previousOffset"}` |
| `PUT /blocks/{accountId}` | `204`; also severs friendship and requests |
| `DELETE /blocks/{accountId}` | `204`, `404` if not blocked |
| `GET /basicPresences?type=primary` | `200 {"basicPresence":{"availability":"unavailable\|availableToPlay","lastAvailableDate","primaryPlatformInfo":{"onlineStatus","platform","lastOnlineDate"},"gameTitleInfoList":[{"npTitleId","titleName","format","launchPlatform","npTitleIconUrl","conceptIconUrl"}]}}`. Own or a friend's; otherwise `403` |
| `PUT /basicPresences` | `204`; body `{"availability","platform","gameTitleInfoList"}`. OpenPak extension (no public equivalent known) |

Mutations apply to the caller's own graph only (`{id}` must be `me` or the caller); anything else is `403`.

## Presence

Without a `PUT`, an account is `availableToPlay`/`online` while its token was used within 5 minutes (every
authenticated request counts), `unavailable`/`offline` after. A `PUT` sets availability, platform and the title
list, honoured under the same 5 minute window.

## Unverified

- Which of these paths the PS5 system software itself calls (the shapes are the phone-app surface).
- HTTP methods and bodies of the friend manage endpoints (PSNAWP names the paths only), the `summary` body, the
  `blocks` response key (`blockList`), the error codes (placeholders), and the access-control rules for
  other accounts' lists.
- `lastAvailableDate` and `lastOnlineDate` formats (ISO 8601 UTC with milliseconds assumed).
