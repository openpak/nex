# luigis-mansion-3

NEX game server for **Luigi's Mansion 3** on Nintendo Switch, built on Pretendo's AGPL `nex-go`
through [`platforms/wiiu-3ds/games/nex`](../../README.md).

**Status (2026-09-09): runs, never seen a console.** It starts, pings a real Postgres, serves
`/healthz` on 22192, listens in WebSocket mode on 22190 (auth) and 22191 (secure), and registers
Luigi's Mansion 3's matchmaking, NAT traversal and Utility protocols on the shared host plus an
acknowledge-only stub for the game's own `0x79` presence protocol. Whether the game reaches it
and likes what it gets back is unanswered: [`docs/design.md`](docs/design.md) lists the questions
a first live run would settle, and the code fails loudly rather than guessing.

## What it does

Registers the protocol set Luigi's Mansion 3 speaks and supplies the two things the common
library leaves to the operator:

- **Friend-only lobbies.** The matchmaking manager's friend lookup goes to
  [`nx-baas`](../../../../../switch/infra/nx-baas) through `platforms/wiiu-3ds/games/nex`, so this title keeps no friend graph of its own
  and a player is the same person here as in any other OpenPak title.
- **Utility settings.** The game reads integer settings the moment it connects; a reply it cannot
  parse drops the connection, so these are answered in the right shape (empty for now — no capture
  of this title's settings exists).

Luigi's Mansion 3 online is co-op matchmaking (ScreamPark and ScareScraper). It touches **no
DataStore**, so none is registered. It does run its own station/presence protocol on `0x79`
("who is here / where are my friends"); that is registered as a **stub** — it acknowledges the
methods the game calls so the connection stays alive, but the populated station/friend records
are deferred until OpenPak has its own capture and the shared host exposes station lookup. See
[`docs/design.md`](docs/design.md).

Login, kerberos, PRUDP, SecureConnection and account identity are all `platforms/wiiu-3ds/games/nex`'s. This
repository does not reimplement any of them.

## Run

```sh
cp example.env .env    # then edit
go build ./cmd/luigis-mansion-3
env $(grep -v '^#' .env | xargs) ./luigis-mansion-3
```

| Env | Meaning |
| --- | --- |
| `NEX_PORT_BASE` | `22190` — this title's block from [ports.md](https://github.com/openpak/docs/blob/main/ports.md). Two listeners: auth `22190`, secure `22191`. Defaulted by the server if unset |
| `NEX_ACCESS_KEY` | **required.** Luigi's Mansion 3's NEX access key (`aab95246`). A per-title constant from the game binary, not a secret, but configuration — never a literal in source. A wrong one fails *silently* |
| `NEX_VERSION` | NEX library the game links: `4.0.0`. Defaulted |
| `NEX_SECURE_HOST` | bare address the console is told to dial for the secure endpoint |
| `NX_INTERNAL_URL` | nx-baas game API, default `http://127.0.0.1:20070` |
| `NX_INTERNAL_KEY` | **required.** nx-baas's internal key. Without it every login is refused |
| `LM3_DATABASE_URL` | **required.** Postgres for matchmaking and NEX unique IDs. The libraries create their own schemas on first run |

Every other per-title fact — `prudps` station scheme, two-listener secure stream `1`, 32-byte
source key, secure PID `2`, websocket transport — is defaulted correctly for this title by the
server; see [`docs/provenance.md`](docs/provenance.md) for the source of each. Firewall: ports.md
rule 4 opens the 22xxx game-transport ports; on WebSocket transport these are TCP. The HTTP port
stays internal.

## Test

```sh
go build ./...
go vet ./...
go test ./...
```

`go test ./...` needs nothing running: it checks that the per-title facts reach a valid host
config (a wrong one is a silent login failure on hardware) and that the `0x79` stub acknowledges
exactly the methods the game calls, and only those.

## Documentation

- [`docs/design.md`](docs/design.md) — what the game needs, what is implemented, what is a stub,
  and the open questions a first live run answers.
- [`docs/provenance.md`](docs/provenance.md) — every non-obvious fact and where it came from.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before writing a
  line. Public protocol documentation, our own captures, and compatibly licensed code are the only
  inputs; `docs/provenance.md` records which was which.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Luigi's Mansion 3's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material, binaries,
certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
