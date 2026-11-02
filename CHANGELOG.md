# Changelog: ps5-friends

## Unreleased

- 2026-11-02: first cut. Friends list, friend requests (send/accept/cancel), blocks and basic presence under
  `/api/userProfile/v1/internal/users/{id}` with `oauth.Require` from ps5-common; in-memory store behind the
  `social.Store` interface; presence inferred from recent token use or set by `PUT /basicPresences`. Tests with
  httptest. Not deployed, no tag, not tried against a console.
