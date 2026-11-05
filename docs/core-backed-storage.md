# Core-backed storage (card openpak/prds#131, phase 8)

The NEX hub is becoming an adapter over the console-neutral core services
(`prds/repos/data-duplication-audit.md`, "The Pretendo-derived NEX game servers"). A title
container keeps only what its wire protocol needs (class P: `matchmaking.*`, `tracking.*`,
`utility.unique_ids`, `nexhost.login_names`); the per-game data with a cross-network meaning
(class G) moves to the core, one store and one title at a time, behind a flag.

Owner decisions (2026-11-05): rankings go to **leaderboards**, DataStore objects and ratings to
**title-storage**, messaging to **chat**. Per-title boards by default (`cross_network` false).
PIDs map to accounts through nn-account's Resolution service with a few minutes of cache.
Production held no ranking, DataStore, message or score rows in any of the 48 title databases
on 2026-11-05, so no data migration is needed; the code paths are real and tested.

## The flag

```
NEX_CORE_STORAGE=rankings[,datastore][,messaging]   # per title container, default empty (off)
```

| Variable | Meaning |
| --- | --- |
| `NEX_CORE_STORAGE` | comma list of the stores this title keeps in the core. Empty: the title's own tables, exactly as before |
| `NEX_LEADERBOARDS_ADDR` | leaderboards gRPC (`openpak-leaderboards:20211` on `openpak.network`) |
| `NEX_LEADERBOARDS_KEY` | its internal key; falls back to `LEADERBOARDS_INTERNAL_KEY` (already in the shared `.env`) |
| `NEX_TITLE_STORAGE_ADDR` | title-storage gRPC (`openpak-title-storage:20231`) |
| `NEX_TITLE_STORAGE_KEY` | falls back to `TITLE_STORAGE_INTERNAL_KEY` |
| `NEX_ACCOUNT_GRPC_ADDR`, `NEX_ACCOUNT_GRPC_KEY` | nn-account (PID <-> account), already set on every Wii U/3DS title |

A flag that is on with a setting missing **refuses to start** (`NEX_CORE_STORAGE=rankings: ...`):
a title never silently keeps local tables while the operator thinks it is core-backed. `rankings`
and `datastore` are built (`datastore` since nex v1.11.0, wired for Mario Kart 8 only); `messaging`
is a reserved name and does nothing until the chat gaps below are closed.

## Identity and cache

`internal/coreid`: `Resolution.ResolvePid(namespace, pid)` and `ResolveAccount(namespace,
account)` (namespace `wiiu` for Wii U titles, `3ds` for 3DS titles), wrapped in a cache:

- positive answers 5 minutes, both directions filled at once;
- "no link" 30 seconds, so a player who links mid-session is picked up quickly;
- errors are never cached; the cache resets past 50 000 entries.

A rank row carries its PID inside the entry's info blob, so reading a board needs no reverse
lookup; only rows written by something else ask `ResolveAccount`.

## Fallback rules

| Situation | Write (upload score, common data) | Read (rankings, common data) |
| --- | --- | --- |
| player has no account link yet | kept in `corestore.pending_scores` / `pending_common_data` in the title's own database | caller unknown to the core: empty list (no board position to show) |
| core unreachable, timeout, internal error | kept pending, one log line | NEX error to the console (`Core::SystemError`; common data `Ranking::NotFound`), never stale local data |
| core refuses (bad argument, too large) | NEX error to the console, nothing kept | n/a |
| board does not exist yet | created by the first Submit with the title policy | empty list |

`Drain` runs at start and every ten minutes and moves pending rows whose player now has a link
into the core, oldest first; a row is deleted only after the core has it. Nothing is dropped.
Every core call has a 3 s deadline.

## Store by store

### Rankings (Ranking protocol 0x70, common hooks) -> leaderboards: BUILT

`internal/coreranking`, installed by `Store.Install(commonProtocol)` in place of a title's local
hooks. One leaderboards title per NEX title: neutral key = native id = the `NEX_TITLE` name
(`mario-kart-8`), network `wiiu`/`3ds`, mapped in leaderboards and title-storage at start
(`MapTitle`, title-storage `UpsertTitle cross_network=false`).

