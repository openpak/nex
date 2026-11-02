# Protocol: nex

## What it does

- Implements `ValidateAndRequestTicketWithParam` (method 6) and
  `ValidateAndRequestTicketWithCustomData` (method 2) — the Switch login methods, which
  `nex-protocols-common-go` leaves unset, so a NEX 4 title otherwise answers
  `Core::NotImplemented` to every login.
- Resolves a console to an OpenPak account through [`nx-baas`](../../../../switch/infra/nx-baas)'s internal game
  API, the same door [`platforms/switch/games/npln/titles/stardew-valley`](../../../../switch/games/npln/titles/stardew-valley) uses for NPLN, and hands
  the title the player's friend graph with it. It also records the name the console logged in
  under — its NSA id, which is not its NEX PID — because titles go on to ask for players by
  that name and nothing else can translate it (`Host.PIDForLoginName`).
- Will host the shared DataStore object store (`github.com/openpak/nex/datastore`, agreed and
  specified in [`docs/design.md`](design.md), **not yet built** — three titles have their
  own today and should not add a fourth).
- Makes Pretendo's complete DataStore usable: `Host.FriendPIDs` fills the friends-only
  permission callback that upstream documents as not working on Switch for want of a friend
  graph. Titles whose whole online mode is DataStore depend on it.
- Runs the authentication and secure endpoints on one port block from
  [`ports.md`](https://docs.openpak.org/contributors/ports/), with an HTTP `/healthz` because nothing in OpenPak infers
  liveness from a UDP socket.

## Bans

An account banned in the OpenPak admin is locked out of every title here
([`website/docs/ban-lookup.md`](../../../../../openpak/website/docs/ban-lookup.md)). Login is refused with
`RendezVous::AccountDisabled` (0x00030067) and live connections are dropped within a minute
by `nexhost.WatchBans`. Switch titles learn the ban from nx-baas (`/internal/switch/identity`
answers 403 `account_banned`); Wii U and 3DS titles from nn-account (`GetNEXPassword` refuses a
banned owner). Nothing to configure.

## Host library and title shape

One module, one image (`ghcr.io/openpak/nex`), one tag per release; a container runs one title,
chosen by `NEX_TITLE`. The shared host is thin: Pretendo's `nex-go`, `nex-protocols-go` and
`nex-protocols-common-go` do PRUDP, kerberos and the standard protocols; this adds Switch token
login, OpenPak identity and the OpenPak service shape.

Each game is a package `titles/<game>` exposing `Title = nexhost.Title{Name, Defaults, Register}`.
`Defaults` sets per-title facts (NEX version, stream shape, port block) that the environment may
override; `Register` puts the title's protocols on `host.Secure`. `titles/titles.go` lists them.
`cmd/nexd` builds the config as `Defaults -> title -> environment`; with no `NEX_TITLE` it is
the bare host.

Library use: `host.Auth` and `host.Secure` are plain `nex.PRUDPEndPoint`s.
`host.PlayerByPID` answers who a PID is on OpenPak; `host.SecureStationURL` is what the login
response tells the console about the secure server (overwrite before `Run`). Register service
protocols BEFORE `NewCommonProtocol`: it reads the endpoint off the protocol and building it
first panics with `interface conversion: nex.EndpointInterface is nil`. Worked example:
`design.md`.

The Pretendo-derived Wii U and 3DS titles (36 AGPL servers plus OpenPak's Go port of Mario
Kart 8) use `Run` instead of `Register`: upstream builds its own auth and secure PRUDP servers
and logs consoles in through nn-account and nn-friends over gRPC. `title.go` translates the
host Config into the `PN_*` environment upstream reads; `OPENPAK.md` beside each README lists
the mapping. Consoles find these servers through nn-account's `servers` table, not Traefik.

Open dependency: nx-baas proves an emulator's bound claim but not a real console's BAAS id
token, so a genuine console's login is refused until it can (end of `design.md`).
