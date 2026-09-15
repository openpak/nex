# Design

What Mario Kart 8 Deluxe needs from a server, what this repository does, what it does not,
and why.

## What the title needs

MK8D's online mode is peer-to-peer. Once twelve consoles have found each other, the race
itself runs between them over Pia and the server sees nothing but keep-alives. So the server
is a lobby broker with three side jobs:

1. **Log the console in.** Ticket-granting on the authentication endpoint, then a kerberos
   ticket the console presents to the secure endpoint.
2. **Find or make a gathering.** `AutoMatchmakeWithParamPostpone` for worldwide and regional
   races, `CreateMatchmakeSessionWithParam` / `JoinMatchmakeSessionWithParam` for the rest,
   and `GetSessionURLs` so joiners learn how to reach the host.
3. **Get the consoles pointed at each other.** NAT traversal probes.
4. **Keep two small piles of state.** A NEX unique ID per console (Utility), and per-player
   time-trial scores plus an opaque "common data" blob other players read for a racer's name
   and Mii (Ranking).

[`provenance.md`](provenance.md) lists every protocol and method MK8D was observed calling,
and where that observation comes from.

## What this repository is

Wiring. Pretendo's AGPL-3.0 `nex-protocols-common-go` already implements every protocol in
that list, and [`servers/nex`](../../../README.md) already implements the Switch login and OpenPak
identity on top of `nex-go`. `cmd/mk8d` is two files:

- `main.go` — MK8D's per-title configuration defaults, the Switch byte-stream profile, and
  one `RegisterServiceProtocol` call per protocol.
- `ranking.go` — the only backend this title needs that no library ships: two Postgres
  tables behind Ranking's three methods.

There is no MK8D package, no interface, no handler registry. A protocol that
`nex-protocols-common-go` implements is registered and left alone.

## The four per-title defaults

`config()` overrides four things `servers/nex` defaults differently, and every one of them
stays settable from the environment because every one of them is a claim a capture could
overturn:

| Setting | MK8D | Host default | Why |
| --- | --- | --- | --- |
| `NEX_VERSION` | `4.0.0` | `4.4.0` | MK8D reports 4.0.0. This decides the structure encoding, and it decides that the console logs in with ticket-granting **method 2**, not method 6 — a 4.4.0+ title uses method 6 |
| `NEX_TRANSPORT` | `websocket` | `udp` | A Switch NEX title speaks PRUDPLite over a WebSocket. UDP is the Wii U and 3DS form. This is not MK8D-specific; it is true of every Switch NEX title |
| `NEX_STATION_SCHEME` | `prudp` | `prudps` | The scheme MK8D's secure station is advertised with |
| `NEX_TITLE` | `mario-kart-8-deluxe` | `nex` | logs only |

Stream IDs stay at the host default of 1 and 1, which means **two listeners**: authentication
on `22150`, secure on `22151`. That is the shape MK8D was observed running in (`sid=1` in its
secure station URL, and a separate secure port), not the one-listener two-stream shape
`servers/nex` documents as the retail Switch form. If a capture shows MK8D switching virtual
port on the connection it already has, set `NEX_SECURE_STREAM_ID=2` and the host collapses to
one listener with no other change here.

## The Switch byte-stream profile

`switchByteStream` sets two things on both endpoints:

- **8-byte PIDs.** `nex-go` defaults to 4.
- **Structure headers on.** A NEX 3.5+ structure is prefixed with a version byte and a u32
  content length. `nex-go` defaults to off.

Neither has a library-version gate — `ByteStreamOut.PIDSize()` and `UseStructureHeader()`
read the setting directly — so they must be set by hand. Get either wrong and every field
after the first in every structure is read at the wrong offset, which does not fail cleanly:
the console reads garbage and gives up somewhere later.

This is per-console, not per-title. It belongs in `servers/nex`, and the `ponytail:` comment
in `main.go` says so; it lives here only because the host has no Switch profile yet.

## Ranking, the one backend

Two tables, created on first run the way the common protocols create theirs:

- `ranking.common_data` — one blob per NEX unique ID. Nothing on the server interprets it.
- `ranking.scores` — one row per player per category, last write wins.

