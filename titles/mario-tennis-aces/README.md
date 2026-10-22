# mario-tennis-aces

The NEX game server for **Mario Tennis Aces** on Nintendo Switch, built on
[`platforms/wiiu-3ds/games/nex`](../../README.md) and Pretendo's AGPL `nex-go` / `nex-protocols-go` /
`nex-protocols-common-go`.

**Status (2026-09-09): it builds, starts, serves `/healthz`, creates its schemas and answers
every protocol the title is known to use. Nothing has been run against a console.** The NEX
version is an inherited guess and the account chain still waits on nx-baas proving a real
console's BAAS id token. Treat "implemented" as exactly that, not as "works on hardware".

## What it does

- Matchmaking, matchmake-extension, match-making-ext, NAT traversal and utility from
  `nex-protocols-common-go`, unmodified, over Postgres.
- **Two behaviours this title needs that the common library does not cover:** a created
  session is marked open immediately, closing the "double host" deadlock two searching
  players otherwise fall into; and `FindMatchmakeSessionByParticipant` is answered, which is
  how playing with a friend finds their room.
- **A real DataStore**, not a stub: metadata, tags, ratings,
  persistence slots and search live in Postgres, and object bodies in an S3 bucket when one
  is configured.
- **Rankings**: leaderboards in all five ranking modes,
  ranked by the database, with the friends modes reading OpenPak's friend graph.

Gameplay is peer to peer. Once two consoles hold each other's station URLs the server is
out of the match.

See [`docs/design.md`](docs/design.md) for what is deliberately not implemented, and
[`docs/provenance.md`](docs/provenance.md) for where every protocol fact came from.

## Run

```sh
cp example.env .env    # then edit it
go build ./cmd/mario-tennis-aces
DATABASE_URL=… NEX_PORT_BASE=22170 NEX_ACCESS_KEY=… NX_INTERNAL_KEY=… ./mario-tennis-aces
curl localhost:22172/healthz
```

Postgres is required: `nex-protocols-common-go` keeps matchmaking there, and this server
keeps DataStore objects and leaderboards there.

| Env | Meaning |
| --- | --- |
| `DATABASE_URL` | **required.** Postgres connection string. The server creates its own schemas at startup |
| `NEX_PORT_BASE` | **required.** `22170` — this title's block from [ports.md](https://github.com/openpak/docs/blob/main/ports.md). Authentication on `+0`, secure on `+1`, `/healthz` on `+2` |
| `NEX_ACCESS_KEY` | **required.** `29f267b9`. Public, documented per title, and configuration rather than a literal in source |
| `NX_INTERNAL_KEY` | **required.** nx-baas's internal key; without it every login is refused |
| `NX_INTERNAL_URL` | nx-baas game API, default `http://127.0.0.1:20070` |
| `NEX_VERSION` | default `4.3.5` — **a guess**, see the design notes. Accepts `40305` too |
| `NEX_SECURE_HOST` | address the console is told to reach the secure endpoint on |
| `NEX_STATION_SCHEME` | `prudps` (the default, and what this title's station URL used) |
| `NEX_STREAM_ID`, `NEX_SECURE_STREAM_ID` | both `1` (the default). This title runs **two** listeners with the secure endpoint on its own port — do not set the secure stream to 2 |
| `NEX_TLS_CERT`, `NEX_TLS_KEY` | set both to terminate `wss` here, or neither to let Traefik terminate and forward plain `ws` |
| `NEX_SECURE_PID` | secure server principal ID, default `2` |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:22172` |
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

The tests cover the parts that are not one-liners: the session-opening fix, the score
update rule, the leaderboard and search query builders. The storage tests need a database
and skip without one:

```sh
podman run --rm -d -e POSTGRES_PASSWORD=check -p 23099:5432 postgres:16-alpine
MTA_TEST_DATABASE_URL='postgres://postgres:check@127.0.0.1:23099/postgres?sslmode=disable' go test ./...
```

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs; every fact taken from earlier research notes is a fact, recorded in
  [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Mario Tennis Aces's publisher, or
any original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
