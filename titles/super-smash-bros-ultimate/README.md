# super-smash-bros-ultimate

NEX game server for **Super Smash Bros. Ultimate** on Nintendo Switch, built on Pretendo's AGPL
`nex-go` through [`platforms/wiiu-3ds/games/nex`](../../README.md).

**Status (2026-09-09): runs, never seen a console.** Verified locally: it builds, vets, passes
its tests, and its DataStore queries run against a real Postgres. It starts, creates its schema,
serves `/healthz` on 22352, listens in WebSocket mode on 22350 and 22351, and registers SSBU's
matchmaking, NAT traversal, Utility and DataStore protocols on the shared host. Whether **SSBU**
reaches it and likes what it gets back is unanswered — [`docs/design.md`](docs/design.md) names
the two things a first live run would settle, the Pia type-8 keepalive foremost.

## What it does

Registers the protocol set SSBU speaks and makes real the two protocols a menu-only stub
would leave empty:

- **A real DataStore.** A stub answers every `0x73` call with an empty list and `GetMeta` with
  `NotFound`. Here `GetMeta`,
  `GetMetas`, `SearchObject`, `PreparePostObject`, `PrepareGetObject`, `CompletePostObject`,
  `PostMetaBinary`, `ChangeMeta`, `RateObject` and `DeleteObject` are implemented against
  Postgres, with object bodies presigned straight to S3 so none passes through this process.
- **A real Utility.** SSBU calls the NEX-unique-ID getters during online bring-up; a stub
  answers them with an empty list. Here they return real, persistent
  IDs from `nex-protocols-common-go`'s Utility manager.
- **Friend-list lobbies and friends-only DataStore searches.** The friend lookup goes to
  [`nx-baas`](../../../../../switch/infra/nx-baas) through `platforms/wiiu-3ds/games/nex`, so SSBU keeps no friend graph of its own and
  a player is the same person here as in any other OpenPak title.

Login, kerberos, PRUDP, SecureConnection and account identity are all `platforms/wiiu-3ds/games/nex`'s. This
repository does not reimplement any of them. Ranking (`0x70`) and the Pia type-8 keepalive are
deliberately not built — see [`docs/design.md`](docs/design.md).

## Run

```sh
cp example.env .env    # then edit
go build ./cmd/super-smash-bros-ultimate
env $(grep -v '^#' .env | xargs) ./super-smash-bros-ultimate
```

| Env | Meaning |
| --- | --- |
| `NEX_PORT_BASE` | **required.** `22350` — this title's block from [ports.md](https://github.com/openpak/docs/blob/main/ports.md). Two listeners: authentication `22350`, secure `22351` |
| `NEX_ACCESS_KEY` | **required.** SSBU's NEX access key, `9587602b`. A per-title constant out of the game binary, not a secret, but configuration — never a literal in source. A wrong one fails *silently*: the console retries forever and nothing is logged |
| `NEX_VERSION` | **unverified.** `4.0.0` inherited from earlier research notes where it was a copied MK8 template value; a capture reporting `apiVersionGeneral` settles it |
| `NEX_SECURE_HOST` | bare address the console is told to dial for the secure endpoint |
| `NEX_TRANSPORT` | leave unset. SSBU is `websocket`, the `platforms/wiiu-3ds/games/nex` default |
| `NEX_SECURE_STREAM_ID` | leave unset. SSBU uses the two-listener shape, stream `1`, the default. Setting `2` is the single-listener shape and would break it |
| `NEX_SOURCE_KEY_BYTES` | leave unset. The key MUST be 32 bytes or the console rejects the ticket; 32 is the default |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:22352` |
| `NX_INTERNAL_URL` | nx-baas game API, default `http://127.0.0.1:20070` |
| `NX_INTERNAL_KEY` | **required.** nx-baas's internal key. Without it every login is refused |
| `SSBU_DATABASE_URL` | **required.** Postgres for matchmaking, NEX unique IDs and DataStore metadata. The libraries create their own schemas on first run |
| `SSBU_S3_ENDPOINT` | object store for DataStore bodies. **Optional** — unset, every metadata call still works and only the three that move a body answer `Core::NotImplemented` |
| `SSBU_S3_BUCKET`, `SSBU_S3_ACCESS_KEY`, `SSBU_S3_SECRET_KEY` | required *if* `SSBU_S3_ENDPOINT` is set; a half-configured store fails at startup |
| `SSBU_S3_INSECURE` | `1` for a plain-HTTP store, e.g. a local MinIO |

See [`example.env`](example.env). Firewall: ports.md rule 4 opens the 22xxx game-transport
ports explicitly; on WebSocket transport these are TCP, not UDP. The HTTP port stays internal.

## Test

```sh
go build ./...
go vet ./...
go test ./...
```

`go test ./...` alone covers the DataStore search-query builder without needing anything
running: that no value a console sends reaches the SQL as text, that an unknown sort column
falls back instead of being interpolated, that the page size is capped, and that only searches
which need a friend graph pay for the lookup.

Point it at a database and it also exercises the SQL for real — post, complete, read, search,
page, rate, delete, and the persistence-slot rule:

```sh
SSBU_TEST_DATABASE_URL=postgres://openpak:openpak@127.0.0.1:23000/openpak?sslmode=disable go test ./...
```

Those tests skip when the variable is unset, so the default run needs no database.

## Documentation

- [`docs/design.md`](docs/design.md) — what SSBU needs, what is implemented, what is not, and
  the open questions a first live run answers.
- [`docs/provenance.md`](docs/provenance.md) — every non-obvious fact and where it came from.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs; `docs/provenance.md` records which was which.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Super Smash Bros. Ultimate's
publisher, or any original service provider. It contains no proprietary game code, assets, SDK
material, binaries, certificates, keys, firmware, extracted files, credentials, or raw packet
captures.

## License

AGPL-3.0-only — see `LICENSE`.
