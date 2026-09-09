# Design

What Splatoon 2 needs from a server, what this repository implements, and the straight
answer on DataStore — the reason this title was worth porting.

## The shape of it

Five files of Go, one binary. `openpak/nex` brings the PRUDP/WebSocket transport, the kerberos
handshake, the Switch login (including the method-2 login this title uses) and OpenPak account
identity; `nex-protocols-common-go` brings working implementations of every service protocol
this title speaks. What is written here is only what neither of them can know:

| File | What it is |
| --- | --- |
| `main.go` | which protocols the title speaks, and the order they are wired in |
| `datastore.go` | where DataStore objects live (Postgres + presigned S3) |
| `ranking.go` | where leaderboards live |
| `schema.go` | the two Postgres schemas those two own |
| `splatfest.go` | the Splatoon 2 methods that sit above the standard protocols |

There is no wrapper around any of it. A protocol the common library implements is registered
and left alone; a method above the standard set is answered through nex-go's own `Patches` hook.

## What the title needs

The protocol set is the one the earlier server's server registers (see [`provenance.md`](provenance.md)):
match-making, matchmake-extension, match-making-ext, NAT traversal, ranking, utility, DataStore,
plus the secure-connection protocol `servers/nex` registers itself. Gameplay is peer to peer
once matched — the server's job ends when two consoles have each other's station URLs.

**Login is method 2, not method 6.** Splatoon 2 is a NEX 4.0.0 title and logs in with
`LoginEx` (`ValidateAndRequestTicketWithCustomData`, method `0x2`), where later titles like SSBU
use `ValidateAndRequestTicketWithParam` (`0x6`). `servers/nex` already wires both, so this needs
no code here — but it is the one behaviour that separates Splatoon 2 from every method-6 sibling,
and it is why `NEX_VERSION=4.0.0` matters beyond a version string.

## DataStore: implemented, not stubbed — the point of the port

The earlier server has **no** DataStore at all. `earlier-nex` never implemented the protocol,
so its server answers an empty RMC success to *every* `0x73` method and logs which was called.
Its own README says this is why matchmaking is incomplete out of the box: the calls a match
depends on return nothing.

Pretendo's `nex-protocols-common-go/v2/datastore` implements the protocol properly and asks
only where objects live. `datastore.go` answers that with Postgres, so these calls now do what
they say instead of returning an empty success:

| Method | Was, on earlier | Is, here |
| --- | --- | --- |
| `GetMeta` (8) | empty success | the object's real metadata, ratings included |
| `GetMetas` (9), `GetMetasMultipleParam` (36) | empty success | the objects that exist |
| `PostMetaBinary` (21) | empty success (no id returned) | a row, and the id of that row |
| `PreparePostObject` (24), `CompletePostObject` (26), `CompletePostObjects` (37) | empty success | a real object, with a presigned upload URL when a bucket is configured |
| `PrepareGetObject` (25) | empty success | a presigned download URL for a stored object |
| `SearchObject` (12) | empty success | objects matching owner, data type, tags, refer id and time range, ordered and paged as asked |
| `ChangeMeta` (38) | empty success | the period, meta binary or data type actually changes |
| `DeleteObject` (4) | empty success | the object stops being found |
| `RateObject` (15) / `RateObjects` (40) | empty success | a rating slot's running total |

Every one of these was the *same* empty-success stub on earlier, so **all** of them are made
real here — this is a broader fix than the Mario Tennis Aces port, where earlier had at least
special-cased `GetMeta` and `PostMetaBinary`.

Persistence slots are real too: an object posted into a slot replaces the slot's previous
occupant, and deletes it when the client asked for that. Friends-only object permissions work,
because `GetUserFriendPIDs` is wired to OpenPak's friend graph through nx-baas.

Object **bodies** need an S3-compatible bucket, which is optional: without
`DATASTORE_S3_ENDPOINT` the server says so once at startup and the two calls that need a
presigned URL return an error instead of crashing. The metadata half — which is the
matchmaking-relevant part — works without a bucket.

