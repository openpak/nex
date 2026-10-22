# animal-crossing-new-horizons

NEX game server for **Animal Crossing: New Horizons** on Nintendo Switch, built on Pretendo's
AGPL `nex-go` through [`platforms/wiiu-3ds/games/nex`](../../README.md).

**Status (2026-09-09): runs, never seen a console.** Verified locally against a real Postgres:
it starts, creates its schemas (`acnh`, plus the libraries' `matchmaking`/`tracking`/`utility`),
serves `/healthz` on 22182, listens in WebSocket mode two-listener on 22180 (auth) and 22181
(secure), and registers ACNH's matchmaking, NAT traversal, Utility and DataStore protocols on
the shared host. The DataStore round trip — post, complete, read, search, rate, delete — is
covered by tests that run against a real database. Whether **ACNH** reaches it and likes what it
gets back is unanswered, and several ACNH-specific features are not covered by the common
libraries and are documented as gaps: see [`docs/design.md`](docs/design.md).

## What it does

Registers the protocol set ACNH speaks and supplies the two things `nex-protocols-common-go`
leaves to the operator:

- **The "visit a friend" friend lookup.** The matchmaking manager's friend lookup goes to
  [`nx-baas`](../../../../../switch/infra/nx-baas) through `platforms/wiiu-3ds/games/nex`, so ACNH keeps no friend graph of its own and
  a player is the same person here as in any other OpenPak title.
- **A real DataStore.** ACNH's DataStore usage was never mapped from captures, so a stub that logs every call and
  answers empty-success would be the alternative. Here `GetMeta`, `GetMetas`,
  `SearchObject`, `PreparePostObject`, `PrepareGetObject`, `CompletePostObject`, `PostMetaBinary`,
  `ChangeMeta`, `RateObject` and `DeleteObject` are implemented against Postgres, with object
  bodies presigned straight to S3 so none of them passes through this process.

Login, kerberos, PRUDP, SecureConnection and account identity are all `platforms/wiiu-3ds/games/nex`'s. This
repository does not reimplement any of them.

### Not yet covered (see [`docs/design.md`](docs/design.md) → Gaps)

The Pretendo common libraries do not implement four ACNH-specific things; they answer
`Core::NotImplemented` rather than a wrong guess, and each needs a live capture before it can be
built: `FindMatchmakeSessionByParticipant` (`0x6D.0x33`, friend island visits),
`UpdateMatchmakeSession` (`0x6D.0x2C`, writing a Dodo Code post-create), Utility `0x6E.9`/`0x6E.10`
and Ranking `0x70.25`, and the Pia-5.19 public-station shape (`type=0x0B`), which is a change
`platforms/wiiu-3ds/games/nex` must make. Standard session create/join/browse-by-code and Utility 1–8 do work.

## Run

```sh
cp example.env .env    # then edit
go build ./cmd/acnhd
env $(grep -v '^#' .env | xargs) ./acnhd
```

| Env | Meaning |
| --- | --- |
| `NEX_PORT_BASE` | **required.** `22180` — this title's block from [ports.md](https://github.com/openpak/docs/blob/main/ports.md). Two listeners: authentication `22180`, secure `22181` |
| `NEX_ACCESS_KEY` | **required.** `v43a10em` — ACNH's NEX access key, an 8-character **string, not hex**. A per-title constant out of the game binary, not a secret, but configuration. A wrong one fails *silently*: the console retries forever and nothing is logged. Do not confuse it with the Game Server ID `2EE2E300` |
| `NEX_VERSION` | `40000` — the NEX library ACNH links (4.0.0) |
| `NEX_SECURE_HOST` | bare address the console is told to dial for the secure endpoint |
| `NEX_STATION_SCHEME` | leave unset. ACNH is `prudps`, the default |
| `NEX_TRANSPORT` | leave unset. ACNH is `websocket`, the default |
| `NEX_SECURE_STREAM_ID` | leave unset. ACNH uses the two-listener shape, stream `1`, the default. Setting `2` would break it |
| `NEX_SOURCE_KEY_BYTES` | leave unset. ACNH uses `32`, the default |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:22182` |
| `NX_INTERNAL_URL` | nx-baas game API, default `http://127.0.0.1:20070` |
| `NX_INTERNAL_KEY` | **required.** nx-baas's internal key. Without it every login is refused |
| `ACNH_DATABASE_URL` | **required.** Postgres for matchmaking, NEX unique IDs and DataStore metadata. The libraries create their own schemas on first run |
| `ACNH_S3_ENDPOINT` | object store for DataStore bodies. **Optional** — unset, every metadata call still works and only the three that move a body answer `Core::NotImplemented` |
| `ACNH_S3_BUCKET`, `ACNH_S3_ACCESS_KEY`, `ACNH_S3_SECRET_KEY` | required *if* `ACNH_S3_ENDPOINT` is set; a half-configured store fails at startup |
| `ACNH_S3_INSECURE` | `1` for a plain-HTTP store, e.g. a local MinIO |

See [`example.env`](example.env). Firewall: ports.md rule 4 opens the 22xxx game-transport
ports; on WebSocket transport these are TCP, not UDP. The HTTP port stays internal.

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
ACNH_TEST_DATABASE_URL=postgres://openpak:openpak@127.0.0.1:23000/openpak?sslmode=disable go test ./...
```

Those tests skip when the variable is unset, so the default run needs no database.

## Documentation

- [`docs/design.md`](docs/design.md) — what ACNH needs, what is implemented, the gaps, and the
  open questions a first live run answers.
- [`docs/provenance.md`](docs/provenance.md) — every non-obvious fact and where it came from.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs; `docs/provenance.md` records which was which. The DataStore backend is adapted
  from the OpenPak sibling `servers/arms`, our own AGPL code.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Animal Crossing's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material, binaries,
certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
