# Design

What Mario Tennis Aces needs from a server, what this repository implements, what it does
not, and the straight answer on Rankings.

## The shape of it

Four files of Go, one binary. `openpak/nex` brings the PRUDP/WebSocket transport, the
kerberos handshake, the Switch login and OpenPak account identity;
`nex-protocols-common-go` brings working implementations of every service protocol this
title speaks. What is written here is only what neither of them can know:

| File | What it is |
| --- | --- |
| `main.go` | which protocols the title speaks, and the order they are wired in |
| `matchmaking.go` | the two matchmaking behaviours the common implementation does not cover |
| `datastore.go` | where DataStore objects live |
| `ranking.go` | where leaderboards live |
| `schema.go` | the two Postgres schemas those two own |

There is no wrapper around any of it. A protocol the common library implements is registered
and left alone.

## What the title needs

The protocol set is the one the console was observed to use (see
[`provenance.md`](provenance.md)): match-making, matchmake-extension, match-making-ext,
NAT traversal, ranking, utility, DataStore, plus the secure-connection protocol
`servers/nex` registers itself. Gameplay is peer to peer once matched — this title uses real
P2P, so nothing relays match traffic and the server's job ends when two consoles have each
other's station URLs.

Two things about this title are not in any library:

**A created session arrives closed.** The client creates a matchmake session with
participation closed and only opens it in a separate, later call. Every search filters on
`open_participation = true`, so in that window two searching players miss each other, both
create a session, and both wait for a join that never comes. `openParticipationOnCreate`
marks the session open as it is created, on all five methods that can create one. This was a
real, diagnosed deadlock, not a theory.

**Playing with a friend needs `FindMatchmakeSessionByParticipant`.**
`nex-protocols-common-go` has no handler for method `0x33`, so it answers
`Core::NotImplemented` and a friend's open room is never found. The handler here is fifteen
lines over the common library's own playing-session query.

## DataStore: implemented, not stubbed

A server with no DataStore implementation answers an empty list to every DataStore method,
with two hand-written exceptions: `GetMeta` answers `NotFound`, and `PostMetaBinary` answers
a made-up, unstored object id (answering it with the generic empty list left the client with
no id for the tournament data it had just posted, and it died silently).

Pretendo's `nex-protocols-common-go/v2/datastore` implements the protocol properly and asks
only where objects live. `datastore.go` answers that with Postgres, so these calls now do
what they say:

| Method | Without a store | Is, here |
| --- | --- | --- |
| `GetMeta` (8) | always `NotFound` | the object's real metadata, ratings included |
| `GetMetas` (9), `GetMetasMultipleParam` (36) | empty list | the objects that exist |
| `PostMetaBinary` (21) | an id nothing stored | a row, and the id of that row |
| `PreparePostObject` (24), `CompletePostObject` (26), `CompletePostObjects` (37) | empty list | a real object, with a presigned upload URL when a bucket is configured |
| `PrepareGetObject` (25) | empty list | a presigned download URL for a stored object |
| `SearchObject` (12) | empty list | objects matching owner, data type, tags, refer id and time range, ordered and paged as asked |
| `ChangeMeta` (38) | empty list | the period, meta binary or data type actually changes |
| `DeleteObject` (4) | empty list | the object stops being found |
| `RateObject` (15) / `RateObjects` (40) | empty list | a rating slot's running total |

Persistence slots are real too: an object posted into a slot replaces the slot's previous
occupant, and deletes it when the client asked for that.

Object **bodies** need an S3-compatible bucket, which is optional here: without
`DATASTORE_S3_ENDPOINT` the server says so once at startup and the two calls that need a
presigned URL return an error instead of crashing. Nothing observed from this title uploads
an object body — it posts metadata — so this is deliberately not a startup requirement.

Access and update passwords are stored but never set, because nothing sets one; the checks
are written as "no password, or the right one" and are marked in the source.

## Rankings: the straight answer

**It was a missing implementation, not a missing fact, and it is implemented here.**

A Ranking handler written only for the *custom* methods Mario Kart 8 Deluxe
uses — profile upload, competition list, competition ranking, score and time submission —
answers `Core::NotImplemented` in its default branch. Mario Tennis
Aces on such a handler has every **standard** Ranking method it calls refused.
That is exactly the shape of "fully functional besides the Rankings page".

Pretendo's `nex-protocols-common-go/v2/ranking` implements the standard protocol —
`UploadScore`, `UploadCommonData`, `GetCommonData`, `GetRanking` in all five ranking modes,
and `GetCachedTopXRanking`/`GetCachedTopXRankings` — and asks only where the rows live.
`ranking.go` answers with two Postgres tables and lets the database do the ranking:

- an uploaded score keeps the better of the two by the category's own ordering, or overwrites
  outright when the client asks for that;
- ranks come from a window function — `RANK()` for standard "1224" ties, `ROW_NUMBER()` for
  strictly ordinal "1234", ties broken by earlier upload then lower PID, as the protocol
  describes;
- the friends and friends-nearby modes rank within the caller's OpenPak friend graph, which
  nx-baas fills, so there is no per-title friend table.

What is **not** settled is which categories this title uses, whether its boards are ascending
or descending, and whether its Rankings page reads through `GetRanking` or the cached
top-X calls. None of that changes the code — the ordering comes from the score data the title
itself uploads — but until a console has been through the page, "Rankings is implemented" is
the honest claim, not "Rankings works".

## What is not implemented

- **`SecureConnection::TestConnectivity` (method 5).** This title calls it — 23 times in six
   hours in earlier research. `nex-protocols-common-go` leaves it unset, and the protocol
  is registered inside `servers/nex`, where a title server cannot reach it. Registering a
  second secure-connection protocol here would make both answer, so this needs a one-line
  change in `servers/nex`: an empty success. Until then the title gets
   `Core::NotImplemented` for it, which is what it got in earlier research for months without
  visible harm.
- **The Pia keepalive, PRUDP packet type 8.** nex-go logs "unhandled packet type" and drops
  it. `PRUDPEndPoint.RegisterCustomPacketHandler` is exported, so it can be answered without
  an upstream change — but there is no evidence this title sends one, so nothing is
  registered.
- **Block lists.** `FindMatchmakeSessionByParticipant` ignores the block-list parameter
  because OpenPak has no block list to consult.
- **Per-player rating history.** A player can rate the same object twice.
- **The Ranking `TimeScope` and group filters.** A board is all-time and unfiltered.

## Two things that will decide whether it works on hardware

1. **`NEX_VERSION`.** 4.3.5 is a guess inherited from earlier research notes, which say so
   themselves. The console reports `apiVersionGeneral`; the first capture settles it, and
   `NEX_VERSION` takes the integer form directly (`40305`).
2. **`NEX_STATION_SCHEME`.** `prudps` here, from the observed station URL. The wrong value
   fails late — menus work and the session fails when it is needed.

Everything else is checkable now: the server starts, serves `/healthz`, listens on its block,
creates its schemas, and its queries have been run against a real Postgres.

## Sources

Every protocol fact cites its origin in [`provenance.md`](provenance.md).
