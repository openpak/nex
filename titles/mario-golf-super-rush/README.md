# mario-golf-super-rush

NEX game server for **Mario Golf: Super Rush** on Nintendo Switch. It runs on
[`servers/nex`](../nex), OpenPak's shared NEX host, which wraps Pretendo's AGPL-3.0
`nex-go`, `nex-protocols-go` and `nex-protocols-common-go`.

Golf is a peer-to-peer title: the server logs a console in, keeps the lobby list and
introduces four consoles to each other, and the round itself is Pia traffic between the
players. So this repository is one file of wiring — `cmd/golf/main.go` — over libraries that
already implement the protocols.

**Status (2026-09-09): builds, starts, serves `/healthz`, and registers Golf's matchmaking
stack.** Nothing has been run against a console, an emulator or the retail title. The NEX
version is an era guess and the station scheme has never been measured for this title;
[`docs/design.md`](docs/design.md) lists what the next round needs, and
[`docs/provenance.md`](docs/provenance.md) records where every fact came from.

## What it serves

| Protocol | State |
| --- | --- |
| Ticket Granting `0x0A`, Secure Connection `0x0B` | from `servers/nex` |
| Match Making `0x15`, Matchmake Extension `0x6D`, Match Making Ext `0x32` | Postgres |
| NAT Traversal `0x03` | stateless |
| Utility `0x6E` | Postgres |
| Ranking `0x70`, DataStore `0x73` | not registered — see [`docs/design.md`](docs/design.md) |

Friend-only lobbies are answered from [`nx-baas`](../../nx-baas), so there is no friend
table here: a player's friends are the same people to every OpenPak service. Lobbies live in
Postgres and survive a restart.

## Run

```sh
go build ./cmd/golf
cp example.env .env   # then edit it
env $(grep -v '^#' .env | xargs) ./golf
```

| Env | Meaning |
| --- | --- |
| `NEX_TITLE` | name in logs, default `mario-golf-super-rush` |
| `NEX_PORT_BASE` | **required.** `22130` — Golf's block from [`ports.md`](../../ports.md). One listener carries both endpoints on `+0`; `+1` stays reserved and unbound |
| `NEX_ACCESS_KEY` | **required.** `1cb8027c` — Golf's NEX access key: eight hex digits, confirmed, and not the game server ID in the DNS name. Configuration, never a literal in source |
| `NEX_VERSION` | NEX library version the title was built against. `4.6.5` here, an era guess. The integer form `40605` is accepted too |
| `NEX_SECURE_HOST` | address the console is told to reach the secure server on. A bare host or IP; the port comes from the block |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:<NEX_PORT_BASE+2>` |
| `NEX_TRANSPORT` | `websocket`: a Switch NEX title speaks PRUDPLite over a WebSocket, never PRUDP over UDP |
| `NEX_SECURE_STREAM_ID` | `2`, not the shared host's Wii U-era `1`. It gives the secure endpoint its own stream, which is also what puts both endpoints on one listener |
| `NEX_TLS_CERT`, `NEX_TLS_KEY` | Golf is reached by Traefik SNI passthrough, so this server terminates TLS: set both. Both empty means the front proxy terminates and forwards plain `ws`; exactly one is refused at startup |
| `NEX_STATION_SCHEME` | `prudps`, whether the console treats the secure server as a Pia station. Never measured for this title |
| `GOLF_DATABASE_URL` | **required.** Postgres for matchmaking and NEX unique IDs. The tables are created on startup |
| `NX_INTERNAL_URL` | nx-baas game/internal API, default `http://127.0.0.1:20070` |
| `NX_INTERNAL_KEY` | **required.** nx-baas's `NX_INTERNAL_KEY`, sent as `X-Internal-Key` |

Startup fails loudly on a bad port block, a missing access key, a missing internal key or an
unreachable database, rather than serving something that cannot work.

Firewall: one game port, TCP not UDP — `ufw allow 22130/tcp`. Traefik passes
`g211a3f00-lp1.s.n.srv.nintendo.net` through by SNI to 22130, where this server terminates
TLS itself. The `/healthz` port stays internal.

## Test

```sh
go build ./... && go vet ./... && go test ./...
```

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../docs/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs. Every fact this server rests on cites its source in
  [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../ports.md). This repository holds block 22130.

This project is not affiliated with or endorsed by Nintendo, Mario Golf: Super Rush's
publisher, or any original service provider. It contains no proprietary game code, assets,
SDK material, binaries, certificates, keys, firmware, extracted files, credentials, or raw
packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
