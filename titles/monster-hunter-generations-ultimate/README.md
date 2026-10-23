# monster-hunter-generations-ultimate

The OpenPak NEX server for **Monster Hunter Generations Ultimate** (Nintendo Switch). It is
[`platforms/wiiu-3ds/games/nex`](../../README.md) plus this title's protocol registrations plus a real DataStore.

**Status (2026-09-16):** builds, starts, serves `/healthz`, listens on its NEX block, creates
its schema and round-trips DataStore against Postgres. **Never run against the game.** Two
things changed today. The auth listener is TCP, not UDP: the console reaches that stage over
TLS by SNI, so a UDP-only server could never have heard from one. And the protocol set now
matches Pretendo's Monster Hunter XX server, this title's 3DS original, including the five
matchmaking methods `nex-protocols-common-go` leaves unimplemented. The known wall — the
client's `2306-0116` — is answered here with a smaller kerberos ticket **and** the fuller
method 6 response shape, neither yet watched by a console.
See [`docs/design.md`](docs/design.md).

## What it does

- Runs the shared NEX host with this title's access key, NEX 4.4.0, the 3DS-era kerberos
  ticket (version 0) the title's Pia expects, and **two transports**: wss for the
  authentication stage the console reaches by SNI, raw PRUDP over UDP for the secure stage.
- Registers Pretendo's common Match Making, Matchmake Extension, Match Making Ext, NAT
  Traversal, Utility and Message Delivery protocols against Postgres, with friend-only
  matchmaking answered from the player's real OpenPak friend graph, plus the five
  matchmaking methods the common library has no file for ([`mmlegacy`](../../mmlegacy)),
  taken from Pretendo's [Monster Hunter XX](../monster-hunter-xx) server — MHGU is that
  game's Switch port and runs its NEX code, down to the same access key.
- Implements **DataStore** (`0x73`) properly — metadata in Postgres, object bodies presigned
  into an S3-compatible bucket — where the previous stack answered every call with an empty
  list. `GetMeta`, `GetMetas`, `SearchObject`, `PreparePostObject`, `CompletePostObject`,
  `PrepareGetObject`, `RateObject`, `ChangeMeta` and `DeleteObject` all answer for real.

## Run

```sh
go build ./cmd/mhgud
cp example.env .env   # then edit
./mhgud
```

| Env | Meaning |
| --- | --- |
| `NEX_PORT_BASE` | **required.** `22120` — this title's block from [ports.md](https://docs.openpak.org/contributors/ports/). Authentication 22120/**tcp** (Traefik SNI passthrough), secure 22121/**udp**, `/healthz` 22122 |
| `NEX_ACCESS_KEY` | **required.** `4152f312`, the title's NEX access key ([docs/provenance.md](docs/provenance.md)) |
| `NEX_VERSION` | `40400` — MHGU links NEX 4.4.0 |
| `NEX_TRANSPORT` | `websocket`. The auth stage is the Switch one: the console opens TLS on 443 to `g2896bd04-lp1.s.n.srv.nintendo.net` and Traefik passes the SNI through |
| `NEX_SECURE_TRANSPORT` | `udp`. The secure stage is the 3DS one its NEX code came from: raw PRUDP over UDP. This variable exists in `platforms/wiiu-3ds/games/nex` for this title and no other |
| `NEX_TLS_CERT`, `NEX_TLS_KEY` | terminate TLS on the auth listener, because Traefik passes the SNI through rather than terminating. Both or neither |
| `NEX_SECURE_PID` | `2`, the 3DS/Wii U "Quazal Rendez-Vous" account |
| `NEX_SECURE_HOST` | address the console is told to dial for the secure server |
| `NX_INTERNAL_URL`, `NX_INTERNAL_KEY` | **required.** nx-baas's internal game API, which turns a console's id token into an OpenPak account |
| `DATABASE_URL` | **required.** Postgres for matchmaking, unique IDs and DataStore metadata |
| `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_USE_TLS` | object storage for DataStore bodies. Leave `S3_ENDPOINT` empty and the metadata half still works; object transfer answers `NotImplemented` |

Every other `NEX_*` variable is `platforms/wiiu-3ds/games/nex`'s and documented there. Startup fails loudly on
a missing access key, internal key, port block or database.

Firewall: 22121/**udp** needs an explicit `ufw allow` rule (ports.md rule 4). 22120 is now
**tcp** behind Traefik's SNI passthrough and needs no public rule of its own. The
health port stays internal.

## Deploy — the 2026-09-16 cutover

The auth listener moved from UDP to TCP, so this is not a restart, it is a change of socket.
Three things have to line up or the title stays as silent as it has been:

1. **TLS on the auth listener.** Traefik's route for
   `g2896bd04-lp1.s.n.srv.nintendo.net` is `tls.passthrough: true`, so it hands this server
   the raw TLS stream and this server terminates it. Set `NEX_TLS_CERT` and `NEX_TLS_KEY`
   to the same certificate the other Switch-era titles present. Leave them empty and the
   startup line says `ws (TLS at the front proxy)` — that line is the tell, and behind a
   passthrough route it means every console handshake fails with nothing else logged.
2. **Firewall.** 22121/udp needs an `ufw allow`; the console dials it straight from the
   login station URL. 22120 is reached only through Traefik and needs no public rule.
3. **Traefik needs no change.** Its backend for this title has always been
   `openpak-monster-hunter-generations-ultimate:22120` over TCP. Until today nothing was
   listening there.

The first thing to check afterwards is the auth log. Before this change a console could not
reach it at all, so any RMC line on the auth endpoint is new information, and an empty log
is now a finding rather than the expected state.

## Test

```sh
go build ./... && go vet ./... && go test ./...
```

The tests cover the search-parameter translation and the expiry rule without a database. Set
`DATABASE_URL` and one more test runs: a real object through create, upload completion,
metadata, persistence slot, search and rating — the part where the SQL is either right or a
runtime error.

## Ground rules

- Clean room. [clean-room policy](https://docs.openpak.org/contributors/clean-room/); every protocol
  fact cites its source in [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://docs.openpak.org/contributors/ports/).

This project is not affiliated with or endorsed by Nintendo, Monster Hunter Generations
Ultimate's publisher, or any original service provider. It contains no proprietary game code,
assets, SDK material, binaries, certificates, keys, firmware, extracted files, credentials,
or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
