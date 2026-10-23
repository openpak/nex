# Provenance

Where every non-obvious fact in this repository came from. Required by
[clean-room policy](https://docs.openpak.org/contributors/clean-room/): facts about a console and a
protocol are facts, and this file is what makes "we rewrote it from the facts" checkable.

Sources, referred to below by these names:

- **Kinnay** — [kinnay/NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki)
  and its `datastore_smm2` protocol documentation, public reverse-engineering.
- **Pretendo** — the AGPL-3.0 `nex-go`, `nex-protocols-go` and `nex-protocols-common-go`
  sources, which OpenPak links against and may therefore also read; and Pretendo's
  DataStore/SMM protocol docs.
- **MariOver** — TheGreatRambler's `MariOver`, a client that reads Nintendo's real SMM2
  servers; it names the fields of the SMM2 profile stat tables that Kinnay marks "Unknown".
- **Earlier research notes** — facts recorded in earlier work on this title, including a *real*, measured SMM2
  DataStore implementation (course-content and profile methods), so most of the DataStore
  facts below cite them. Facts only; no code, structure, names or comments were carried across.
- **ports.md** — OpenPak's own port plan.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `fdf6617f` | Earlier research notes. Configuration here, never a literal in source (`NEX_ACCESS_KEY`) |
| NEX version | `4.0.0` (`40000`) — **unverified** | Earlier research notes (`nexVersion = 40000`). Carried across as a starting value; the console reports `apiVersionGeneral` and the first capture settles it |
| Secure server PID | 2 | Earlier research notes. The conventional NEX value, and `servers/nex`'s default |
| Source key length | 32 bytes | Earlier research notes: a 32-byte key; a 16-byte key makes the console reject the ticket (the Switch kerberos key size). `NEX_SOURCE_KEY_BYTES=32` |

## Transport and endpoint shape

| Fact | Value | Source |
| --- | --- | --- |
| Transport | PRUDP over WebSocket, TLS | Earlier research notes: a working server listens with a certificate and key on both endpoints |
| Listener shape | **Two** listeners, authentication and secure on separate ports | Earlier research notes: a separate auth server (`:443`) and secure server (`:60007`); `NEX_STREAM_ID` = `NEX_SECURE_STREAM_ID` = 1 keeps them two listeners |
| Station scheme | `prudps` | Earlier research notes: the secure station URL uses `prudps`; measured, `prudp` makes the console complete the handshake but send an EMPTY CONNECT (no ticket), so the session is never authenticated and SMM2 dies at matchmaking. `prudps` makes it hand over the full ticket |
| Secure endpoint stream ID | 1 | Earlier research notes: the station URL carries `sid=1` |
| Station URL fields | `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | Earlier research notes. `stream=10` is the RVSecure stream type, which is what `servers/nex` sets |

### One unverified transport risk: secure PRUDP minor version 0

A working server forces `PrudpMinorVersion = 0` on the **secure** endpoint only, on a strong
measurement: the client offers minor version 6, and if the server answers with its
default 5 the client SKIPS its kerberos ticket (CONNECT payload length 0) and the session is
never authenticated — DataStore reads still work so the online menu looks fine, but SMM2 dies
the moment Pia needs the session key for matchmaking. Answering 0 makes the client send its
full 104-byte ticket. The auth endpoint must NOT be forced to 0 or it stops receiving logins.

This is a fact about that server's *own* NEX stack. Pretendo's `nex-go` negotiates the minor
version differently (it is documented elsewhere in the fleet to answer with the version the
client sent), so this may already be handled — or it may be a `servers/nex` change this title
needs. **Unverified against `nex-go`; flagged as the top hardware risk.** See `design.md`.

## Protocols the title speaks

The set registered on the secure endpoint by a working server, which is the observed set. IDs are
Kinnay's and match Pretendo's constants.

| Protocol | ID | Implemented here by |
| --- | --- | --- |
| Secure Connection | `0x0B` | `servers/nex` (it registers this itself) |
| Match Making | `0x15` | `nex-protocols-common-go` |
| Matchmake Extension | `0x6D` | `nex-protocols-common-go` |
| Match Making Ext | `0x32` | `nex-protocols-common-go` |
| NAT Traversal | `0x03` | `nex-protocols-common-go` |
| Ranking | `0x70` | `nex-protocols-common-go` over our Postgres store |
| Utility | `0x6E` | `nex-protocols-common-go` |
| DataStore (SMM2) | `0x73` | base object layer by `nex-protocols-common-go` over our Postgres + S3; SMM2 title-specific methods recorded below, not yet implemented |

Unlike a server that answers DataStore and Utility from a *captured replay* (a real
privacy leak, since it re-serves another player's measured data), this server answers the base
DataStore methods from real per-object Postgres state and presigned S3.

## DataStore 0x73 — the SMM2-specific method map

**SMM2's entire online mode is DataStore.** Course World is course upload, download, search,
comments, bookmarks, world maps, and per-maker profiles — all protocol `0x73`. The protocol
extends the *base* DataStore (the standard object methods, ≤ ~46) with a large block of
SMM2-specific methods (≥ 47). The base methods are what this server wires today; the
SMM2-specific methods are recorded here as the shape the shared object store must grow to
cover, and were sent as the design input for the shared object store.

### Base object methods SMM2 actually uses (wired here)

| Method | Name | SMM2 meaning | Source |
| --- | --- | --- | --- |
| 24 | `PreparePostObject` | begin a course upload: allocate a data id, return an upload URL/descriptor for the level BLOB | Earlier research notes |
| 25 | `PrepareGetObject` | begin a course download: return a URL for a stored course BLOB | Earlier research notes |
| 26 | `CompletePostObject` | mark the uploaded course ready (or drop it on failure) | Earlier research notes |
| 8 | `GetMeta` | course metadata; answers `NotFound 0x80690004` on a bare server | OpenPak/Kinnay |

`ResultCodes.DataStore.NotFound` = `0x00690004` (`0x80690004` with the error bit) — Kinnay's
`errors.py`.

### SMM2 title-specific methods (recorded, NOT yet implemented)

Method numbers and request/response shapes from earlier research notes (measured),
Kinnay's `datastore_smm2` docs, and Pretendo's SMM DataStore docs, as noted.

| Method | Name | Request → Response | Source |
| --- | --- | --- | --- |
| 47 | `RegisterUser` | create the SMM2 profile: `name` · `UnknownStruct1` (4×u16) · `Mii` (qbuffer) · `language` (u8) · `country` (string) · `deviceId` (string) → **empty** | Kinnay `RegisterUserParam.load`; earlier research notes (measured) |
| 48 | `GetUsers` | list of PIDs + `u32` option → list of `UserInfo` + one result code per requested PID, in order | Kinnay; earlier research notes (measured). **The one method the console demanded first, 4× at boot** |
| 49 | `SyncUserProfile` | (own) → `pid`·`name`·`UnknownStruct1`·`Mii`·`u8`·`country`·`u8`·`bool`·`bool` | Kinnay `SyncUserProfileResult.save`; earlier research notes (measured) |
| 60 | `CanPostCourse` | (none) → `Bool` (allowed) + `u32` (undocumented) | Pretendo SMM2 doc; earlier research notes (measured). **Order/types were the whole fix — a lone u32 desyncs the client** |
| 63 | `GetMiiClothes` | → (Mii clothing) | Kinnay/Pretendo |
| 70 | `GetCourses` | data ids → `courses[]` + `results[]` | Kinnay; earlier research notes (replay shape) |
| 71 | `GetCoursesPointRanking` | → `courses[]` + `ranks[]` + `bool` | Earlier research notes (replay shape) |
| 73 | `SearchCoursesLatest` | → `courses[]` + `bool` | Earlier research notes |
| 74 | `SearchCoursesPostedBy` | creator query → `CourseInfo[]` + `bool` | Kinnay; earlier research notes (measured) |
| 75 | `SearchCoursesPositiveRatedBy` | → `courses[]` | Earlier research notes |
| 76 | `SearchCoursesPlayedBy` | → `courses[]` | Earlier research notes |
| 79 | `SearchCoursesEndlessMode` | → `courses[]` | Earlier research notes |
| 80 | `SearchCoursesFirstClear` | → `courses[]` + `bool` | Earlier research notes |
| 81 | `SearchCoursesBestTime` | → `courses[]` + `bool` | Earlier research notes |
| 53/54/55 | `SearchUsers{Played,Cleared,PositiveRated}Course` | → `users[]` | Earlier research notes |
| 58/72/83/84 | Course World hub feeds (Hot / Popular / New) | → `courses[]` [+`ranks[]`] [+`bool`] | Earlier research notes (undocumented in Kinnay; shape deduced from captures) |
| 85/86/154 | `GetCoursesEvent` / `SearchCoursesEvent` / `GetEventCourseStatus` | event courses → `courses[]`+`results[]` / `courses[]` / (status) | Earlier research notes. 154 legitimately empty: no event running |
| 94/95 | `SearchCommentsInOrder` / `SearchComments` | → `comments[]` [+`bool`] | Earlier research notes |
| 96 | (game result report) | reports a play result | Earlier research notes (referenced by profile stat sourcing) |
| 66 | course level-data upload prep | → S3 presigned-POST descriptor for the level BLOB | Earlier research notes |
| 132 | relation-data upload prep | `dataId`·`type u32`·`size u32` → per-type S3 descriptor; **four types: 1 one-screen thumbnail, 2 entire thumbnail, 3 report thumbnail, 5 clear-check replay** | Earlier research notes (measured) |
| 160/162 | `GetWorldMap` / `SearchWorldMapPickUp` | super-world maps → `maps[]`+`results[]` / `maps[]` | Earlier research notes |
| 159–166 | super-world methods | create/read a maker's super world | Earlier research notes (referenced) |

### `UserInfo` — the SMM2 profile object (the heavy serialized type)

`GetUsers` (48) returns a list of `UserInfo`. It is **revision 3** (a lower revision makes the client stop reading where the server stops writing).
Field order, from Kinnay's `UserInfo.save` and MariOver's field names:

1. `pid` (u64)
2. `makerCode` (string) — the visible Maker ID; must be STABLE per player (base-30 of the PID)
3. `name` (string)
4. `UnknownStruct1` (nested struct: 4×u16, opaque)
5. `Mii` (qbuffer, opaque — stored, never interpreted)
6. `country` (string)
7. `region` (u8)
8. `lastActive` (DateTime — packed epoch, never 0; 0 is year-0 in the bitfield)
9. `Following`, `CommentsEnabled`, `TagsLocked` (3× bool — Kinnay calls them unk3–unk5; MariOver names them. Measured all-true: false made the game show "the creator restricted comments")
10. `play_stats` (map<u8,u32>): `0` plays, `1` clears, `2` attempts, `3` deaths (deaths stay 0 — not reported)
11. `maker_stats` (map): `0` hearts received, `1` maker points
12. `endless_challenge_high_scores` (map): easy, normal, expert, super-expert
13. `multiplayer_stats` (map, **non-contiguous keys**): `0` versus rating, `2` versus plays, `3` versus wins, `10` co-op plays, `11` co-op wins
14. `unk7` weekly maker points (map) · `badges` (list)
15. `unk8`: first-clears, world-records (map)
16. `unk9`: **`[0]` courses posted, `[1]` max allowed** — this is the field that drives the "32/32" upload screen
17. (revision 3 tail) `IsNintendoEmployee` (bool) · `LastUploadedLevel` (DateTime) · `Unk12` (bool) · `Unk13` (nested {bool, DateTime}) · `SuperWorldId` (string — **empty = length 0, not 1+NUL**) · `Unk15` (map: `0` super-worlds played) · `Unk16` (bool — MariOver sends true)

Maker code alphabet (base-30): `0123456789BCDFGHJKLMNPQRSTVWXY` (measured; the visible
code players read to each other, so it must be stable).

### Persistence-slot behaviour (the shape-defining fact for the shared store)

- SMM2 caps a maker at **32 published courses** ("32/32" on the save screen; the `unk9[1]`
  max in `UserInfo`). This is the natural persistence-slot ceiling: a player's own courses
  occupy per-owner slots, and the cap is 32.
- A base `PreparePostObject` (24) carries a `PersistenceInitParam` (slot id + `deleteLastObject`
  flag). Posting into an occupied slot replaces the previous occupant (and deletes it outright
  when the flag is set). This is exactly the base-object persistence the store implements, and
  the SMM2 course-upload flow is its heaviest real user.
- Course **bodies** (the level BLOB) are real S3 objects — unlike most fleet titles, SMM2
  genuinely uploads object bodies, so a bucket is not optional for a working course upload.

### PID vs NSA id — a cross-cutting fact

`GetUsers` (48) requests profiles by the caller's **NSA id** (the console account id, a
7–19-digit number), NOT the NEX PID. Earlier research measured the console asking for
`11389664994428258486` — the NSA the auth layer had just translated to a PID. A profile lookup
that searches by the raw value finds nothing and answers "you don't exist" to the very player
asking. The shared store must translate NSA → PID before a profile lookup. (In OpenPak this
translation is `nx-baas`/`servers/nex`'s identity map; the store needs a hook to reach it.)
This is the same "`pid` in a documented struct is not necessarily *our* pid" trap `userName =
NSA id` names elsewhere in the fleet.

## OpenPak's own facts

| Fact | Value | Source |
| --- | --- | --- |
| NEX port block | 22330: auth 22330, secure 22331, `/healthz` 22332 | ports.md, reserved for this repository |
| Account identity, friend graph | `Host.PlayerByPID` / `Host.FriendPIDs` over nx-baas `/internal/switch/identity` | `servers/nex` |
| Matchmaking, tracking and utility tables | created by `nex-protocols-common-go` when its managers are set | Pretendo source; this repository creates only the `datastore` and `ranking` schemas |
| Course object key base | `super-mario-maker-2` / `super-mario-maker-2-notify` | this repository's `datastore.go` |

| Game-server id / secure host | `g22306d00-lp1.s.n.srv.nintendo.net` (`NEX_SECURE_HOST`, set on production 2026-09-11) | Kinnay's Game Server List, Switch table: Super Mario Maker 2 | 22306D00 is the row marked Main server beside access key fdf6617f. The host shape `g<id>-lp1.s.n.srv.nintendo.net` follows the same shape as the other Switch NEX hosts |
