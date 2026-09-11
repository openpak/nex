# nex

The shared NEX host and, under `titles/`, every OpenPak NEX title. One module, one image
(`ghcr.io/openpak/nex`), one tag per release; a container runs one title, chosen by `NEX_TITLE`.
The shared NEX host every OpenPak NEX title imports. Thin by design: Pretendo's
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
- Will host the shared DataStore object store (`github.com/openpak/nex/datastore`, agreed and
  specified in [`docs/design.md`](docs/design.md), **not yet built** — three titles have their
  own today and should not add a fourth).
- Makes Pretendo's complete DataStore usable: `Host.FriendPIDs` fills the friends-only
  permission callback that upstream documents as not working on Switch for want of a friend
  graph. Titles whose whole online mode is DataStore depend on it.
- Runs the authentication and secure endpoints on one port block from
  [`ports.md`](../../ports.md), with an HTTP `/healthz` because nothing in OpenPak infers
  liveness from a UDP socket.

## Titles

Each game is a package `titles/<game>` exposing one value:

```go
var Title = nexhost.Title{Name: "arms", Defaults: defaults, Register: Register}
```

`Defaults` sets the per-title facts (NEX version, stream shape, port block) that the
environment may still override; `Register` puts the title's protocols on `host.Secure`.
`titles/titles.go` lists them all — adding a game is a new package and one line there.
`cmd/nexd` reads `NEX_TITLE`, builds the config as `Defaults → title → environment` and runs
that one title; with no `NEX_TITLE` it is the bare host below. Each title keeps its own README,
docs and example.env in its directory. The former per-title repositories are archived; their
history is merged here.

## Wii U and 3DS titles (Pretendo-derived)

Twenty-two of Pretendo's AGPL game servers live here too, one package each, history merged.
They are the other `Title` shape: `Run` instead of `Register`. Upstream's code builds its own
authentication and secure PRUDP servers over UDP and logs consoles in through the account
server (nn-account) and friends server (nn-friends) over gRPC; `title.go` in each package only
translates the host Config (port block, secure host, database, gRPC addresses and keys) into
the `PN_*` environment upstream reads, then calls upstream's former `init()` and `main()`.
`OPENPAK.md` beside each upstream README lists the mapping and the pass-through variables.
Consoles find these servers through nn-account's `servers` table, not Traefik.

## Deployed shape

Production runs the passthrough form: Traefik routes each title's `g<id>-lp1.s.n.srv.nintendo.net`
SNI to the title container's `+0` port and the title terminates TLS with one OpenPak-CA leaf
covering `*.s.n.srv.nintendo.net` (`NEX_TLS_CERT`/`NEX_TLS_KEY`). Two-listener titles also
publish `+1` on the host, because the secure station URL names that port directly. See
[`ports.md`](../../ports.md).

## Use (as a library)

```go
host, err := nexhost.New(nexhost.FromEnv())
if err != nil {
	log.Fatal(err)
}
host.Secure.RegisterServiceProtocol(myProtocol) // register BEFORE NewCommonProtocol
log.Fatal(host.Run())
```

Order matters: `nex-protocols-common-go`'s `NewCommonProtocol` reads the endpoint off the
protocol, and `RegisterServiceProtocol` is what sets it. Building the common protocol first
panics with `interface conversion: nex.EndpointInterface is nil`.

`host.Auth` and `host.Secure` are plain `nex.PRUDPEndPoint`s — register nex-go protocols on
them directly. `host.PlayerByPID` answers the one question nex-go cannot: who this PID is on
OpenPak. `host.SecureStationURL` is what the login response tells the console about the secure
server; overwrite it before `Run` to match a retail-observed shape. See
[`docs/design.md`](docs/design.md) for the full worked example.

A Switch title typically wants:

```sh
# websocket is already the default; if Traefik passes the SNI through rather than
# terminating it, this server needs the cert:
NEX_TLS_CERT=/certs/title.crt NEX_TLS_KEY=/certs/title.key
```

### The per-title values

Three settings are genuinely per title, and **every one of them fails silently or late** —
none produces a red test or a useful error. That is why they are stated here with their
failure mode, not just their value.

| Setting | Value | Known for | Failure mode when wrong |
| --- | --- | --- | --- |
| `NEX_SECURE_STREAM_ID` | `1` (default) — two listeners, `+0` auth and `+1` secure, real dialable secure URL | ARMS, Mario Golf, Mario Party Superstars | Not silent, for once: nex-go logs `trying to connect to unbound PRUDPEndPoint 2` for every packet. **That line means set it to `2`** |
| | `2` — one listener on `+0`, placeholder secure URL, `+1` reserved | Super Mario Bros. 35 | Console reaches nothing on the secure stream |
| `NEX_STATION_SCHEME` | `prudps` (default) | ARMS, Mario Party Superstars, Splatoon 2, SSBU | **Late.** Under `prudp` the handshake completes but CONNECT carries an empty payload, so the title works through menus and dies the moment it first needs the session |
| | `prudp` | Mario Kart 8 Deluxe | As above, mirrored |
| `NEX_SOURCE_KEY_BYTES` | `32` (default) | Mario Kart 8 Deluxe, Mario Party Superstars — both derive it with SHA-256 and return all 32 | **Fully silent.** The login fails with nothing logged on either side |
| | `16` | a public reference server working against a retail Super Mario Bros. 35 | As above |

On the source key specifically: the servers that use 32 tend to carry a comment claiming it
*must* be 32, "the Switch kerberos key size". **That generalisation is wrong** — the 16-byte
counter-example is a server that works against a retail console. RC4 takes any key length and
the console is told the key rather than deriving it, so the only thing that must hold is that
the key the ticket is encrypted with is the key advertised in the response. Do not propagate
the "MUST".

Title servers are encouraged to pin all three in their own repository rather than inherit the
default here, so the measured value lives next to the measurement.

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