Access and update passwords are stored but never set, because nothing this title posts sets one;
the checks are written as "no password, or the right one" and are marked in the source.

## Ranking: standard protocol over Postgres

the earlier server's Splatoon 2 answered only its custom splatfest methods (18/19/25) and delegated the
rest to a core handler that has no standard Ranking implementation. Here the standard Ranking
protocol is `nex-protocols-common-go` over two Postgres tables, exactly as in Mario Tennis Aces:

- an uploaded score keeps the better of the two by the category's own ordering, or overwrites
  outright when the client asks for that;
- ranks come from a window function — `RANK()` for standard "1224" ties, `ROW_NUMBER()` for
  strictly ordinal "1234", ties broken by earlier upload then lower PID;
- the friends and friends-nearby modes rank within the caller's OpenPak friend graph.

Which Splatoon 2 leaderboards read through this (X Power, league) is not settled, and does not
change the code — the ordering comes from the score data the title uploads. Until a console has
been through a Rankings page, "standard Ranking is implemented" is the honest claim.

## Splatfest and the other above-standard methods

Splatoon 2 calls six method IDs that are above their protocol's standard set, so
`nex-protocols-go` would answer `Core::NotImplemented` and the title would show `2306-0103` or
fail its network bring-up. `splatfest.go` answers exactly these through nex-go's
`Protocol.Patches` hook: the protocol routes only the IDs listed in `PatchedMethods` to the
patch and runs its own switch for everything else, so there is no double response and no risk of
shadowing a real method.

- `Utility::UpdateCurrentUser` (0x6E.10) → **empty success**. This is the gate: it is what lets
  the title cross to `cNetworkReadySuccess`. The empty body is the earlier server's *measured* value, not
  a guess.
- `Utility::AcquireTagId` (0x6E.9) → a benign non-zero `u64` from the caller PID.
- `Ranking` 0x70.18 / 0x70.19 / 0x70.25 (splatfest queries) → empty list. S2's official servers
  are gone, so a live splatfest can never be observed; an empty list is the best-grounded reply.
- `MatchmakeExtension` 0x6D.60 (custom get-simple-playing-session) → empty list.

## What is not implemented

- **The Reference station relay.** An operational P2P-NAT relay Reference arms per deployment.
  Gameplay is P2P once matched and the common NAT-traversal protocol covers the handshake, so a
  relay is out of scope here.
- **`openParticipationOnCreate` / `FindMatchmakeSessionByParticipant`.** the earlier server's Mario Tennis
  Aces needed these; its Splatoon 2 does not add them, and there is no evidence S2 needs them.
  A capture that shows the closed-session race or a friend-join call is a next-round change.
- **Per-player rating history.** A player can rate the same object twice.
- **Live splatfest data.** The fest queries answer empty because no live fest can be observed.
- **Block lists, ranking time-scope/group filters.** OpenPak has no block list; boards are
  all-time and unfiltered.

## Two things that will decide whether it works on hardware

1. **The public station type (Pia < 5.19).** the earlier server's proven server answers `Register` with a
   public station of type `0x03` and no `Pa` param; SSBU's is `0x0B` + `Pa`. Whether Pretendo's
   common `register` shape satisfies Splatoon 2 is unverified, and a wrong shape fails late. This
   lives in `nex-protocols-common-go`/`servers/nex`, not here — see the risk table in
   [`provenance.md`](provenance.md).
2. **`NEX_VERSION`.** `4.0.0` is the earlier server's runtime value, not a capture. It also selects the
   Switch byte-stream profile and the method-2 login routing, so it is load-bearing. The first
   capture of `apiVersionGeneral` settles it.

Everything else is checkable now: the server starts, serves `/healthz`, listens on its block,
creates its schemas, registers every protocol including a real DataStore, and its queries have
been parsed and run against a real Postgres.

## Sources

Every protocol fact cites its origin in [`provenance.md`](provenance.md).
