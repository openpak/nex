# Notes: nex

The shared NEX host and, under `titles/`, every OpenPak NEX title. One module, one image
(`ghcr.io/openpak/nex`), one tag per release; a container runs one title, chosen by `NEX_TITLE`.
The shared NEX host every OpenPak NEX title imports. Thin by design: Pretendo's
AGPL-3.0 [`nex-go`](https://github.com/PretendoNetwork/nex-go),
[`nex-protocols-go`](https://github.com/PretendoNetwork/nex-protocols-go) and
[`nex-protocols-common-go`](https://github.com/PretendoNetwork/nex-protocols-common-go) do
PRUDP, kerberos and the standard protocols. This adds the three things they leave to us —
Switch token login, OpenPak identity, and the OpenPak service shape. **Not a from-scratch NEX
implementation.**

## Status

Status (2026-09-09): the host builds, runs, serves `/healthz`, listens on a configured NEX
block, and completes the ticket-granting handshake for a Switch login whose id token nx-baas
proves. **Nothing has been run against a console or a real title.** The wire shapes are no
longer guesses — they are taken from working servers and public documentation, each recorded
with its source in [`provenance.md`](provenance.md) — but one dependency is open:
nx-baas cannot yet prove a *real console's* BAAS id token, only an emulator's bound claim, so
a genuine console's login is still refused. See the end of [`docs/design.md`](design.md).

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

Thirty-six of Pretendo's AGPL game servers live here too, and `titles/mario-kart-8` is OpenPak's Go port of Pretendo's Python Mario Kart 8 server, one package each, history merged.
They are the other `Title` shape: `Run` instead of `Register`. Upstream's code builds its own
authentication and secure PRUDP servers over UDP and logs consoles in through the account
server (nn-account) and friends server (nn-friends) over gRPC; `title.go` in each package only
translates the host Config (port block, secure host, database, gRPC addresses and keys) into
the `PN_*` environment upstream reads, then calls upstream's former `init()` and `main()`.
`OPENPAK.md` beside each upstream README lists the mapping and the pass-through variables.
Consoles find these servers through nn-account's `servers` table, not Traefik.

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
[`docs/design.md`](design.md) for the full worked example.

## Ground rules

- Clean room. Read [clean-room policy](https://docs.openpak.org/contributors/clean-room/) before
  writing a line. Public protocol documentation, our own captures, and compatibly licensed
  code are the only inputs, alongside facts read from the implementation being replaced.
  Every protocol fact cites its source in `provenance.md`.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.
- Ports come from [`ports.md`](https://docs.openpak.org/contributors/ports/). Claim your block in the PR that first needs it.

This project is not affiliated with or endorsed by Nintendo, NEX's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.

## License

AGPL-3.0-only — see `LICENSE`.
