# nex

The shared NEX host every OpenPak NEX title server imports. Thin by design: Pretendo's
AGPL-3.0 [`nex-go`](https://github.com/PretendoNetwork/nex-go),
[`nex-protocols-go`](https://github.com/PretendoNetwork/nex-protocols-go) and
[`nex-protocols-common-go`](https://github.com/PretendoNetwork/nex-protocols-common-go) do
PRUDP, kerberos and the standard protocols. This adds the three things they leave to us —
Switch token login, OpenPak identity, and the OpenPak service shape. **Not a from-scratch NEX
implementation.**

Status (2026-09-09): the host builds, runs, serves `/healthz`, listens on a configured NEX
block, and completes the ticket-granting handshake for a Switch login whose id token nx-baas
proves. **Nothing has been run against a console or a real title.** The wire shapes are no
longer guesses — they are taken from working servers and public documentation, each recorded
with its source in [`docs/provenance.md`](docs/provenance.md) — but one dependency is open:
nx-baas cannot yet prove a *real console's* BAAS id token, only an emulator's bound claim, so
a genuine console's login is still refused. See the end of [`docs/design.md`](docs/design.md).

## What it does

- Implements `ValidateAndRequestTicketWithParam` (method 6) and
  `ValidateAndRequestTicketWithCustomData` (method 2) — the Switch login methods, which
  `nex-protocols-common-go` leaves unset, so a NEX 4 title otherwise answers
  `Core::NotImplemented` to every login.
- Resolves a console to an OpenPak account through [`nx-baas`](../../nx-baas)'s internal game
  API, the same door [`servers/stardew-valley`](../stardew-valley) uses for NPLN, and hands
  the title the player's friend graph with it. It also records the name the console logged in
  under — its NSA id, which is not its NEX PID — because titles go on to ask for players by
  that name and nothing else can translate it (`Host.PIDForLoginName`).
- Makes Pretendo's complete DataStore usable: `Host.FriendPIDs` fills the friends-only
  permission callback that upstream documents as not working on Switch for want of a friend
  graph. Titles whose whole online mode is DataStore depend on it.
- Runs the authentication and secure endpoints on one port block from
  [`ports.md`](../../ports.md), with an HTTP `/healthz` because nothing in OpenPak infers
  liveness from a UDP socket.

## Use

```go
host, err := nexhost.New(nexhost.FromEnv())
if err != nil {
	log.Fatal(err)
}
host.Secure.RegisterServiceProtocol(myMatchmakeExtensionProtocol)
log.Fatal(host.Run())
```

`host.Auth` and `host.Secure` are plain `nex.PRUDPEndPoint`s — register nex-go protocols on
them directly. `host.PlayerByPID` answers the one question nex-go cannot: who this PID is on
OpenPak. `host.SecureStationURL` is what the login response tells the console about the secure
server; overwrite it before `Run` to match a retail-observed shape. See
[`docs/design.md`](docs/design.md) for the full worked example.

A Switch title typically wants:

```sh
NEX_TRANSPORT=websocket NEX_SECURE_STREAM_ID=2   # one listener, secure on stream 2
# and, if Traefik passes the SNI through rather than terminating it:
NEX_TLS_CERT=/certs/title.crt NEX_TLS_KEY=/certs/title.key
```

The startup line says which of the three shapes is running (`udp`, `ws (TLS at the front
proxy)`, `wss (TLS terminated here)`), because a console failing a TLS handshake against a
plain `ws` listener looks exactly like silence.

In a title server's `go.mod`:

```
require openpak/nex v0.0.0
replace openpak/nex => ../nex
```

## Run

```sh
go build ./cmd/nexd
NEX_PORT_BASE=22100 NEX_ACCESS_KEY=… NX_INTERNAL_KEY=… ./nexd
```

`cmd/nexd` is a title server's `main` minus the protocol registration: deploy it to find out
whether a console reaches us and gets through the handshake, before any title work starts.

| Env | Meaning |
| --- | --- |
| `NEX_TITLE` | name in logs, default `nex` |
| `NEX_PORT_BASE` | **required.** The title's NEX block base from [ports.md](../../ports.md): a multiple of ten in 22100–22199 or 22300–22599. Authentication listens on `+0`, secure on `+1` |
| `NEX_ACCESS_KEY` | **required.** The title's NEX access key |
| `NEX_VERSION` | NEX library version the title was built against, default `4.4.0`. Accepts `4.6.5` or the integer form the console reports, `40605`. Decides which login method the client uses and how structures are serialised |
| `NEX_SECURE_HOST` | address the console is told to reach the secure server on, default `127.0.0.1` |
| `NEX_STATION_SCHEME` | `prudps` (default) or `prudp`, per title. Whether the console treats the secure server as a Pia station. The wrong value is not a clean failure: menus keep working and the title gives up later |
| `NEX_TRANSPORT` | `udp` (default) or `websocket`. **Switch NEX titles speak PRUDP over WebSocket**; UDP is the Wii U/3DS form. Choosing websocket also selects PRUDP Lite — in nex-go they are the same switch |
| `NEX_TLS_CERT`, `NEX_TLS_KEY` | terminate TLS on the websocket listener. A console connects over `wss`, so either Traefik terminates and forwards plain `ws` (leave both empty) or Traefik passes the SNI through and this server terminates (set both). Setting exactly one is refused |
| `NEX_STREAM_ID` | PRUDP virtual port the authentication endpoint binds, default `1` |
| `NEX_SECURE_STREAM_ID` | PRUDP virtual port the secure endpoint binds, default `1`. **Set it different (retail Switch shape: 2) and both endpoints share one listener on `+0`** — the console never opens a second socket, it switches virtual port on the connection it has. `+1` then stays reserved and unused |
| `NEX_SECURE_PID` | secure server's principal ID, default `2`: the kerberos ticket target, the station URL `PID=`, and the source of pushed notifications. Some titles have a retail-observed constant |
| `NEX_HTTP_ADDR` | `/healthz` listener, default `:<NEX_PORT_BASE+2>` |
| `NX_INTERNAL_URL` | nx-baas game/internal API, default `http://127.0.0.1:20070` |
| `NX_INTERNAL_KEY` | **required.** nx-baas's `NX_INTERNAL_KEY`, sent as `X-Internal-Key` |

See [`example.env`](example.env). Startup fails loudly on a bad port block, a missing access
key or a missing internal key rather than serving something that cannot work.

Firewall: the two PRUDP ports are UDP and need explicit `ufw allow <port>/udp` rules
(ports.md rule 4). The HTTP port stays internal.

## Deploy

Tag `v1.2.3` and the release workflow publishes `ghcr.io/openpak/nex:1.2.3` and `:latest` for
amd64 and arm64; the deploy host runs `podman-auto-update` against `:latest`.

## Test

```sh
go build ./...
go test ./...
go vet ./...
```

The tests cover the parts that are not one-liners: config validation, both spellings of a NEX
version, reading the binding claim out of a token *and* accepting a real console's token that
has none, the nx-baas identity call including the refused-token case, that a login name is an
NSA id rather than a PID, the per-title station scheme, and a full kerberos ticket round trip
— minted by the authentication endpoint, opened with the source key the client is handed, and
the internal ticket opened again by the secure server.

## Provenance

Wire-level facts in this repository come from public protocol documentation, from Pretendo's
AGPL libraries, and from reading existing working servers for facts under the clean-room
policy. Every one is listed with its source in [`docs/provenance.md`](docs/provenance.md).
No code, comment, structure or naming was copied from a source-available implementation.

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../docs/clean-room-policy.md) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs, alongside facts read from the implementation being replaced.
  Every protocol fact cites its source in `docs/provenance.md`.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](../../ports.md). Claim your block in the PR that first needs it.

This project is not affiliated with or endorsed by Nintendo, NEX's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