| NEX call (hook) | Core call | Notes / gap | Latency |
| --- | --- | --- | --- |
| UploadScore (`InsertRankingByPIDAndRankingScoreData`) | `Submit(ref{wiiu, title, board=category}, account, sub 0, score, info, groups, create_policy{order, best, cross_network false})` | info = pid, unique id, param, OrderBy; `groups` = the entry's groups bytes (both score-bound: replaced only with a better score). `UpdateModeDeleteOld` = the same `Submit` with `aggregation_override: "latest"` (one atomic call; gap L3 BUILT, leaderboards v0.3.0) | 1 Resolution (cached) + 1 |
| UploadCommonData | title-storage `PutObject(collection ranking_common, name = unique id, owner = account, public)` | | 1 |
| GetCommonData(unique id) | `GetObject(owner "", ranking_common, name)` | missing = `Ranking::NotFound` (as the local MK8 store) | 1 |
| GetRanking mode Range | `Range(start = offset+1, limit = length)` + `ListObjects` for the page's common data | | 2 |
| mode Near | `ByAccounts([self])` -> position, then `Range` around it | | 3 |
| mode FriendRange / FriendNear | `FriendRanks(self, include_self)` (the account core's friend graph, the same one nn-friends serves) | ranks renumbered among friends | 2 |
| mode User | `ByAccounts([self])` | | 2 |
| `RankingOrderParam.OrderCalculation` | entry `rank` (1224) or `position` (1234) | | 0 |
| `RankingOrderParam.GroupIndex/GroupNum` | `groups_filter {group_index, group_value}` on `Range`/`ByAccounts`/`FriendRanks` (gap L1 BUILT, leaderboards v0.3.0); ranks are among the matching entries | the core filters, no board scan | same calls as an unfiltered read |
| `TimeScope` | ignored | as every local store here | |
| multiple unique ids per player per category | **gap L2, decided: policy, no schema change**: one score per player per category (sub 0), which is what `UpdateModeNormal` means; the core's `(account, sub)` key with an int32 `sub` could hold more, but a u64 unique id cannot fit it and no title needs it | MK8 already stored one per player | |

The rankings contract (`RANKINGS_ADDR`, read by the website) is served from the core when the
flag is on (`Store` implements `rankingsapi.Source`, rows carry `account_id` and `pid`), so the
website needs no change. Mario Kart 8's own read API (`MK8_API_ADDR`, the MKTV site) reads its
rankings, player standings and per-course counts from the core too; ghosts, highlights and
tournaments stay local until DataStore moves.

Erasure: a flipped title's scores are the core's (leaderboards and title-storage erase on
`account_deleted`). `erase.go` still reaches `corestore.pending_*` by its `owner_pid` rule
(`TestScrubDatabaseCorePendingRows`), and the old local tables are still scrubbed for unflipped
titles and for anything written before a flip.

Not moved with this flag: Mario Kart 8 tournaments and their scores (`mk8.tournament_scores`,
MatchmakeExtension 36-41 and Ranking 14-16). They are a board per (tournament, season) with team
scores and a 256-byte metadata blob; they map onto leaderboards (board `t<id>s<season>`, team as a
column) but need their own adapter. Splatoon's Splatfest scores (`ranking_splatoon.user_scores`)
likewise.

### DataStore (0x73) -> title-storage: BUILT (nex v1.11.0, `internal/coredatastore`; core gaps D1-D5 closed in title-storage v0.3.0-v0.7.0)

The common DataStore protocol leaves storage to ~15 callbacks (`InitializeObjectByPreparePostParam`,
`GetObjectInfoByDataID`, `UpdateObjectUploadCompletedByDataID`, `GetObjectInfosByDataStoreSearchParam`,
`RateObjectWithPassword`, `InitializeObjectRatingWithSlot`, `DeleteObjectByDataID`, ...). Titles
with a DataStore backend: Mario Kart 8, Mario Kart 7, Super Mario Maker, Pokken, Mario vs. DK,
MHGU, plus the Switch titles (served by switch-nex in production).

| NEX call | Core call | Gap |
| --- | --- | --- |
| PreparePostObject -> data id | `PutObject` (empty body, `meta_json.upload_completed=false`), `id` = data id | **D1 BUILT** (title-storage v0.3.0): PreparePostObject -> `PrepareUpload(ref, owner, writer, collection = data type, name, size, post_form = true)`: `id` is the data id, `put_url` + `form_fields` are the `DataStoreReqPostInfo` (an S3 POST policy pinned to the exact size, bucket `title-storage` on MinIO, public host `title-storage.openpak.org`); CompletePostObject -> `CompleteUpload(id, writer)` (size checked against the bucket; `FailedPrecondition` = the console's upload did not match); PrepareGetObject / GetMeta -> `GetObjectByID(...).get_url` (presigned GET, 15 min). Quota and `max_object_bytes` are checked at prepare (raise `max_object_bytes` per title with `UpsertTitle` for MK8 MKTV video); abandoned uploads expire after an hour; deletes and erasure remove the bytes |
| GetMeta / GetObjectInfoByDataID | `GetObjectByID(network, viewer, id)` | fits (meta binary <= 1024 B in `info`, name, data type as `collection`, tags/period/flags/refer id in `meta_json`) |
| permission 0 public / 1 friends / 3 private | visibility public / friends / private | fits |
| permission 2 "specified recipients" (and 4, friends + recipients), delete permission recipients | `PutObject` / `PrepareUpload` / `UpdateObjectMeta` with `visibility = VISIBILITY_RECIPIENTS`, `recipients` = `RecipientIDs` PIDs mapped to accounts, `recipients_include_friends` for 4; `ListObjects(all_owners, received=true)` | **D2 BUILT** (title-storage v0.6.0, card #135): readable by owner + recipients (+ the owner's friends with the flag) in every get, list, rate and rating read; for permissions 0/1/3 the list is stored and returned but grants nothing (the meta echoes `RecipientIDs`); `SearchTypeReceived` = `received`; erasure takes an account out of every list (`recipient_of`). `DelPermission` and its recipients stay in `meta_json`, checked adapter-side before `DeleteObjects` |
| ChangeMeta (`UpdateObjectMetaBinary`, `UpdateObjectPeriod`, `UpdateObjectDataType`, permission changes) | `UpdateObjectMeta(network, id, writer, info?, meta_json?, meta_merge_json, name?, collection?, visibility + recipients, expires_unix_ms?, condition)` | **D5 BUILT** (title-storage v0.7.0, card #146): metadata only, the body (inline or blob) and the ratings stay; meta binary = `info`, tags/period/flags = `meta_merge_json`, data type = `collection`, period = `expires_unix_ms`; `condition.expect_updated_unix_ms` is the optimistic version check. `PutObject` stays a full replacement that drops an uploaded blob: never use it for ChangeMeta |
| SearchObject (`DataStoreSearchParam`) | `ListObjects(all_owners / friends_of_viewer / owners, exclude_owners, collections, meta_filter_json, meta_range_json, created/updated bounds, sort, min_rating_count, include_ratings, offset, limit)` | **D3 BUILT** (title-storage v0.4.0, rating sorts v0.5.0): `OwnerType` anybody / friends / anybody-except = `all_owners` / `friends_of_viewer` / `all_owners` + `exclude_owners`; `DataType(s)` = `collection(s)`; `Tags` = `meta_filter_json {"tags":[...]}`; `CreatedAfter/Before`, `UpdatedAfter/Before` = the inclusive Unix-ms bounds; numeric meta ranges with `meta_range_json {"key":{"ge":n,"le":m}}`; `ResultOrderColumn` data id / size / name / created / updated = `sort`, `Rating0-15` = `rating:<slot>`, `RatingAverage0-15` = `rating_avg:<slot>`, `MinimalRatingFrequency` = `min_rating_count`. Not expressible: sort by data type (collection) and by referred count (MK8 never increments it, so `id` order is the same answer) |
| RateObject / GetRating / rating slots with lock, min/max, initial value | `InitRating(id, writer, slot, policy)`, `RateObject(id, slot, rater, value)`, `GetRatings(ids, slots, rater)`, `ResetRatings`, `include_ratings` on `GetObjectByID` / `ListObjects` | **D4 BUILT** (title-storage v0.5.0): `rating_slots` + one `ratings` row per (object, slot, rater account); policy = `DataStoreRatingInitParam` (initial value, `RatingLockType` none/interval/period/permanent with `PeriodHour`/`PeriodDuration`, range only when `InternalFlag` UseRangeMin/Max, `Flag` modifiable / round-minus / disable-self-rating); `locked=true` = `DataStore::OperationNotAllowed`; deleting an object deletes its ratings; erasure removes an account's ratings (totals adjusted) and those on its objects. Mapping table: title-storage `docs/adapter-guide.md` |
| persistence slots (`GetObjectInfoByPersistenceTarget`) | `GetObject(owner, collection "persist", name = slot)` | fits |
| password-protected calls (`...WithPassword`) | `meta_json.access_password`, checked adapter-side | fits (the password is title data, not a credential of ours) |

Latency: every DataStore read is one gRPC hop (1-3 ms on the box) plus the Resolution cache.
Searches that today are one SQL query stay one `ListObjects` call where D3 does not apply.

#### The adapter as built (nex v1.11.0)

`internal/coredatastore` registers the DataStore method handlers itself (`Store.Install` on the
title's `datastore.Protocol`: GetMeta, GetMetas, GetMetasMultipleParam, SearchObject,
PrepareGetObject, PreparePostObject, CompletePostObject(s), PostMetaBinary, ChangeMeta,
DeleteObject, RateObject(s)) instead of filling nex-protocols-common-go's callbacks: those
callbacks do not carry the caller, and every core read needs the caller's account as the viewer.
Visibility is then the core's: there is no second permission check in the adapter for reads.
Where it differs from the table above, deliberately:

| Point | As built | Why |
| --- | --- | --- |
| object name | the core name is the id (an empty name at prepare); the NEX `Name` is `meta_json.name` | NEX names repeat within an owner (every ghost is "ghost"); a core name is unique per owner and collection and would replace |
| collection | the data type in decimal (`"17"`); `meta_json.data_type` too | every DataStore search adds `meta_range_json {"data_type":{}}`, which leaves the title's other objects (`ranking_common`) out |
| persistence slot | `meta_json.persistence_slot`, always present (65535 = none); a new object in a slot deletes the owner's previous one, as the local stores did | lets the read API ask for "no slot" (highlights) with `meta_filter_json` |
| period | kept in `meta_json.period` and returned as `ExpireTime`; **no core expiry** is set | the local store never expired objects, and NEX periods are refreshed on reference, which nothing here does: a core expiry would silently delete players' ghosts after 90 days |
| rating init | the post's `RatingInitParams` ride in `meta_json.rating_init` and become `InitRating` at `CompleteUpload` (a pending upload holds no slots); `RateObject` sends the slot's policy too, so a slot is never missing | |
| owner PID | `meta_json.owner_pid`, so a read needs no reverse lookup | as rankings' info blob |
| `DelPermission` | checked adapter-side (owner, public, friends from nn-friends, recipients) before `UpdateObjectMeta` / `DeleteObjects`; the core's write rule applies on top | |
| sort by name / data type / referred count | `id` | the core name is the id; data type and referred count cannot be sorted (MK8 never increments the count) |
| `SearchTypeOwnPending/Rejected` | the owner's objects, no status filter | as the local store |
| no account link | reads are anonymous (public objects only); writes and ratings are `DataStore::PermissionDenied` | no pending rows: a DataStore write is answered with a data id or URL the console uses at once |
| core down | `Core::SystemError`, nothing kept locally | |

Errors: `NotFound` -> `DataStore::NotFound`, `InvalidArgument`/`AlreadyExists` ->
`InvalidArgument`, `PermissionDenied` -> `PermissionDenied` (a self-rating -> `OperationNotAllowed`),
`ResourceExhausted` -> `OverCapacity`, `FailedPrecondition` (upload size mismatch) ->
`OperationNotAllowed`, a locked rating -> `OperationNotAllowed`, anything else -> `Core::SystemError`.

#### Wii U reachability of the bodies (decided 2026-11-05: direct, no proxy, no new redirect)

The console uploads to `put_url` (an S3 POST policy) and downloads from `get_url`, both signed by
title-storage for `TITLE_STORAGE_S3_PUBLIC_URL`, which in production is
**`http://title-storage.openpak.org`** (path style, `/title-storage/<id>/<random>`). On the box,
Traefik's `web` entrypoint (port 80) routes `Host(title-storage.openpak.org) &&
PathPrefix(/title-storage/)` to `openpak-minio:9000` (router `title-storage-ds-web`, the same
pattern as `mk8d-ds.openpak.org` for the Switch's MK8D DataStore). Public DNS answers the name
with the box address.

| Console path | Resolves to | TLS |
| --- | --- | --- |
| Aroma + Inkay-openpak | the PPC DNS hook answers every name under the bundled family `.openpak.org` with `OPENPAK_SERVER_IP` (`common/redirect_list.cpp`, `redirect_bundled()`); the signed list only adds families | none: plain HTTP on port 80 |
| stock console on nn-sssl-dns | `*.openpak.org` is answered with the box | none |

So no entry in the signed redirect list is needed and no console-side change: the name is already
inside the compiled-in `.openpak.org` family, and plain HTTP avoids the certificate question
(the Wii U's CA store would not trust the public certificate; Inkay's NoSSL would). A proxy through
an existing title host was not needed: Mario Kart 8 has no console-facing HTTP host (NEX is
UDP), and any host the server could proxy through would be another `*.openpak.org` name with the
same reachability. Probed 2026-11-05: `http://title-storage.openpak.org/title-storage/` from
outside answers MinIO's 403 (the route is live). Not yet proven: a retail Wii U accepting an
`http://` DataStore URL (Pretendo hands out HTTPS); if a console refuses it, an HTTPS router for
the same name on 443 works for Inkay consoles (NoSSL) without any other change.

#### Mario Kart TV (`mk8.mktv_consent`, `mk8.mktv_videos`): stay local

Both are the title's own feature state, class 1: a player's opt-in to the OpenPak YouTube channel
and the worker's per-highlight upload status and YouTube id. They are not DataStore objects, have
no cross-network meaning and are read only by this title, so they stay in `mk8wiiu`. Erasure
already covers them by the PID-column rule (`docs/erasure.md`: "MKTV consent and videos"). What
moved is their reads of DataStore objects: with the flag on, the YouTube worker finds a
highlight's owner with `Store.Owner` (a public object in the core; a private or missing one is
`unknown_object`), and the read API (`MK8_API_ADDR`) lists highlights and ghosts, the per-course
ghost counts, thumbnails (the oldest public object referring to a highlight), ratings and bodies
(through `get_url`) from title-storage as an anonymous viewer, joining only the YouTube id from
`mk8.mktv_videos`.

### Messaging (0x17) and MessageDelivery (0x1B) -> chat: NOT BUILT, gaps

Titles: Animal Crossing: New Leaf, the Monster Hunter family (3U, 4, Stories, XX, GU), Pokemon
X/Y/ORAS, Ice Station Z, RE: The Mercenaries 3D, Yo-kai Watch 2 and Blasters, Super Mario Maker,
Super Mario Bros. 35.

| NEX need | chat today | Gap |
| --- | --- | --- |
| recipient type 1 (a PID) | `SendMessage(from, to)` | fits, but **M1**: chat refuses non-friends; NEX titles message any player met in a gathering |
| recipient type 2 (a gathering) | none | **M2**: no group/room recipient |
| per-title inbox (`GetMessagesHeaders`, `GetNumberOfMessages`, `RetrieveAllMessagesWithinRange`) | one global conversation per pair, text DMs shown in the website/app chat | **M3**: no title scope; NEX game messages would appear in the player's real chat |
| `DeleteMessages(ids)`, `DeleteAllMessages` | none | **M4**: no delete |
| binary bodies, subject, sender name, lifetime (expiry), flags, parent id | `kind` + string `body` | **M5**: no binary kind, no expiry, no subject/flags/thread |
| instant (delivered, not stored) messages | n/a | stays in the adapter (class P) |

## Gap list and proposed core API changes

| Id | Core | Proposed change (additive unless noted) |
| --- | --- | --- |
| L1 **BUILT** (v0.3.0, nex v1.10.0) | leaderboards | `RangeRequest`/`TotalRequest`: optional `bytes groups_filter` match (`int32 group_index`, `uint32 group_value`) against a new optional `Entry.groups bytes` set on `SubmitRequest.groups`; index on `(title, board, get_byte(groups, i))` not required at today's sizes |
| L2 **DECIDED: policy** (nex v1.10.0) | leaderboards | allow `int64 sub` or a `string sub_key` so one account can hold several NEX unique ids on one board; or accept "one score per player per category" as policy for NEX (recommendation: accept, it is UpdateModeNormal's meaning) |
| L3 **BUILT** (v0.3.0, nex v1.10.0) | leaderboards | optional `SubmitRequest.aggregation_override` ("latest") so DeleteOld is one atomic call instead of Delete + Submit |
| D1 **BUILT** (title-storage v0.3.0, card #134) | title-storage | blob tier: `PrepareUpload(ref, owner, writer, collection, name, size, sha256, ..., post_form) -> {id, method, put_url, headers, form_fields, expires_unix_ms}`, `CompleteUpload(id, writer)`, `get_url` on every read of a blob-backed object (presigned, MinIO bucket `title-storage` behind it); body stays inline for small objects |
| D2 **BUILT** (title-storage v0.6.0, card #135) | title-storage | `Visibility VISIBILITY_RECIPIENTS` + `repeated string recipients` + `bool recipients_include_friends` on `PutObjectRequest`/`PrepareUploadRequest`/`Object`, readable by owner + recipients (+ the owner's friends with the flag, NEX permission 4); `ListObjectsRequest.received`; erasure removes the account from every list |
| D3 **BUILT** (title-storage v0.4.0, card #136) | title-storage | `ListObjectsRequest`: `all_owners`, `exclude_owners`, `collections`, `created_after_ms/created_before_ms`, `updated_after_ms/updated_before_ms`, `meta_range_json` (numeric `ge`/`le` on meta keys); `sort = "rating:<slot>"` / `"rating_avg:<slot>"` and `min_rating_count` with D4 |
| D4 **BUILT** (title-storage v0.5.0, card #137) | title-storage | `rating_slots` + `ratings` sibling tables: `InitRating(id, writer, slot, policy)`, `RateObject(id, slot, rater, value, policy) -> total, count, locked`, `GetRatings(ids, slots, rater)`, `ResetRatings(id, slots)`; erasure covers rater and owner sides |
| D5 **BUILT** (title-storage v0.7.0, card #146) | title-storage | `UpdateObjectMeta(network, id, writer, setter, info?, meta_json?, meta_merge_json, name?, collection?, visibility + recipients, expires_unix_ms?, condition)`: metadata only, keeps the body (inline or blob) and the ratings; changing the data type moves the object to another collection. `PutObject` is unchanged (a replacement; over a blob-backed object it drops the blob) |
| M1-M5 | chat | a "title message" kind: `SendTitleMessage(title, network, from, to_account or to_room, subject, body bytes, lifetime_s, flags, parent_id)`, `ListTitleMessages(title, account, range)`, `DeleteTitleMessages(ids)`; not friendship-gated (the console's own block list applies), not shown in the social chat UI |

Until a title is flipped, its DataStore stays local, and `messaging` stays local everywhere until
M1-M5 land; the erasure endpoint keeps covering local tables by rule (`docs/erasure.md`).

## Pilots

| Family | Title | Store | State |
| --- | --- | --- | --- |
| Wii U | Mario Kart 8 (`openpak-mario-kart-8`, db `mk8wiiu`) | rankings | `NEX_CORE_STORAGE=rankings` in production since v1.9.0 |
| Switch | (switch-nex) | rankings | already core-backed since switch-nex v0.12.0/v0.13.0 (card #63): MK8D, Splatoon 2, Mario Tennis Aces and every Ranking/Ranking2 title submit to leaderboards; no flag needed |
| Wii U | Mario Kart 8 | datastore | `NEX_CORE_STORAGE=rankings,datastore` (nex v1.11.0, card openpak/prds#147) |
| Switch | Super Mario Maker 2 (switch-nex) | datastore | core side ready (D1-D5 built); waits on a switch-nex adapter |

**DataStore after D5 (2026-11-05): every core gap is closed** (D1-D5 live in title-storage v0.7.0) and the nex adapter is built (v1.11.0); another title moves with a wiring release (its DataStore registration takes `NEX_CORE_STORAGE=datastore` as MK8's does) and an env flip. A title's DataStore hooks
share one object store, so a title cannot keep searches or ratings over local tables while its
objects live in the core; the whole hook set moves at once. What each DataStore title registers
(title code under `titles/`), and therefore what it waits on:

| Title | Hooks beyond upload/get/delete | Waits on |
| --- | --- | --- |
| Mario Kart 8 (`mk8wiiu`) | `GetObjectInfosByDataStoreSearchParam` (MKTV browsing, `searchQuery`), `RateObjectWithPassword` / `InitializeObjectRatingWithSlot` (`attachRatings` on every search and get), recipient lists (`RecipientIDs`) | the adapter (D1-D5 built) |
| Mario Kart 7, Mario vs. Donkey Kong: Tipping Stars, Monster Hunter Generations Ultimate | the same common hook set: search, ratings, recipients | the adapter (D1-D5 built) |
| Pokken Tournament | search, ratings, `verify_object_permission` with recipients | the adapter (D1-D5 built) |
| Super Mario Maker (Wii U) | course queries sorted by stars/plays (`course_query.go`), ratings in 15 files, recipients | the adapter (D1-D5 built) |
| Super Smash Bros. for Wii U / 3DS replays | title protocol: `PreparePostReplay` presigns into S3, but the replay list is in memory and `GetNextReplay` returns a fixed fake replay | not a pilot: the replay server is unimplemented upstream; once written it needs D1 only (plus a `ListObjects` pick) |
| Super Mario Maker 2 (switch-nex) | everything above | the adapter (D1-D5 built) |

D2 (recipients) turns out to be needed by every common-DataStore title, not only by "specified
people" uploads: each title stores `Permission.RecipientIDs` and `DelPermission.RecipientIDs` and
returns them in every meta (Pokken also enforces them).

**DataStore pilot = Mario Kart 8 (Wii U), flipped 2026-11-05 (nex v1.11.0, card openpak/prds#147).**
What the pilot needed, in order:

1. ~~**D2 recipients in title-storage**~~ **BUILT** (v0.6.0, card #135): `Permission.RecipientIDs`
   = `recipients` (+ `recipients_include_friends` for permission 4), `SearchTypeReceived` =
   `received`; `DelPermission.RecipientIDs` stays in `meta_json` and the adapter checks it before
   `DeleteObjects`. `SendSpecified*` searches are the owner's own objects (`owners = [self]`).
2. ~~**D5 metadata-only update in title-storage**~~ **BUILT** (v0.7.0, card #146): MK8's
   `UpdateObjectMetaBinaryByDataIDWithPassword`, `UpdateObjectPeriodByDataIDWithPassword` and
   `UpdateObjectDataTypeByDataIDWithPassword` are `UpdateObjectMeta` (`info` / `expires_unix_ms`
   and `meta_merge_json` / `collection`), which keeps the uploaded ghost or MKTV video and its
   ratings.
3. ~~**The nex adapter**~~ **BUILT** (v1.11.0; "The adapter as built" above) (`internal/coredatastore`, the `datastore` flag): the whole MK8 hook set
   from `titles/mario-kart-8/datastore.go` and `search.go` onto the calls in the table above
   (PID <-> account through the Resolution cache; collection = data type; `meta_json` carries
   period, flag, tags, refer data id, persistence slot and the PIDs the wire needs; `info` = meta
   binary; `PrepareUpload(post_form)` replaces the title's own S3 presign; ratings through
   `InitRating` / `RateObject` and `include_ratings` on every get and search, which is MK8's
   `attachRatings`). `SearchTypePublicExcludeOwnAndFriends` is `all_owners` + `exclude_owners`
   = self + friends (accounts from the friend graph). Fallback rules as rankings: no account link
   or core down = a NEX error, never stale local data.
4. ~~**Console reachability of the blob host**~~ **DONE**: `http://title-storage.openpak.org` is
   inside the bundled `.openpak.org` family and nn-sssl-dns's `*.openpak.org`; no signed-list entry
   and no proxy needed ("Wii U reachability of the bodies" above).
5. ~~**MK8's read API**~~ **DONE**: ghosts, highlights, thumbnails, ghost counts and bodies from
   the core; MKTV consent/videos stay local ("Mario Kart TV" above).
6. ~~**Before the flip**~~ **DONE** (recount 2026-11-05, before the flip): `mk8.datastore_objects` 0,
   `mk8.datastore_ratings` 0, `mk8.mktv_consent` 0, `mk8.mktv_videos` 0, `corestore.pending_*` 0;
   title-storage `mario-kart-8` held 1 object (`ranking_common`, the rankings pilot's row) and 0
   DataStore objects. Nothing to move (a DataStore object would need its body too: MK8 had no
   bucket configured, `MK8_S3_*` unset, so no body ever existed). The empty local tables stay and
   are no longer read.

Mario Kart 8 quadlet (on the box, `~/.config/containers/systemd/openpak-mario-kart-8.container`;
the Wii U/3DS title quadlets are not tracked in any repository, dated `.before-core-rankings` and
`.before-core-datastore-20261105` copies are kept beside it):

```
Environment=NEX_CORE_STORAGE=rankings,datastore NEX_LEADERBOARDS_ADDR=openpak-leaderboards:20211 NEX_TITLE_STORAGE_ADDR=openpak-title-storage:20231
```

`datastore` was added 2026-11-05 (nex v1.11.0). At start the title sets its title-storage
`max_object_bytes` to 8 MiB (the read API's body ceiling).

Local rows written before a flip are not read any more, so move them first: copy them into
`corestore.pending_*` (same columns) and the start-up drain files them in the core. For Mario
Kart 8 on 2026-11-05 that was 0 scores and 1 common-data row:

```sql
INSERT INTO corestore.pending_common_data (owner_pid, unique_id, data)
  SELECT pid, unique_id, data FROM mk8.ranking_common_data ON CONFLICT DO NOTHING;
INSERT INTO corestore.pending_scores (owner_pid, unique_id, category, score, order_by, update_mode, groups, param)
  SELECT pid, unique_id, category, score, order_by, 0, coalesce(groups, ''), param FROM mk8.ranking_scores;
```

Rollback: remove that line and restart the unit. Scores written while flipped stay in the core
(the local tables were empty); nothing is lost, the title just stops seeing them. DataStore alone:
take the word `datastore` out of `NEX_CORE_STORAGE` (keep `rankings`) and restart
`openpak-mario-kart-8`; objects, bodies and ratings written while flipped stay in title-storage and
the bucket, and the title goes back to its (empty) local tables.

## Rollout order (rankings, after the pilot has been green for a day)

Env-only flips (the title already uses `coreranking` after its wiring release):

1. `mario-kart-8` (pilot, done)

Need a one-line wiring release first (their Ranking hooks come from `internal/scoreboard`; one
change makes `scoreboard.Install` take a `nexhost.CoreTitle` and pick the core when the flag is
on), then each is an env change, in this order (Wii U before 3DS; titles with known players
first):

2. `super-smash-bros-wiiu` (wiiu)
3. `pokken-tournament` (wiiu)
4. `puyo-puyo-20th` (3ds)
5. `tetris-axis` (3ds)
6. `super-smash-bros-3ds` (3ds)
7. `monster-hunter-4`, 8. `monster-hunter-xx`, 9. `monster-hunter-stories` (3ds)
10. `yo-kai-watch-2` (3ds), 11. `ice-station-z` (3ds), 12. `resident-evil-the-mercenaries-3d` (3ds)

Need their own adapter (their Ranking hooks are title code): `puyo-puyo-tetris`,
`sonic-all-stars-racing-transformed-wiiu`, `terraria` (wiiu), `dr-luigi`, `miracle-cure`,
`pikmin-3`, Splatoon fest scores, Mario Kart 8 tournaments, Super Mario Bros. 35 (Ranking2).
