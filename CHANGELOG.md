# Changelog — nex

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

Note: the early history below is the upstream project (Pretendo nex-go and title servers);
OpenPak work starts at the port/fork commit.

## v1.1.0 — 2026-09-16

- host: `NEX_SECURE_TRANSPORT` splits the secure listener's transport from the auth
  listener's. Monster Hunter Generations Ultimate is the caller: its auth stage is wss
  behind Traefik's SNI passthrough, its secure stage raw PRUDP over UDP. Unset, the secure
  listener takes `NEX_TRANSPORT` as before.
- `titles/monster-hunter-generations-ultimate`: the auth listener is wss, not UDP, and the
  protocol set now matches `titles/monster-hunter-xx` — this title is that game's Switch
  port and runs its NEX code, down to the same access key. Message Delivery `0x1B`, Ranking
  `0x70` registered without an implementation, `EnableInsecureRegister`, `SendReport`
  accepting and discarding, `mmlegacy`, and the station URL type back to Public.
- `mmlegacy`: the five matchmaking methods `nex-protocols-common-go` v2.6.1 does not
  implement and the 3DS-lineage titles call — `FindByOwner` (0x15.0x16),
  `UpdateMatchmakeSessionAttribute` (0x6D.0x0C), `FindMatchmakeSessionBySingleGatheringID`
  (0x6D.0x31), `BrowseMatchmakeSessionNoHolderNoResultRange` (0x6D.0x34) and
  `FindMatchmakeSessionByParticipant` (0x6D.0x33). Lifted from `titles/monster-hunter-xx`
  with the matchmaking manager passed in instead of read from a package global, so the
  Switch port of that game can register the same set.
- fix: the release workflow parses again. `${{ runner.temp }}` in job-level `env` is not a
  valid context there, and an unparseable workflow has no triggers at all — the `v1.0.1`
  and first `v1.1.0` tags produced no run whatsoever, so no image had been published since
  `v0.8.0` while every host auto-updates against `:latest`.
- fix: the image builds again. `go mod download` cannot resolve the vendored nex-go
  `replace` target unless that tree is copied in first.

## v1.0.1 — 2026-09-15 (tagged, never built)

- nex-go: write raw PRUDPLite responses to bare-TCP Switch clients [a4e46e3]
- release: job-scoped registry auth file [7ba65b8]
- release: build on the self-hosted runner with podman; private-repo minutes are metered [7b4b63b]


## v0.7.0 — 2026-09-11



## v0.6.0 — 2026-09-11



## v0.5.1 — 2026-09-11



## v0.5.0 — 2026-09-11



## v0.4.0 — 2026-09-11



## v0.3.0 — 2026-09-10



## v0.2.1 — 2026-09-09



## v0.2.0 — 2026-09-09



## v0.1.0 — 2026-09-09



## v0.1.0 — 2026-09-09

- change: real module path github.com/openpak/nex [7c2ef78]
- fix: apply the Switch byte-stream profile in the host [0a48923]
- feat: expose the SecureConnection common protocol on Host [0c13f33]
- fix: Switch source key is 32 bytes, not 16, or the console rejects the ticket [d8d9eb2]
- fix: FriendPIDs must not truncate a 64-bit PID into another account [3c52fb4]
- change: default NEX_TRANSPORT to websocket [d9d3f57]
- feat: TLS on the websocket listener; nex-protocols-go 2.3.7 [f17788c]
- feat: websocket transport, one listener with two streams, secure PID [7479047]
- fix: correct the Switch login against the facts; expose DataStore [87b4cda]
- feat: the shared NEX host on Pretendo's nex-go [7f334db]
- seed: empty nex, AGPL-3.0-only, clean-room gate [9160fb5]

