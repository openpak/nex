# super-mario-odyssey

OpenPak NEX game server for **Super Mario Odyssey** on Nintendo Switch, built on Pretendo's
AGPL `nex-go` through the shared host [`servers/nex`](../../README.md) (`github.com/openpak/nex`).

Odyssey is the odd one out of the NEX fleet: **no matchmaking, no lobby, no peer-to-peer play
at all.** Its entire online feature is **Balloon World** — hide a balloon (a capture-pose
screenshot plus costume snapshot at an in-game spot), and other players search for and pop it,
with "clap" ratings. That is DataStore (`0x73`) end to end. So this binary registers a small
protocol set, and the one protocol the whole title lives on is a real implementation, not a
stub.

## What it registers

| Protocol | ID | Role |
| --- | --- | --- |
| TicketGranting / SecureConnection | `0x0A` / `0x0B` | login — owned by `servers/nex` |
| Utility | `0x6E` | NEX unique IDs (name a balloon before posting it) |
| DataStore | `0x73` | Balloon World: place, find, rate, delete — **real**, over Postgres + presigned S3 |

Not registered: MatchMaking/MatchmakeExtension/NATTraversal (Odyssey has no lobby) and Ranking
(`0x70`, speculative — see [`docs/design.md`](docs/design.md)).

## DataStore: real base, one open gap

The **base** DataStore protocol is real — `GetMeta`/`GetMetas`, `PreparePostObject`/
`CompletePostObject`/`PrepareGetObject`, `DeleteObject`, `SearchObject` (a real query, not the
`dataType`-only one the earlier server had), `RateObject`/`RateObjects`, `PostMetaBinary`,
`ChangeMeta` — backed by durable Postgres metadata, presigned S3 for object bodies, and the
real OpenPak friend graph for friends-only visibility.

Odyssey also speaks a **title-specific Balloon extension on `0x73` (methods 47-53:
`SearchBalloon`, `FetchMyInfos`, the pose-buffer queues)** that **no common library
implements** and this server does **not** yet serve. That is the capture-gated gap: `FetchMyInfos`
is the first DataStore call Odyssey makes, so Balloon World needs the extension to fully work
on hardware. [`docs/provenance.md`](docs/provenance.md) and [`docs/design.md`](docs/design.md)
have the full story and what building it properly requires.

## Configuration

Everything is environment-driven; see [`example.env`](example.env). Port block **22340** from
[`ports.md`](https://github.com/openpak/docs/blob/main/ports.md): auth `22340`, secure `22341`, `/healthz` `22342`. Every
`servers/nex` transport default is correct for Odyssey; the one unconfirmed value is
`NEX_VERSION` (a `4.0.0` guess). No secrets live in the repository.

## Status

Builds, vets and tests green. Run against a throwaway Postgres it starts in the two-listener
shape, answers `/healthz`, and creates its schema — but no console has touched it. The NEX
version, the access key (reference-verified, not binary-verified), and the whole Balloon
extension are open questions a live capture must answer. Treat multiplayer claims as a plan
until then.

## Ground rules

Clean room — see [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md). Public protocol
documentation, our own captures, and compatibly licensed code are the only inputs; the Reference
Odyssey server was read for facts, never copied. AGPL-3.0-only, English only, Go 1.25.

This project is not affiliated with or endorsed by Nintendo, Super Mario Odyssey's publisher,
or any original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
