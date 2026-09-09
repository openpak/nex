# Design

What Odyssey needs from a server, what is here, and what is not.

## What this repository is

One binary that registers Odyssey's protocol set on the shared NEX host and gets out of the
way. `servers/nex` owns PRUDP over WebSocket, kerberos, the Switch ticket-granting methods,
SecureConnection, the nx-baas account lookup, the port block and `/healthz`. Pretendo's
`nex-protocols-common-go/v2` owns the DataStore and Utility handlers. What is left — and all
this repository actually is — is two things:

1. Which protocols to register (a short list: Utility and DataStore).
2. A real DataStore backing, because DataStore is not a side feature here — it is the entire
   game, and it is the one thing the common library deliberately leaves to the operator.

`cmd/super-mario-odyssey/main.go` is 1. `datastore.go` and `search.go` are 2.

## What makes Odyssey different

Every other title in the fleet spends its effort on matchmaking — lobbies, NAT traversal,
session search. Odyssey has none of that. Its only online feature is **Balloon World**: place a
balloon (a capture-pose screenshot plus costume snapshot at an in-game location) and other
players search for and pop it, with "clap" ratings. There is no live session, no lobby, no P2P.

So this server is simpler than ARMS/MPS in one direction — it registers no
MatchMaking/MatchmakeExtension/MatchMakingExt/NATTraversal at all — and the DataStore it does
register has to carry the whole title rather than a single event-data sideline.

| Protocol | ID | Why Odyssey needs it |
| --- | --- | --- |
| Utility | `0x6E` | NEX unique IDs — the console acquires one to name a new balloon before posting it |
| DataStore | `0x73` | Balloon World: place, find, rate, delete |

Ranking (`0x70`) is **not** registered; `provenance.md` records why (speculative in the
earlier server, no client observed linking it, three lines to add on evidence).

## Transport

Nothing to set: every `servers/nex` default is already right for Odyssey, and `provenance.md`
records the evidence for each. `NEX_TRANSPORT=websocket` (the earlier server serves both
endpoints over WSS and opens no UDP socket), `NEX_SECURE_STREAM_ID=1` (the two-listener shape:
its auth `:8453` and secure `:60013` are separate servers, and the secure station URL carries
`sid=1`), `NEX_SOURCE_KEY_BYTES=32` (its source key is a 32-byte SHA-256), `prudps`, secure
PID 2. The one value that is *not* confirmed is `NEX_VERSION`, a `4.0.0` guess — the first
thing to change if login never completes.

## State

DataStore metadata (the balloons) and the Utility NEX-unique-ID counter both live in Postgres,
because `nex-protocols-common-go` keeps them there and writing an in-memory store to avoid one
connection string would be more code, not less — and the earlier server's in-memory-map-to-
JSON-file is exactly the durability the OpenPak version is meant to replace. The Utility
library creates its own `utility` schema; `datastore.go` creates `odyssey`. `SMO_DATABASE_URL`
is required and startup pings it: a title whose entire online feature is DataStore is not a
degraded server without a database, it is a dead one, so failing loudly beats starting.

## DataStore — the base protocol, made real

The retail server backs DataStore objects with S3: it tracks metadata and issues a
signed upload/download URL, and the object bytes (screenshot + costume data) never touch it.
`nex-protocols-common-go` is built for exactly that split and ships every base handler; it
needs two things we supply — object metadata storage, and an S3 client for bodies. So the base
methods are implemented here rather than stubbed:

| Call | Method | What now happens |
| --- | --- | --- |
| `GetMeta` / `GetMetas` | 8 / 9 | read by data ID, or by owner + persistence slot |
| `PreparePostObject` | 24 | create the metadata row, return a presigned S3 POST |
| `PrepareGetObject` | 25 | presigned S3 GET for an existing object |
| `CompletePostObject` | 26 | check the uploaded body's size, make the object visible |
| `DeleteObject` | 4 | soft-delete |
| `SearchObject` | 12 | a real query — see below |
| `RateObject` / `RateObjects` | 15 / 40 | apply a clap, return the new totals |
| `PostMetaBinary` | 21 | a metadata-only object |
| `ChangeMeta` | 27 | name, period, data type, meta binary |

