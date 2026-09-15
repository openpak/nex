# mario-strikers

NEX game server for **Mario Strikers: Battle League** on Nintendo Switch, built on Pretendo's
AGPL `nex-go` through [`servers/nex`](../../README.md).

**Status:** the confirmed protocol substrate is registered and the binary builds, vets and tests
green; full club/online play is capture-driven next-round work. `servers/nex` owns login; this
binary registers what Strikers speaks on the secure endpoint.

## What runs

`cmd/strikersd` imports the shared NEX host and registers Strikers' confirmed protocol set:

| Protocol | ID | Backing |
| --- | --- | --- |
| MatchMaking / MatchMakingExt / MatchmakeExtension | `0x15` / `0x32` / `0x6D` | `nex-protocols-common-go`, Postgres |
| NATTraversal | `0x03` | `nex-protocols-common-go` |
| Utility | `0x6E` | `nex-protocols-common-go`, Postgres |
| DataStore | `0x73` | `nex-protocols-common-go`, Postgres + optional S3 |

TicketGranting (`0x0A`) and SecureConnection (`0x0B`) come from [`servers/nex`](../../README.md).

Ranking (`0x70`), MatchmakeReferee (`0x78`), Subscriber (`0x79`), Ranking2 (`0x7A`) and the
Strikers-specific MatchmakeExtension **club** methods (`0x6D` method 54+) are **deferred** —
they have no common implementation and/or are unconfirmed reverse-engineering in the Reference
source, so they wait on OpenPak captures. See [`docs/provenance.md`](docs/provenance.md) and
[`docs/design.md`](docs/design.md).

## Configure

Copy [`example.env`](example.env) to `.env`. The whole title-specific configuration is:

| Var | Value | Why |
| --- | --- | --- |
| `NEX_PORT_BASE` | `22300` | this title's block (auth 22300, secure 22301, healthz 22302) |
| `NEX_ACCESS_KEY` | `MAGNI5jG` | Strikers' access key — an 8-char ASCII string, not hex, and not the `0x26CFAF00` game-server ID |
| `NEX_VERSION` | `4.0.0` | the NEX library Strikers links |
| `STRIKERS_DATABASE_URL` | Postgres URL | matchmaking, unique IDs, DataStore metadata |

Everything else is a `servers/nex` default that is already correct for Strikers (two-listener
`prudps`, secure PID 2, 32-byte source key, websocket transport). `STRIKERS_S3_*` is optional and
only needed if a DataStore object body ever has to move.

## Build and test

```sh
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test ./...                                  # search tests run; DataStore SQL skips
STRIKERS_TEST_DATABASE_URL=postgres://... go test ./...   # DataStore SQL against a real Postgres
```

`github.com/openpak/nex` is a private module: set `GOPRIVATE=github.com/openpak/*` and a
`~/.netrc` with a GitHub token before building.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../../../docs/clean-room-policy.md) before writing a
  line. The earlier server is read for facts — ports, keys, method IDs, wire shapes, flows —
  never copied. Facts and their sources are recorded in [`docs/provenance.md`](docs/provenance.md).
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../../../ports.md).

This project is not affiliated with or endorsed by Nintendo, Mario Strikers: Battle League's
publisher, or any original service provider. It contains no proprietary game code, assets, SDK
material, binaries, certificates, keys, firmware, extracted files, credentials, or raw packet
captures.