Three callbacks are set: `InsertRankingByPIDAndRankingScoreData`, `UploadCommonData`,
`GetCommonData`. The five leaderboard **reads** (`GetRanking`, the cached top-X pair, the
friends and nearby variants) are left unset on purpose. The common handlers check their
callback and answer `Core::NotImplemented` rather than crashing, so leaving them unset is a
real answer, and MK8D was never observed calling them. Adding them later is filling in
function fields — the handlers, wire types and ordering are already written upstream.

Storing scores that nothing reads back is deliberate: the alternative is answering
`NotImplemented` to `UploadScore`, which discards data we cannot recover once players start
setting times.

## DataStore

**MK8D does not use it, so it is not registered.**

This is worth saying explicitly, because the general instruction for this port was to
implement DataStore properly wherever the earlier server stubbed it. For MK8D there is
nothing to fix: earlier network registers no DataStore handler for this title, its NEX core
has no general DataStore implementation at all (only a Super Mario Maker 2 one), and its live
monitoring — which names every protocol it has seen from a real client — has no DataStore
entry. Registering Pretendo's DataStore here would mean standing up an S3 bucket and fifteen
database callbacks for a protocol this title never speaks.

The capability is real and it is worth using where it applies: Super Mario Maker 2, Splatoon
2's SplatNet data and Super Smash Bros. Ultimate all put user-generated content in DataStore,
and `servers/nex` documents how to wire Pretendo's implementation. It just does not apply to
Mario Kart.

## What is not implemented

### MK8D's own matchmake-extension methods

Method IDs above `0x36` on protocol `0x6D` are the title's own subclass: tournaments
(`0x39`, `0x3A`, `0x3F`), the playing-session query (`0x3C`), the friends query (`0x41`), and
private rooms with a shareable code (`0x44`, `0x45`). Pretendo's `nex-protocols-go` stops at
`0x36`, so all of them currently answer `Core::NotImplemented`.

They are not implemented because the response layouts are not something we can guess and we
have no capture of our own. `provenance.md` records the shapes earlier network measured for
each one, which is enough to start from but not enough to ship blind.

The upgrade path is already in the library and costs nothing to take when the evidence
arrives: `matchmake_extension.Protocol` has a `Patches nex.ServiceProtocol` field and a
`PatchedMethods []uint32` list, and `HandlePacket` routes a listed method ID to the patch
before its own switch. So a small MK8D protocol registered as the patch adds these methods
without touching anything else.

Consequence today: worldwide and regional races and normal VS lobbies work; tournaments and
room codes do not.

### Nothing else deliberately

Everything else MK8D was observed calling is registered and backed by the common
implementation.

## What has never been run

No console, no emulator, no capture. The server builds, starts, serves `/healthz`, listens
on its block, creates its schemas and answers RMC. Every protocol claim above rests on
earlier network's observations or on Pretendo's library, not on OpenPak seeing a packet.

## Blocking issue in `servers/nex`

`login.go` mints a **16-byte** source key. earlier network records, from a live MK8D server,
that the Switch kerberos key size is 32 bytes and that a 16-byte source key makes the console
reject the ticket. If that holds, no Switch title on OpenPak can complete a login until
`sourceKeyLength` becomes 32. Nothing in this repository can work around it — the constant is
unexported and the handlers belong to the host.

## Evidence the next round needs

In the order that unblocks the most:

1. **Source key length.** Does a console accept a ticket encrypted under a 16-byte key? This
   gates every Switch NEX title, not just MK8D.
2. **The login packet.** Method ID (expect 2), `apiVersionGeneral` (expect 40000), the
   `userName` string, and the byte length of the token in `AuthenticationInfo` — the token
   field has an observed ceiling of 918 bytes and a full BAAS id token JWT may not fit.
3. **The CONNECT packet's virtual port**, which settles whether MK8D uses two listeners
   (`sid=1`, what this server assumes) or one listener with the secure endpoint on stream 2.
4. **The station URL scheme**, `prudp` versus `prudps`. The wrong one is not a clean failure:
   menus keep working and the title gives up later, when it needs the session.
5. **An `AutoMatchmakeWithParamPostpone` round trip** — whether Pretendo's common matchmaking
   answers something MK8D accepts, or whether the title's search criteria need the cleanup
   hooks (`CleanupMatchmakeSessionSearchCriterias`) other titles need.
6. **Any RMC method that comes back `NotImplemented`** in a real session. That list is the
   real backlog, and it is where the `0x39`–`0x45` block will show up.
