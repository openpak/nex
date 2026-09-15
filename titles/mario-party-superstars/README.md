# mario-party-superstars

NEX game server for **Mario Party Superstars** on Nintendo Switch (title `01006FE013472000`,
NEX 4.6.5, Pia 5.33.0), built on [`servers/nex`](../../README.md) and Pretendo's AGPL `nex-go`.

**Status (2026-09-09): it builds, starts, serves `/healthz`, listens on both WebSocket
listeners of its NEX block and creates its schemas.** Nothing has been run against a console
or the game. The identity and wire-shape values it
uses — access key, NEX version, station scheme, listener shape — are confirmed measurements carried across from the
earlier network server for this title; everything about how the *game* behaves against
`nex-go` is unverified. [`docs/design.md`](docs/design.md) says what is implemented and what
is not; [`docs/provenance.md`](docs/provenance.md) says where every fact came from and which
assumptions are still guesses.

## What it does

Registers this title's protocols on the shared NEX host and adds the two things neither the
host nor `nex-protocols-common-go` provides for it:

- **Friend join.** `FindMatchmakeSessionByParticipant` (`0x6D`.`0x33`) and
  `FindMatchmakeSessionByGatheringIDDetail` (`0x6D`.`0x29`), the two methods the game's Join
  Room screen uses to follow a friend into a lobby. The common library implements neither,
  and without them the game silently starts its own session instead of joining.
- **Utility settings** in the shape the game expects, loadable from the environment because
  nobody has captured this title's real values yet.

DataStore is deliberately registered by nothing: no capture has ever shown this title making
a DataStore call, and the object store is a shared component of `servers/nex` rather than a
per-title one. See [`docs/design.md`](docs/design.md).

Everything else — PRUDP, kerberos, Switch login, OpenPak accounts, match making, NAT
traversal, NEX unique IDs, cleaning up a lobby after a dropped connection — is a call into
`servers/nex` or `nex-protocols-common-go`.

## Run

```sh
export GOPRIVATE='github.com/openpak/*'   # github.com/openpak/nex is a private module
go build ./cmd/mario-party-superstars
NEX_ACCESS_KEY=… NX_INTERNAL_KEY=… MPS_DATABASE_URL=postgres://… ./mario-party-superstars
```

It listens on **two WebSocket listeners** — authentication on 22110, secure on 22111 — which
is this title's measured shape, not a default. The startup line says which transport is
running.

| Env | Required | Meaning |
| --- | --- | --- |
| `NEX_ACCESS_KEY` | **yes** | The title's NEX access key. See [`example.env`](example.env) — it is configuration, not a secret |
| `NX_INTERNAL_KEY` | **yes** | nx-baas's `NX_INTERNAL_KEY`, sent as `X-Internal-Key`. Without it every login is refused |
| `MPS_DATABASE_URL` | **yes** | Postgres for matchmaking and NEX unique IDs. Startup fails without it: a NEX title with no lobby storage cannot do anything |
| `NEX_PORT_BASE` | no | Defaults to **22110**, this title's block from [`ports.md`](../../../../ports.md). Authentication `+0`, secure `+1`, `/healthz` `+2` |
| `NEX_VERSION` | no | Defaults to **4.6.5**, the version the game was built against. Changing it changes the login method and the wire encoding — do not, without a capture that says otherwise |
| `NEX_SECURE_HOST` | no | Address the console is told to reach the secure server on. Default `127.0.0.1`; set it to the host's real address for anything but a loopback test |
| `NEX_TITLE`, `NEX_HTTP_ADDR`, `NX_INTERNAL_URL` | no | As in [`servers/nex`](../../README.md) |
| `MPS_INTEGER_SETTINGS` | no | `Utility::GetIntegerSettings` values as `index:key=value` entries, e.g. `0:0=60, 0:1=30`. Empty by default because this title's real values have never been captured |
| `NEX_STATION_SCHEME` | no | Defaults to **`prudps`**, measured for this title. Under `prudp` the handshake completes but CONNECT carries an empty payload, and the game fails later, when it first needs the session |
| `NEX_SECURE_STREAM_ID` | no | Defaults to **1**: this is a two-listener title, secure on its own port. Do not set it to 2 |
| `NEX_TRANSPORT` | no | `websocket` is the host's default and correct for this title |
| `NEX_SOURCE_KEY_BYTES` | no | Defaults to **32**, measured for this title. Per title, and the wrong value fails *silently* — the login just does not work, with nothing logged either side |

Firewall: 22110 and 22111 carry game traffic and need explicit `ufw allow` rules
([`ports.md`](../../../../ports.md) rule 4). They are **TCP**, not UDP — this title's transport is
PRUDP over WebSocket, so the rule that block wants is `ufw allow <port>/tcp`. 22112 stays
internal.

## Test

```sh
go build ./...
go vet ./...
go test ./...
```

For local work across both repositories, use a `go.work` at `~/REPOS/Openpak/servers` rather
than a `replace` directive — see [`servers/nex`](../../README.md)'s README.

## Deploy

Tag `v1.2.3` and the release workflow publishes
`ghcr.io/openpak/mario-party-superstars:1.2.3` and `:latest`; the deploy host runs
`podman-auto-update` against `:latest`.

**One prerequisite is not yet met:** `github.com/openpak/nex` is a private repository, so
the build needs credentials the public proxy cannot supply. CI wants an org secret
`OPENPAK_MODULE_TOKEN` with read access to `openpak/nex` — `GITHUB_TOKEN` covers only this
repository. Until that secret exists the workflow fails at `go mod download`, loudly. Every
NEX title server needs the same secret.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../../../docs/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs; every protocol fact cites its source in
  [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../../../ports.md). This title holds 22110–22119.

This project is not affiliated with or endorsed by Nintendo, Mario Party Superstars's
publisher, or any original service provider. It contains no proprietary game code, assets,
SDK material, binaries, certificates, keys, firmware, extracted files, credentials, or raw
packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
