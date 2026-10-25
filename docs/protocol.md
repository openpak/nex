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