**S3 is optional.** With `SMO_S3_ENDPOINT` unset, every metadata call still works and the three
body-movers answer `Core::NotImplemented` — which is exactly what the library does for an unset
callback, so it is not a special case, just an unset one. That keeps the server runnable on a
laptop with only Postgres. Balloon World needs S3 in production, since a balloon is a
screenshot; a half-configured store (endpoint set, credentials missing) is a startup error
rather than a surprise at the first upload.

The DataStore backing is our own code, carried across from `servers/arms` and
`servers/mario-tennis-aces` (OpenPak repositories), not from earlier. It is a **lift
candidate**: `servers/nex` has agreed to move the object store into a shared subpackage,
`github.com/openpak/nex/datastore`, behind a one-call `store.Wire(common, host)`. Nothing in
`datastore.go`/`search.go` is Odyssey-shaped — the only title-specific values are the schema
name `odyssey` and the S3 key bases, and the columns are a transcription of NEX's own
`DataStoreMetaInfo`. Delete both files when the shared store lands. What must change on the way
across is scoping: this keeps rows in a per-title Postgres schema; the shared store wants a
title-key column so one database can hold several titles without a search in one leaking
objects from another.

### Search

`searchQuery` in `search.go` translates a `DataStoreSearchParam` into one SELECT with a
`count(*) OVER ()` for the unpaged total, so a search is one round trip. It handles owner
filters, the friend-owned and public-access search types, data type (both the single-value and
the NEX-4 list form), tags, refer-data-ID and the four created/updated bounds; it orders by a
column from a fixed whitelist map and pages with the client's `ResultRange`. This is where
OpenPak's DataStore is materially more real than the Reference one, which decoded only
`dataType`.

Everything a console sends is a bound parameter. The only text a console influences is the
ORDER BY column, and that comes out of the map or falls back to `data_id`. `search_test.go`
checks exactly that: placeholder count equals argument count, no sent value appears as text,
an unknown sort column falls back, and the page size is capped. Balloon World's search is the
entire game's read path, so the injection-proofing is not incidental.

Two deliberate limits, both marked in the code:

- Rating sort columns fall back to `data_id`. They need a join per slot and no Odyssey search
  has been seen asking for one.
- A search type this code does not translate into SQL simply does not narrow the query. That
  is safe rather than lax: `nex-protocols-common-go` re-checks every returned object against
  the caller's permissions before it goes on the wire, so an untranslated type costs a wasted
  row, never a leaked one.

## DataStore — the Balloon extension, and why it is not here

Odyssey does not only speak the base DataStore protocol. The earlier server implements a
**title-specific extension on `0x73`, methods 47-53** — `AddToBufferQueue(s)`,
`GetBufferQueue(s)`, `ClearBufferQueues` (the pose-screenshot buffers), `SearchBalloon` (the
search the Balloon World UI actually uses, as distinct from generic `SearchObject`), and
`FetchMyInfos`.

**No common library implements any of it.** Pretendo's `nex-protocols-go` ships datastore
extensions for Miitopia, Super Mario Maker, SSB4 and the Pokémon titles, but none for Odyssey.
So the common protocol answers 47-53 with `Core::NotImplemented`, and this server does not
serve them.

This is the honest limit of what this repository delivers, and `provenance.md` records the
sharp edge: `FetchMyInfos` (53) is the **first** DataStore call Odyssey makes — confirmed live
in the Reference notes — so Balloon World does not actually get off the ground on a base-only
server. It is not served here for two reasons that both point the same way:

