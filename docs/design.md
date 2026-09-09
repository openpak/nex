# Design

What Mario Party Superstars needs from a server, what this repository implements, what it
does not, and why.

## The shape of the thing

Almost nothing here is Mario Party Superstars code. Three layers do the work:

| Layer | What it does | Whose code |
| --- | --- | --- |
| `nex-go`, `nex-protocols-go`, `nex-protocols-common-go` | PRUDP, kerberos, RMC, and working handlers for every standard NEX protocol | Pretendo, AGPL-3.0 |
| [`servers/nex`](../../nex) | Switch login (`ValidateAndRequestTicketWith{Param,CustomData}`), OpenPak identity through nx-baas, the port block, `/healthz` | OpenPak |
| this repository | The three things neither of those covers for this title | OpenPak |

If a line here could have been a call into one of the layers above, it should be. The whole
package is under 800 lines, and three files of it are the title-specific part.

## What this title needs

The set of protocols comes from what the earlier network server for this title registers
(recorded, with sources, in [`provenance.md`](provenance.md)): Secure Connection, Match
Making, Matchmake Extension, Match Making Ext, NAT Traversal, Utility, DataStore, Ranking.

Everything except Utility's settings, two Matchmake Extension methods, and DataStore's
storage is already implemented by `nex-protocols-common-go` and is registered as-is.

### 1. Friend join — the two missing matchmaking methods

`matchmaking.go`. `nex-protocols-common-go` implements most of Matchmake Extension but not
`FindMatchmakeSessionByGatheringIDDetail` (`0x29`) or `FindMatchmakeSessionByParticipant`
(`0x33`), and those two are how this game follows a friend into a lobby:

- The joiner asks **`FindMatchmakeSessionByParticipant`** which gathering the host is in.
  Answered empty — which is what an unimplemented method amounts to — the game gives up on
  the friend and runs a public auto-matchmake instead, ending up in a session of its own.
  This is the "my friend could not see my lobby" failure the earlier network server traced.
- The Join Room screen then polls **`FindMatchmakeSessionByGatheringIDDetail`** roughly once
  a second against that gathering ID, and expects `RendezVous::SessionVoid` once the room is
  gone.

Both are adapters, not a second matchmaking implementation: the queries they need
(`GetPlayingSession`, `GetMatchmakeSessionByID`) already exist in the common library's own
database package, and everything else — creating, joining, leaving, cleaning up after a
dropped connection — is the common protocol untouched.

`FindMatchmakeSessionByGatheringIDDetail` is answered to anyone who knows the gathering ID,
so it clears the session key and user password before replying. Joining is what hands those
over.

### 2. Utility integer settings

`utility.go`. The game calls `GetIntegerSettings` immediately after
`SecureConnection::Register`, and on the earlier network stack the connection died about a
second later, every time, because the reply was malformed — that server's generic handler
read the single index parameter as a list length. Pretendo's implementation parses it
correctly, so the parsing half of that bug does not exist here.

What is left is content, and nobody knows it. There is no capture of this title's settings
and no public reference server. The choice made here is to answer a correctly shaped **empty**
map rather than borrow another title's numbers, and to make the real values loadable from
`MPS_INTEGER_SETTINGS` — a wrong guess should cost a restart, not a release. Everything else
Utility does (NEX unique IDs) is the common protocol, backed by the same Postgres.

### 3. DataStore, implemented rather than stubbed

`datastore.go`. This is the capability OpenPak has that the earlier network stack does not:
`earlier-nex` has no DataStore implementation at all, so their server answers every
DataStore method with a stub — an empty list for most methods and `0x80690004` for
`GetMetaMultiple` — written defensively after Super Smash Bros. Ultimate soft-locked with no
DataStore handler registered.

`nex-protocols-common-go/v2/datastore` implements every method for real, but holds no state:
it asks the operator for object metadata through a set of callbacks and for object bytes
through an S3 presigner. This file is those callbacks. With them, these answer with real,
restart-surviving state instead of a stub:

