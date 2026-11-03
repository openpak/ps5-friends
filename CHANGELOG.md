# Changelog: ps5-friends

## v0.1.0 (2026-11-03)

- 2026-11-03: friends, requests and blocks are durable in Postgres (`PS5_DATABASE_URL`; `social.Postgres`, tables `ps5_friends`,
  `ps5_friend_requests`, `ps5_blocks`); presence stays in memory. The service exits without a database unless
  `PS5_DEV_MEMORY=1`. Storage failures answer 500 instead of 404. Restart test against a real Postgres. Not deployed.

- 2026-11-02: first cut. Friends list, friend requests (send/accept/cancel), blocks and basic presence under
  `/api/userProfile/v1/internal/users/{id}` with `oauth.Require` from ps5-common; in-memory store behind the
  `social.Store` interface; presence inferred from recent token use or set by `PUT /basicPresences`. Tests with
  httptest. Not deployed, no tag, not tried against a console.