1. **It is not in any library we build on.** Serving it means a new title-specific protocol —
   a Go definition of the seven methods and their `[Structure]`-wrapped parameter/result types
   — registered alongside the common base protocol on `0x73`. That is real protocol work, not
   a callback.
2. **Its wire shape is unconfirmed.** The earlier server's own README says none of
   `SearchBalloon`'s per-kingdom/rank result bucketing or `AddToBufferQueue`'s slot field width
   is cross-checked against a real capture. Reproducing those guesses would be building on an
   unconfirmed layout, and doing it by reading their handlers would mirror their shape — both
   things the clean-room policy and plain caution say to avoid.

So the extension is left for the round that has a capture. The right home for it, once the wire
shape is known, is a title module (mirroring how Pretendo structures `datastore/super-mario-
maker`) that registers on the same secure endpoint. The one confirmed fact to build on is that
`FetchMyInfos`'s 45-byte body carries 17 `balloonDataTypes` entries behind the standard Switch
NEX-4 structure header — and that header is already handled, because `servers/nex` sets
`UseStructureHeader = true` for NEX ≥ 4.0.0.

## Identity and friends

`servers/nex` resolves a console to an OpenPak account through nx-baas and hands back the
player's friend graph with it. `host.FriendPIDs` is given to DataStore's `GetUserFriendPIDs`,
which is the callback documented as not working on Switch upstream — it asks by 32-bit PID and
Pretendo has no Switch friend graph. OpenPak does. For Odyssey that is not a nicety: Balloon
World's "friends' balloons" view is a friends-only DataStore search, so the friend graph is the
visibility rule. `host.FriendPIDs` fails closed on a lookup error (a friends-only view stays
empty rather than leaking) and drops a friend whose PID does not fit 32 bits rather than
truncating it into a different account. `needsFriends` keeps the nx-baas call off the searches
that do not need the graph.

## What is not here, and why

- **The Balloon extension (`0x73` methods 47-53).** The big one; the section above is the whole
  story. Capture-gated, and the top item for the next round.
- **Ranking (`0x70`).** Speculative upstream, no client observed. Three lines on evidence — and
  the likeliest place Balloon World's weekly standings turn out to live, if they are not in
  `SearchBalloon`.
- **Matchmaking / lobby / NAT traversal.** Odyssey has no live session at all.
- **A local blob host.** The earlier server stands one up in place of S3; OpenPak uses a real
  bucket through the library's presigner.
- **A monitoring dashboard and a presence reporter.** `/healthz` answers "is it up"; "who is
  online" belongs to the account side.

## Open questions — what the next round needs

`servers/nex`'s design doc lists the ones common to every NEX title (method 2 vs 6, the
`0x0B.1` vs `0x0B.4` register call, the Pia type-8 keepalive). These are Odyssey's own, in the
order a first live run would answer them:

1. **Does Odyssey reach us, and is `NEX_VERSION` right?** The access key is reference-verified
   only and the NEX version is a `4.0.0` guess. A wrong access key fails silently; a wrong
   version fails at `LoginEx`. **Evidence needed:** any packet at `22340`, then the RMC method
   ID and `apiVersionGeneral` of the first login.
2. **The Balloon extension wire shape.** Which of 47-53 Odyssey actually calls, in what order,
   and the exact `[Structure]` layout of each parameter and result — especially
   `FetchMyInfos`, `SearchBalloon`'s result bucketing, and `AddToBufferQueue`'s slot width.
   **Evidence needed:** the `0x73` method IDs and bodies from a session that reaches Balloon
   World, starting with the `FetchMyInfos` that fires first.
3. **Where do Balloon World's standings live** — Ranking (`0x70`) or `SearchBalloon` buckets?
   **Evidence needed:** whether any `0x70` call appears at all.

Until 1 is answered on hardware, "it starts against a real Postgres, serves `/healthz`,
registers Utility and a real base DataStore, and that DataStore round-trips objects" is all
this repository claims — and that much has been run, not assumed.
