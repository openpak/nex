# monster-hunter-generations-ultimate

The OpenPak NEX server for **Monster Hunter Generations Ultimate** (Nintendo Switch). It is
[`servers/nex`](../../README.md) plus this title's protocol registrations plus a real DataStore.

**Status (2026-09-09):** builds, starts, serves `/healthz`, listens on its NEX block, creates
its schema and round-trips DataStore against Postgres. **Never run against the game.** MHGU
is a 3DS-lineage title and the one place it is known to break — the client's `2306-0116`
during the secure handshake — is answered here with a smaller kerberos ticket that nobody has
yet watched a console accept. See [`docs/design.md`](docs/design.md).

## What it does

- Runs the shared NEX host with this title's access key, NEX 4.4.0, raw PRUDP over UDP, and
  the 3DS-era kerberos ticket (version 0) the title's Pia expects.
- Registers Pretendo's common Match Making, Matchmake Extension, Match Making Ext, NAT
  Traversal and Utility protocols against Postgres, with friend-only matchmaking answered
  from the player's real OpenPak friend graph.
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
| `NEX_PORT_BASE` | **required.** `22120` — this title's block from [ports.md](../../../../ports.md). Authentication 22120/udp, secure 22121/udp, `/healthz` 22122 |
| `NEX_ACCESS_KEY` | **required.** `4152f312`, the title's NEX access key ([docs/provenance.md](docs/provenance.md)) |
| `NEX_VERSION` | `40400` — MHGU links NEX 4.4.0 |
| `NEX_TRANSPORT` | `udp`. MHGU speaks raw PRUDP, not the WebSocket transport native Switch titles use |
| `NEX_SECURE_PID` | `2`, the 3DS/Wii U "Quazal Rendez-Vous" account |
| `NEX_SECURE_HOST` | address the console is told to dial for the secure server |
| `NX_INTERNAL_URL`, `NX_INTERNAL_KEY` | **required.** nx-baas's internal game API, which turns a console's id token into an OpenPak account |
| `DATABASE_URL` | **required.** Postgres for matchmaking, unique IDs and DataStore metadata |
| `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_USE_TLS` | object storage for DataStore bodies. Leave `S3_ENDPOINT` empty and the metadata half still works; object transfer answers `NotImplemented` |

Every other `NEX_*` variable is `servers/nex`'s and documented there. Startup fails loudly on
a missing access key, internal key, port block or database.

Firewall: 22120/udp and 22121/udp need explicit `ufw allow` rules (ports.md rule 4). The
health port stays internal.

## Test

```sh
go build ./... && go vet ./... && go test ./...
```

The tests cover the search-parameter translation and the expiry rule without a database. Set
`DATABASE_URL` and one more test runs: a real object through create, upload completion,
metadata, persistence slot, search and rating — the part where the SQL is either right or a
runtime error.

## Ground rules

- Clean room. [`docs/clean-room-policy.md`](../../../../docs/clean-room-policy.md); every protocol
  fact cites its source in [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../../../ports.md).

This project is not affiliated with or endorsed by Nintendo, Monster Hunter Generations
Ultimate's publisher, or any original service provider. It contains no proprietary game code,
assets, SDK material, binaries, certificates, keys, firmware, extracted files, credentials,
or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
