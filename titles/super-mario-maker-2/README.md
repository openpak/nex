# super-mario-maker-2

NEX game server for **Super Mario Maker 2** on Nintendo Switch, built on Pretendo's AGPL
`nex-go` through [`servers/nex`](../../README.md).

SMM2's whole online mode is DataStore: Course World is course upload, download, search,
comments, bookmarks, world maps and per-maker profiles — all one protocol (`0x73`). This is
the most DataStore-heavy title in the fleet and the reference case for the shared object store.

## Status

- **Base DataStore object layer: implemented and tested.** Course metadata, tags, ratings and
  persistence slots in Postgres; course level BLOBs in presigned S3. `PreparePostObject` /
  `PrepareGetObject` / `CompletePostObject` / `SearchObject` / `RateObject` / `DeleteObject`
  all do what they say. Persistence-slot replacement (a maker's own-course slots, capped at 32)
  is real and covered by a test that runs against Postgres.
- **Ranking, matchmaking, NAT traversal, utility: wired** through `nex-protocols-common-go`,
  the same protocol set the console was observed to use.
- **Course World title-specific methods: specified, not yet implemented.** `GetUsers` (48),
  `RegisterUser` (47), `SyncUserProfile` (49), `CanPostCourse` (60), the browse/comment/
  world-map methods and the custom relation uploads (66, 132) are recorded in full in
  [`docs/provenance.md`](docs/provenance.md) and are the remit of the shared
  `github.com/openpak/nex/datastore` store. Until that store serves them, a real client boots
  but Course World does not populate. See [`docs/design.md`](docs/design.md).

## Configuration

Copy [`example.env`](example.env) to `.env`. Per-title values (with provenance in
[`docs/provenance.md`](docs/provenance.md)): access key `fdf6617f`, NEX version `4.0.0`
(unverified guess), station scheme `prudps`, two listeners on stream 1, 32-byte source key,
port block **22330** (auth 22330, secure 22331, `/healthz` 22332). SMM2 genuinely uploads
object bodies, so `DATASTORE_S3_*` is needed for course upload/download to work — not optional.

## Build and test

```sh
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test ./...   # DB-backed tests skip without SMM2_TEST_DATABASE_URL
```

`github.com/openpak/nex` is private; a local build needs a `~/.netrc` with a GitHub token and
`GOPRIVATE=github.com/openpak/*`.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed code
  are the only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Super Mario Maker 2's publisher, or
any original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
