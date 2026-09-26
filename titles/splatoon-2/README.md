# splatoon-2

The NEX game server for **Splatoon 2** on Nintendo Switch, built on [`servers/nex`](../../README.md) and
Pretendo's AGPL `nex-go` / `nex-protocols-go` / `nex-protocols-common-go`.

> Splatoon 2 is a **NEX** title — this repository. Splatoon 3 is **NPLN**, a different protocol
> in a different repository. Don't confuse the two.

**Status (2026-09-09): it builds, starts, serves `/healthz`, creates its schemas, registers
every protocol the title is known to use — including a real DataStore where earlier had a
stub — and its queries have been run against a real Postgres. Nothing has been run against a
console.** `NEX_VERSION` is the earlier server's runtime value rather than a capture, the public station
type for this Pia generation is unverified, and the account chain still waits on nx-baas proving
a real console's BAAS id token. Treat "implemented" as exactly that, not "works on hardware".

## What it does

- Matchmaking, matchmake-extension, match-making-ext, NAT traversal and utility from
  `nex-protocols-common-go`, over Postgres.
- **A real DataStore, where the earlier server had a complete stub.** Reference answered *every*
  `0x73` method with an empty success; here metadata, tags, ratings, persistence slots and
  search live in Postgres, object bodies in an S3 bucket when one is configured, and friends-only
  permissions read OpenPak's friend graph. This is the reason the title was worth porting — see
  [`docs/design.md`](docs/design.md).
- **Standard Rankings** over Postgres, in all five ranking modes, ranked by the database.
- **The Splatoon 2 methods above the standard protocols**, answered through nex-go's `Patches`
  hook so nothing double-responds: `Utility::UpdateCurrentUser` (the network-ready gate) and
  `AcquireTagId`; the splatfest Ranking queries (18/19/25); and the custom matchmake-extension
  method 60.
- **Login is method 2** (`LoginEx` / `ValidateAndRequestTicketWithCustomData`), not the method 6
  its later siblings use. `servers/nex` already wires both, so this needs no code here, but it is
  why `NEX_VERSION=4.0.0` is load-bearing.

Gameplay is peer to peer. Once two consoles hold each other's station URLs the server is out of
the match.

See [`docs/design.md`](docs/design.md) for what is deliberately not implemented, and
[`docs/provenance.md`](docs/provenance.md) for where every protocol fact came from.

## Run

```sh
cp example.env .env    # then edit it
go build ./cmd/splatoon-2
DATABASE_URL=… NEX_PORT_BASE=22320 NEX_ACCESS_KEY=4eb18d39 NX_INTERNAL_KEY=… ./splatoon-2
curl localhost:22322/healthz
```

Postgres is required: `nex-protocols-common-go` keeps matchmaking there, and this server keeps
DataStore objects and leaderboards there.

| Env | Meaning |
| --- | --- |
| `DATABASE_URL` | **required.** Postgres connection string. The server creates its own schemas at startup |
| `NEX_PORT_BASE` | **required.** `22320` — this title's block from [ports.md](https://github.com/openpak/docs/blob/main/ports.md). Authentication on `+0`, secure on `+1`, `/healthz` on `+2` |
| `NEX_ACCESS_KEY` | **required.** `4eb18d39`. Public, documented per title, and configuration rather than a literal in source |
| `NX_INTERNAL_KEY` | **required.** nx-baas's internal key; without it every login is refused |
| `NX_INTERNAL_URL` | nx-baas game API, default `http://127.0.0.1:20070` |
| `NEX_VERSION` | default `4.0.0` — the earlier server's runtime value, **not a capture**. Accepts `40000` too |
| `NEX_SOURCE_KEY_BYTES` | `32`, the Switch kerberos key size Reference uses for this title |
| `NEX_SECURE_HOST` | address the console is told to reach the secure endpoint on |
| `NEX_STATION_SCHEME` | `prudps` (the default, and what this title's station URL used) |
| `NEX_STREAM_ID`, `NEX_SECURE_STREAM_ID` | both `1` (the default). This title runs **two** listeners with the secure endpoint on its own port — do not set the secure stream to 2 |
| `NEX_TLS_CERT`, `NEX_TLS_KEY` | set both to terminate `wss` here, or neither to let Traefik terminate and forward plain `ws` |
| `NEX_SECURE_PID` | secure server principal ID, default `2` |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:22322` |
| `DATASTORE_S3_ENDPOINT` | optional object bucket for DataStore object bodies. Unset means metadata only, said once at startup |
| `DATASTORE_S3_ACCESS_KEY`, `DATASTORE_S3_SECRET_KEY`, `DATASTORE_S3_BUCKET` | credentials and bucket for the above |
| `DATASTORE_S3_INSECURE` | set to any value to talk to the bucket over plain HTTP |

`NEX_TITLE` is ignored: this binary names itself. `NEX_TRANSPORT` defaults to `websocket`,
which is what a Switch speaks — leave it alone.

Firewall: the listener is TCP behind Traefik, not UDP.

## Test

```sh
go build ./... && go vet ./... && go test ./...
```

The tests cover the parts that are not one-liners: the patch method/handler consistency, the
score update rule, the leaderboard and search query builders. The storage tests need a database
and skip without one:

```sh
podman run --rm -d -e POSTGRES_PASSWORD=check -p 23099:5432 postgres:16-alpine
S2_TEST_DATABASE_URL='postgres://postgres:check@127.0.0.1:23099/postgres?sslmode=disable' go test ./...
```

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs; every fact taken from the earlier server is a fact, recorded in
  [`docs/provenance.md`](docs/provenance.md), and none of its code is here.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Splatoon 2's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material, binaries,
certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