| Method | What it now does |
| --- | --- |
| `GetMeta`, `GetMetas`, `GetMetasMultipleParam` | Return the object's real metadata, by data ID or by persistence target, with access-password and permission checks |
| `PreparePostObject` | Allocates a data ID, records the metadata, and returns a presigned S3 upload |
| `CompletePostObject`, `CompletePostObjects` | Verify the uploaded size against the declared size and publish the object |
| `PrepareGetObject` | Returns a presigned S3 download |
| `SearchObject` | Real filtering by owner, data type, tags, refer ID and created/updated ranges, with ordering, paging and a total count |
| `ChangeMeta` | Updates period, data type and meta binary under the update password |
| `RateObject`, `RateObjects` | Accumulate votes per rating slot and return the running total |
| `DeleteObject` | Deletes, with the update password and the delete permission enforced |
| `PostMetaBinary` | Records a meta binary against an object |

Only the S3 half is optional: with `MPS_S3_ENDPOINT` unset, the metadata methods still work
and the upload and download methods answer `Core::NotImplemented`, which is exactly what they
did before this file existed. Nothing degrades silently — startup says so.

Object metadata lives in one Postgres table, `datastore.objects`. The whole
`DataStoreMetaInfo` is stored as one serialised blob, with only the fields something queries
by mirrored into columns. That is a deliberate trade: writes are one statement, a future
structure revision needs no migration, and the cost is that the columns must be rewritten
whenever the blob is, which is what `mutate` exists to guarantee.

## What is not implemented

- **Ranking (`0x70`).** the earlier network server registers a generic acknowledgement here
  and its own README says it is unknown whether this title needs real leaderboard data. A
  generic acknowledgement is not better than `Core::NotImplemented` — both tell the game
  "no data" — and a real implementation needs a category and score-format schema that no
  capture has provided. Left out until something shows the game calling it.
- **Messaging, notifications, subscriber.** the earlier network server registers none of
  these either, though its logs show the client *linking* a Subscriber protocol and possibly
  a NAT-traversal-internal one. That is an observation about the game binary, not about what
  it calls. Their fallback logger exists to find out; ours is `nex-go`'s own
  `Core::NotImplemented`, which is the same information in a different place.
- **A Pia keepalive handler.** `earlier-nex`'s PRUDP switch only knows packet types 0–4, so
  it drops Pia's periodic type-8 keepalive and the console gives up on the connection. That
  is a gap in their transport, not a property of the game, and `nex-go` is a different
  transport. Whether it has the same gap is an open question, not something to pre-emptively
  patch around.
- **A presence reporter.** Their server reports active PIDs to their account service every
  30 seconds so the console's friend list shows players as online. OpenPak's equivalent
  belongs in nx-baas, which already owns presence for NPLN titles, rather than in each of
  forty title servers. Worth raising as a fleet-wide question.
- **A dashboard.** `/healthz` plus the Gatus status page is OpenPak's answer to "is it up".

## Deploy — an unresolved fleet problem

There is no `Dockerfile` or release workflow here yet, and adding a broken one would be
worse than none. `servers/nex` is imported as the module path `openpak/nex` through a
`replace` directive pointing at `../nex`. That works for a local build and for `go test`. It
cannot work in a container build or in CI, where the checkout is this repository alone and
`../nex` does not exist.

Every NEX title in the fleet will hit this on its first deploy, so it wants one fleet-wide
answer rather than forty workarounds. The obvious one is for `servers/nex` to take a real
module path (`github.com/openpak/nex`) and be consumed as a tagged dependency. Raised with
the `servers/nex` maintainer; nothing here should be built around a guess at the outcome.

## What the next round needs

In rough order of how much they unblock:

1. **A console that gets past `GetIntegerSettings`.** Everything below is downstream of a
   connection that survives. If it does not, the two candidates are the settings content and
   the Pia keepalive, in that order.
2. **The login capture `servers/nex` is asking for** — token shape, token length, `userName`,
   and whether this title uses method 2 or method 6. NEX 4.6.5 points at method 6, but that
   is inference from a version number.
3. **A two-console friend join.** Host creates a lobby, joiner opens Join Room. What the
   joiner calls, in what order, and with what gathering ID, tells us whether the two methods
   implemented here are the right two and whether the friend graph reaches them.
4. **Any DataStore call at all.** Nobody has ever seen this title make one. The
   implementation is built to the protocol rather than to the title, which is the best that
   can be done without evidence, but the persistence-slot assumption in particular wants a
   real `PreparePostObject` to confirm.
5. **Whether Ranking is ever called.**

## Sources

Every protocol fact, with where it came from, is in [`provenance.md`](provenance.md).
