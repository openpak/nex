# minecraft

NEX game server for **Minecraft** on Nintendo Switch, built on Pretendo's AGPL `nex-go`
through the shared host in [`platforms/wiiu-3ds/games/nex`](../../README.md).

Minecraft's Switch online is session join: a console logs in, then hosts a world or joins
one — including a friend's. This binary registers the protocol set Minecraft speaks on the
secure endpoint and wires the friend lookup; the shared host owns everything up to and
including login.

## Status

Starts against a real Postgres, serves `/healthz`, listens on its port block, and registers
Minecraft's protocols. Verified locally (`go build/vet/test` green; run against Postgres 16,
`/healthz` → `ok minecraft websocket auth=22310 secure=22311`). **Not yet run against a real
console** — the station scheme and access key are the two things a hardware run confirms. See
[`docs/design.md`](docs/design.md) for the open questions and [`docs/provenance.md`](docs/provenance.md)
for where each fact came from.

## Protocols

| Protocol | ID | Where |
| --- | --- | --- |
| TicketGranting | `0x0A` | `platforms/wiiu-3ds/games/nex` |
| SecureConnection | `0x0B` | `platforms/wiiu-3ds/games/nex` |
| NATTraversal | `0x03` | here |
| MatchMaking | `0x15` | here |
| MatchMakingExt | `0x32` | here |
| MatchmakeExtension | `0x6D` | here |
| Utility | `0x6E` | here |

DataStore (`0x73`) and Ranking (`0x70`) are **not** registered — no DataStore client is linked, and the only known Ranking handler is measured against Mario Kart 8 Deluxe with
no Minecraft-specific fact behind it. See `docs/provenance.md`.

## Configuration

Copy [`example.env`](example.env) to `.env` and fill it in. The per-title values, with
provenance in `docs/provenance.md`:

- `NEX_ACCESS_KEY=f4983d87` — cryptographically verified against a measured CONNECT signature.
- `NEX_VERSION=40000` (4.0.0) — the value earlier research ran; a note says 4.3.1
  (recorded contradiction, behaviourally equal for us).
- `NEX_STATION_SCHEME=prudp` — Minecraft's observed scheme, unusual for the fleet; the top
  thing to verify on hardware.
- Stream ID and transport are `platforms/wiiu-3ds/games/nex` defaults (two-listener, websocket) and are right
  for Minecraft.

`MINECRAFT_DATABASE_URL` (Postgres) is required; matchmaking and NEX unique IDs live there.

## Building

Rides the private module `github.com/openpak/nex`:

```
printf 'machine github.com login tobagin password %s\n' "$(gh auth token)" > ~/.netrc && chmod 600 ~/.netrc
export GOPRIVATE='github.com/openpak/*'
GOWORK=off go build ./...
rm ~/.netrc
```

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md): block `22310`.

This project is not affiliated with or endorsed by Nintendo, Minecraft's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
