# arms

NEX game server for **ARMS** on Nintendo Switch, built on Pretendo's AGPL `nex-go` through
[`servers/nex`](../../README.md).

**Status (2026-09-09): runs, never seen a console.** Verified locally against a real Postgres:
it starts, creates its schemas, serves `/healthz` on 22142, listens in WebSocket mode on
22140 and 22141, and registers ARMS's matchmaking, NAT traversal, Utility and DataStore protocols
on the shared host. The DataStore round trip — post, complete, read, search, rate, delete — is
covered by tests that run against a real database. Whether **ARMS** reaches it and likes what
it gets back is unanswered: [`docs/design.md`](docs/design.md) lists the five questions a first
live run would settle, and the code fails loudly rather than guessing at any of them.

## What it does

Registers the protocol set ARMS speaks and supplies the two things
`nex-protocols-common-go` leaves to the operator:

- **Friend-list lobbies.** The matchmaking manager's friend lookup goes to
  [`nx-baas`](../../../../nx-baas) through `servers/nex`, so ARMS keeps no friend graph of its own
  and a player is the same person here as in an NPLN title.
- **A real DataStore.** ARMS's update links a DataStore client (the base game does not).
  `earlier-nex` has none, so the earlier network ARMS server answers
  every `0x73` call with an empty list. Here `GetMeta`, `GetMetas`, `SearchObject`,
  `PreparePostObject`, `PrepareGetObject`, `CompletePostObject`, `PostMetaBinary`,
  `ChangeMeta`, `RateObject` and `DeleteObject` are implemented against Postgres, with object
  bodies presigned straight to S3 so none of them passes through this process.

Login, kerberos, PRUDP, SecureConnection and account identity are all `servers/nex`'s. This
repository does not reimplement any of them.

## Run

```sh
cp example.env .env    # then edit
go build ./cmd/armsd
env $(grep -v '^#' .env | xargs) ./armsd
```

| Env | Meaning |
| --- | --- |
| `NEX_PORT_BASE` | **required.** `22140` — this title's block from [ports.md](https://github.com/openpak/docs/blob/main/ports.md). Two listeners: authentication `22140`, secure `22141` |
| `NEX_ACCESS_KEY` | **required.** ARMS's NEX access key. A per-title constant out of the game binary, not a secret, but configuration — never a literal in source. A wrong one fails *silently*: the console retries forever and nothing is logged |
| `NEX_VERSION` | NEX library ARMS links: `4.3.5`, what the update links. The base game links `4.0.3` |
| `NEX_SECURE_HOST` | bare address the console is told to dial for the secure endpoint |
| `NEX_TRANSPORT` | leave unset. ARMS is `websocket`, which is now the `servers/nex` default |
| `NEX_SECURE_STREAM_ID` | leave unset. ARMS uses the two-listener shape, stream `1`, which is the default. Setting `2` is the single-listener shape and would break it |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:22142` |
| `NX_INTERNAL_URL` | nx-baas game API, default `http://127.0.0.1:20070` |
| `NX_INTERNAL_KEY` | **required.** nx-baas's internal key. Without it every login is refused |
| `ARMS_DATABASE_URL` | **required.** Postgres for matchmaking, NEX unique IDs and DataStore metadata. The libraries create their own schemas on first run |
| `ARMS_S3_ENDPOINT` | object store for DataStore bodies. **Optional** — unset, every metadata call still works and only the three that move a body answer `Core::NotImplemented` |
| `ARMS_S3_BUCKET`, `ARMS_S3_ACCESS_KEY`, `ARMS_S3_SECRET_KEY` | required *if* `ARMS_S3_ENDPOINT` is set; a half-configured store fails at startup |
| `ARMS_S3_INSECURE` | `1` for a plain-HTTP store, e.g. a local MinIO |

See [`example.env`](example.env). Firewall: ports.md rule 4 opens the 22xxx game-transport
ports explicitly; on WebSocket transport these are TCP, not UDP. The HTTP port stays internal.

## Test

```sh
go build ./...
go vet ./...
go test ./...
```

`go test ./...` alone covers the DataStore search query builder without needing anything
running: that no value a console sends reaches the SQL as text, that an unknown sort column
falls back instead of being interpolated, that the page size is capped, and that only the
searches which need a friend graph pay for the lookup.

Point it at a database and it also exercises the SQL for real — post, complete, read, search,
page, rate, delete, and the persistence-slot rule:

```sh
ARMS_TEST_DATABASE_URL=postgres://openpak:openpak@127.0.0.1:23000/openpak?sslmode=disable go test ./...
```

Those tests skip when the variable is unset, so the default run needs no database.

## Documentation

- [`docs/design.md`](docs/design.md) — what ARMS needs, what is implemented, what is not, and
  the open questions a first live run answers.
- [`docs/provenance.md`](docs/provenance.md) — every non-obvious fact and where it came from.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs; `docs/provenance.md` records which was which.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, ARMS's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
