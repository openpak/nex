# mario-party-superstars

NEX game server for **Mario Party Superstars** on Nintendo Switch (title `01006FE013472000`,
NEX 4.6.5, Pia 5.33.0), built on [`servers/nex`](../nex) and Pretendo's AGPL `nex-go`.

**Status (2026-09-09): it builds, starts, serves `/healthz`, binds its NEX block and creates
its schemas.** Nothing has been run against a console or the game. The identity values it
uses — access key, NEX version — are confirmed measurements carried across from the
earlier network server for this title; everything about how the *game* behaves against
`nex-go` is unverified. [`docs/design.md`](docs/design.md) says what is implemented and what
is not; [`docs/provenance.md`](docs/provenance.md) says where every fact came from and which
assumptions are still guesses.

## What it does

Registers this title's protocols on the shared NEX host and adds the three things neither
the host nor `nex-protocols-common-go` provides for it:

- **Friend join.** `FindMatchmakeSessionByParticipant` (`0x6D`.`0x33`) and
  `FindMatchmakeSessionByGatheringIDDetail` (`0x6D`.`0x29`), the two methods the game's Join
  Room screen uses to follow a friend into a lobby. The common library implements neither,
  and without them the game silently starts its own session instead of joining.
- **Utility settings** in the shape the game expects, loadable from the environment because
  nobody has captured this title's real values yet.
- **A real DataStore.** Postgres object metadata plus presigned S3 uploads and downloads, so
  `GetMeta`, `SearchObject`, `RateObject`, `ChangeMeta`, `DeleteObject` and the whole
  prepare/complete upload handshake answer with real state. the earlier network server stubs
  all of these, because the NEX core it runs on has no DataStore at all.

Everything else — PRUDP, kerberos, Switch login, OpenPak accounts, match making, NAT
traversal, NEX unique IDs, cleaning up a lobby after a dropped connection — is a call into
`servers/nex` or `nex-protocols-common-go`.

## Run

```sh
go build ./cmd/mario-party-superstars
NEX_ACCESS_KEY=… NX_INTERNAL_KEY=… MPS_DATABASE_URL=postgres://… ./mario-party-superstars
```

| Env | Required | Meaning |
| --- | --- | --- |
| `NEX_ACCESS_KEY` | **yes** | The title's NEX access key. See [`example.env`](example.env) — it is configuration, not a secret |
| `NX_INTERNAL_KEY` | **yes** | nx-baas's `NX_INTERNAL_KEY`, sent as `X-Internal-Key`. Without it every login is refused |
| `MPS_DATABASE_URL` | **yes** | Postgres for matchmaking, NEX unique IDs and DataStore metadata. Startup fails without it: a NEX title with no lobby storage cannot do anything |
| `NEX_PORT_BASE` | no | Defaults to **22110**, this title's block from [`ports.md`](../../ports.md). Authentication `+0`, secure `+1`, `/healthz` `+2` |
| `NEX_VERSION` | no | Defaults to **4.6.5**, the version the game was built against. Changing it changes the login method and the wire encoding — do not, without a capture that says otherwise |
| `NEX_SECURE_HOST` | no | Address the console is told to reach the secure server on. Default `127.0.0.1`; set it to the host's real address for anything but a loopback test |
| `NEX_TITLE`, `NEX_HTTP_ADDR`, `NX_INTERNAL_URL` | no | As in [`servers/nex`](../nex) |
| `MPS_INTEGER_SETTINGS` | no | `Utility::GetIntegerSettings` values as `index:key=value` entries, e.g. `0:0=60, 0:1=30`. Empty by default because this title's real values have never been captured |
| `MPS_S3_ENDPOINT` | no | S3-compatible object store for DataStore object bytes. Without it the metadata methods still work and the upload/download methods answer `Core::NotImplemented` |
| `MPS_S3_BUCKET` | no | Default `mario-party-superstars` |
| `MPS_S3_ACCESS_KEY`, `MPS_S3_SECRET_KEY` | with S3 | Object store credentials |
| `MPS_S3_USE_SSL` | no | `1` to reach the object store over HTTPS |

Firewall: 22110 and 22111 are UDP and need explicit `ufw allow <port>/udp` rules
([`ports.md`](../../ports.md) rule 4). 22112 stays internal.

## Test

```sh
go build ./...
go vet ./...
go test ./...
```

The DataStore checks want a real database, because a store is not worth testing against a
mock of itself. They skip unless one is offered:

```sh
MPS_TEST_DATABASE_URL=postgres://…/throwaway go test ./...
```

They truncate `datastore.objects`, so point them at a database you do not mind losing.

## Deploy

Not yet — see the deploy section of [`docs/design.md`](docs/design.md). `servers/nex` is
imported through a local `replace` directive, which no container build or CI checkout can
resolve. That needs one fleet-wide answer, not a per-title workaround.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs; every protocol fact cites its source in
  [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../ports.md). This title holds 22110–22119.

This project is not affiliated with or endorsed by Nintendo, Mario Party Superstars's
publisher, or any original service provider. It contains no proprietary game code, assets,
SDK material, binaries, certificates, keys, firmware, extracted files, credentials, or raw
packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
