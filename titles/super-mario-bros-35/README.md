# super-mario-bros-35

NEX game server for **Super Mario Bros. 35** on Nintendo Switch, built on
[`servers/nex`](../../README.md) — OpenPak's shared NEX host, which wraps Pretendo's AGPL `nex-go`.

Nintendo delisted this game on 30 March 2021 and switched its service off the next day. It
was a time-limited release, and it has no live service to compete with: **nobody can play it
online today except against a server like this one.** That makes it a pure preservation
target, and it is the reason there is no identity wall here — there is no vendor left to
validate a token.

**Status:** builds, starts, serves `/healthz`, and registers every NEX protocol the game
calls, with the wire types and values taken from a reference server that is publicly
documented to have worked against a retail client. **Nothing has been run against the game.**
The one thing missing before a match can actually be played is the Eagle relay, which is not
in this repository and does not exist in OpenPak yet — see [`docs/design.md`](docs/design.md).
Every remaining assumption is listed there as an open question, and every protocol fact cites
its source in [`docs/provenance.md`](docs/provenance.md).

## What it does

- **Matchmaking.** `AutoMatchmakeWithParamPostpone` and friends, through Pretendo's common
  protocols. The game proposes a session for 35 players; the server finds one matching the
  search criteria or creates it, joins the caller, and notifies the rest. Nothing on the
  server counts to 35 or decides a lobby is ready — no NEX call starts a match.
- **The Eagle handoff.** Super Mario Bros. 35 does not play peer to peer. When a player joins
  a lobby, the server pushes one notification carrying the relay's address and a per-player
  signed token. Without it the lobby fills and the game waits forever, showing no error.
- **Ranking2 and Matchmake Referee**, the two protocols the game calls that
  `nex-protocols-common-go` has no implementation of: the name-and-Mii exchange every lobby
  does, and round bookkeeping that refuses a round naming players who are not in it.
- **Lobby messaging** and the game's `GetIntegerSettings` tuning table.

DataStore is deliberately absent: this game never calls it.

## Build

`github.com/openpak/nex` is a private module, so the toolchain must be told not to check it
against the public sum database — otherwise the build fails on a 404, locally and in CI
alike, even when the module is already present:

```sh
export GOPRIVATE='github.com/openpak/*'
```

## Run

```sh
go build ./cmd/smb35d
NEX_PORT_BASE=22160 NEX_ACCESS_KEY=… SMB35_DATABASE_URL=… ./smb35d
```

`/healthz` answers on `NEX_PORT_BASE+2`.

### Environment

The shared host reads the `NEX_*` and `NX_*` half; see [`servers/nex`](../../README.md) for the full
list. These are the values that are **specific to this title** and wrong by default:

| Env | Value here | Why |
| --- | --- | --- |
| `NEX_PORT_BASE` | `22160` | this title's block from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md). Both endpoints share the listener on `+0`, `+1` is reserved and unused, `/healthz` is on `+2` |
| `NEX_ACCESS_KEY` | **required** | the title's NEX access key: public, per title, and configuration rather than a literal in source |
| `NEX_VERSION` | `40600` | NEX 4.6.0 |
| `NEX_SECURE_STREAM_ID` | `2` | the secure endpoint is on virtual port 2, which puts both endpoints on one listener. **Specific to this title** — most Switch titles in the fleet use two listeners |
| `NEX_SECURE_PID` | `257049437023956657` | the retail secure-server principal ID, not the conventional 2 |
| `NEX_SOURCE_KEY_BYTES` | `16` | the host defaults to 32; this title has same-title retail evidence for 16, and a wrong value is a login that fails with nothing logged on either side |
| `NEX_STATION_SCHEME` | `prudps` | per title; the wrong value fails late, when a session is actually needed |
| `NX_INTERNAL_KEY` | **required** | nx-baas's internal key; without it every login is refused |

And this title's own:

| Env | Meaning |
| --- | --- |
| `SMB35_DATABASE_URL` | **required.** Postgres. Matchmaking, messaging and the NEX unique-id table live here; the common protocols create their own schemas on first start |
| `SMB35_EAGLE_URL` | base URL of the Eagle relay. A lobby's session is `<url>/<gathering id>`. Empty means no handoff is sent, and the server says so at startup |
| `SMB35_EAGLE_KEY` | HMAC-SHA256 key the relay verifies handoff tokens with, hex, 16 bytes or more. Shared with the relay and nothing else. Setting a URL without a key is refused at startup |

See [`example.env`](example.env).

Firewall: the PRUDP listener needs an explicit `ufw allow 22160/tcp` rule (the transport is
WebSocket, not UDP, on every Switch NEX title). The HTTP port stays internal.

## Test

```sh
export GOPRIVATE='github.com/openpak/*'
go build ./...
go vet ./...
go test ./...
```

The tests cover the parts that are not one-liners: the Eagle handoff token, field by field,
against the format a relay will parse — including that two players in one lobby get different
tokens; the round store's lifecycle and its pruning of rounds abandoned without an `EndRound`;
the integer-settings table and its refusal of an unknown index; and the configuration checks
that fail closed.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs. Every protocol fact in
  [`docs/provenance.md`](docs/provenance.md) cites its source.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

This project is not affiliated with or endorsed by Nintendo, Super Mario Bros. 35's
publisher, or any original service provider. It contains no proprietary game code, assets,
SDK material, binaries, certificates, keys, firmware, extracted files, credentials, or raw
packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
