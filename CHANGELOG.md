# Changelog — nex

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

Note: the early history below is the upstream project (Pretendo nex-go and title servers);
OpenPak work starts at the port/fork commit.

## Unreleased

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

