# mario-kart-8-deluxe

NEX game server for **Mario Kart 8 Deluxe** on Nintendo Switch, built on the shared OpenPak
NEX host in [`platforms/wiiu-3ds/games/nex`](../../README.md) and Pretendo's AGPL-3.0
[`nex-protocols-common-go`](https://github.com/PretendoNetwork/nex-protocols-common-go).

**Status (2026-09-09): builds, starts, listens, never tested against a console.** The server
brings up its two PRUDP-over-WebSocket listeners and an HTTP `/healthz`, creates its Postgres
schemas, and answers the protocols MK8D's online mode uses. Nothing here has seen a real
Switch or an emulator. [`docs/design.md`](docs/design.md) lists what is missing and what
evidence would settle it; [`docs/provenance.md`](docs/provenance.md) says where every fact
came from.

One blocking issue is outside this repository: `platforms/wiiu-3ds/games/nex` mints a 16-byte kerberos source
key, and the Switch key size is 32. Until that changes, expect the console to reject the
ticket. See [`docs/design.md`](docs/design.md#blocking-issue-in-serversnex).

## What it does

- **Login** — through `platforms/wiiu-3ds/games/nex`: ticket-granting method 2
  (`ValidateAndRequestTicketWithCustomData`, the shape a NEX 4.0.0 title uses), with the
  console's BAAS id token proven by nx-baas and the OpenPak account behind it.
- **Matchmaking** — Match Making, Matchmake Extension and Match Making Ext, all from
  `nex-protocols-common-go`, backed by Postgres. Worldwide and regional races, VS lobbies,
  session URLs, participation tracking, and cleanup when a console drops without saying so.
- **NAT traversal** — the probe methods MK8D uses to open a path between peers.
- **Utility** — NEX unique IDs, and empty integer/string settings maps, which is what MK8D
  needs and what it asks for immediately after registering.
- **Ranking** — time-trial score uploads and the per-player "common data" blob other racers
  read for a name and Mii, in two Postgres tables.
- **Friends** — from nx-baas, so there is no friend table here.

**Not implemented:** MK8D's own matchmake-extension methods above `0x36` — tournaments and
private-room codes. They answer `Core::NotImplemented`; see
[`docs/design.md`](docs/design.md#what-is-not-implemented). DataStore is not registered
either, because MK8D does not use it.

## Run

```sh
go build ./cmd/mk8d
NEX_PORT_BASE=22150 NEX_ACCESS_KEY=… NX_INTERNAL_KEY=… MK8D_DATABASE_URL=… ./mk8d
```

Copy [`example.env`](example.env) and fill it in. Startup fails loudly on a bad port block, a
missing access key, a missing internal key or an unreachable database rather than serving
something that cannot work.

| Env | Meaning |
| --- | --- |
| `NEX_PORT_BASE` | **required.** `22150` for this title ([ports.md](https://docs.openpak.org/contributors/ports/)): authentication on `+0`, secure on `+1`, `/healthz` on `+2` |
| `NEX_ACCESS_KEY` | **required.** MK8D's NEX access key. Per title, derivable from the game; configuration, never a literal in this repository |
| `NX_INTERNAL_KEY` | **required.** nx-baas's `NX_INTERNAL_KEY`, sent as `X-Internal-Key`. Without it every login is refused |
| `MK8D_DATABASE_URL` | **required.** Postgres URL. Matchmaking, utility and ranking keep their state here and create their schemas on first run |
| `NX_INTERNAL_URL` | nx-baas game/internal API, default `http://127.0.0.1:20070` |
| `NEX_VERSION` | default **`4.0.0`** for this title, not the host's `4.4.0`. Decides the structure encoding and which login method the console uses |
| `NEX_TRANSPORT` | default **`websocket`** for this title. A Switch speaks PRUDPLite over a WebSocket; TLS belongs to the front proxy, so these listeners are plain `ws` |
| `NEX_STATION_SCHEME` | default **`prudp`** for this title, not the host's `prudps` |
| `NEX_SECURE_HOST` | address the console is told to reach the secure server on, default `127.0.0.1` |
| `NEX_SECURE_STREAM_ID` | default `1`, which gives two listeners. Set it to `2` if a capture shows MK8D switching virtual port on one connection; the host then serves both endpoints on `+0` |
| `NEX_SECURE_PID` | secure server principal ID, default `2`, which is MK8D's observed value |
| `NEX_TITLE` | name in logs, default `mario-kart-8-deluxe` |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:<NEX_PORT_BASE+2>` |

Firewall and routing: the two PRUDP ports are TCP WebSocket listeners, and the console
reaches them through the front proxy on 443, not directly. The HTTP port stays internal.

## Test

```sh
go build ./...
go vet ./...
go test ./...
```

The tests cover the parts that are not one-liners: the four per-title configuration defaults
(and that the environment still wins over them), that the defaults make a config the host
accepts, and that the Switch byte-stream profile lands on both endpoints — the setting that
silently misaligns every structure if it is wrong.

## Ground rules

- Clean room. Read [clean-room policy](https://docs.openpak.org/contributors/clean-room/) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs, and every fact taken from an existing implementation is recorded
  in [`docs/provenance.md`](docs/provenance.md) as a fact, never as code.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://docs.openpak.org/contributors/ports/); this title holds 22150.

This project is not affiliated with or endorsed by Nintendo, Mario Kart 8 Deluxe's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
