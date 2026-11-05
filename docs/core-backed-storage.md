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
a title never silently keeps local tables while the operator thinks it is core-backed. Only
`rankings` is built; `datastore` and `messaging` are reserved names and do nothing until the core
gaps below are closed.

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
| UploadScore (`InsertRankingByPIDAndRankingScoreData`) | `Submit(ref{wiiu, title, board=category}, account, sub 0, score, info, create_policy{order, best, cross_network false})` | info = pid, unique id, param, OrderBy, groups (score-bound: replaced only with a better score). `UpdateModeDeleteOld` = `Delete` then `Submit` (two calls, not atomic) | 1 Resolution (cached) + 1 (2 for delete-old) |
| UploadCommonData | title-storage `PutObject(collection ranking_common, name = unique id, owner = account, public)` | | 1 |
| GetCommonData(unique id) | `GetObject(owner "", ranking_common, name)` | missing = `Ranking::NotFound` (as the local MK8 store) | 1 |
| GetRanking mode Range | `Range(start = offset+1, limit = length)` + `ListObjects` for the page's common data | | 2 |
| mode Near | `ByAccounts([self])` -> position, then `Range` around it | | 3 |
| mode FriendRange / FriendNear | `FriendRanks(self, include_self)` (the account core's friend graph, the same one nn-friends serves) | ranks renumbered among friends | 2 |
| mode User | `ByAccounts([self])` | | 2 |
| `RankingOrderParam.OrderCalculation` | entry `rank` (1224) or `position` (1234) | | 0 |
| `RankingOrderParam.GroupIndex/GroupNum` | **gap L1**: no filter in leaderboards; the adapter pages the whole board (cap 10 000) and filters | correct, but O(board) per filtered read | up to 100 Range calls |
| `TimeScope` | ignored | as every local store here | |
| multiple unique ids per player per category | **gap L2**: `sub` is int32, the NEX unique id is u64; the adapter keeps one score per player per category (sub 0) | MK8 already stored one per player; the scoreboard titles stored one per unique id | |

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

### DataStore (0x73) -> title-storage: NOT BUILT, gaps

The common DataStore protocol leaves storage to ~15 callbacks (`InitializeObjectByPreparePostParam`,
`GetObjectInfoByDataID`, `UpdateObjectUploadCompletedByDataID`, `GetObjectInfosByDataStoreSearchParam`,
`RateObjectWithPassword`, `InitializeObjectRatingWithSlot`, `DeleteObjectByDataID`, ...). Titles
with a DataStore backend: Mario Kart 8, Mario Kart 7, Super Mario Maker, Pokken, Mario vs. DK,
MHGU, plus the Switch titles (served by switch-nex in production).

| NEX call | Core call | Gap |
| --- | --- | --- |
| PreparePostObject -> data id | `PutObject` (empty body, `meta_json.upload_completed=false`), `id` = data id | fits. **D1**: the console uploads the body with a presigned S3 URL; title-storage has no upload URL, its body is inline `bytea` capped by `max_object_bytes` (4 MiB default). Either the adapter copies the S3 object into `PutObject.body` on CompletePostObject (double storage, ~4 MiB cap, Super Smash Bros. replays and SMM courses fit, MK8 MKTV video does not) or title-storage grows a blob tier with presigned PUT/GET |
| GetMeta / GetObjectInfoByDataID | `GetObjectByID(network, viewer, id)` | fits (meta binary <= 1024 B in `info`, name, data type as `collection`, tags/period/flags/refer id in `meta_json`) |
| permission 0 public / 1 friends / 3 private | visibility public / friends / private | fits |
| permission 2 "specified recipients" (and 4, friends + recipients), delete permission recipients | **D2**: no recipient list in title-storage. Adapter-side check means storing them private and reading as the owner, which is a workaround the owner asked not to build silently |
| SearchObject (`DataStoreSearchParam`) | `ListObjects(owners, meta_filter_json, sort, offset, limit)` | **D3**: no range filters (created after/before, `data_type` ranges), no sort by rating, no "result order column" beyond updated/created/size/name/id; SMM's popular-course searches sort by rating |
| RateObject / GetRating / rating slots with lock, min/max, initial value | variables `owner = "v:<data id>"`, slot = rating slot, `AddVariable` | **D4**: virtual owner names are `[A-Za-z0-9_.-]{1,64}` so `v:<id>` works, but there is no per-rater lock (rate once / once a day) and no clamp; needs a `ratings` table keyed by object id (title-storage adapter guide already lists it as planned) |
| persistence slots (`GetObjectInfoByPersistenceTarget`) | `GetObject(owner, collection "persist", name = slot)` | fits |
| password-protected calls (`...WithPassword`) | `meta_json.access_password`, checked adapter-side | fits (the password is title data, not a credential of ours) |

Latency: every DataStore read is one gRPC hop (1-3 ms on the box) plus the Resolution cache.
Searches that today are one SQL query stay one `ListObjects` call where D3 does not apply.

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
| L1 | leaderboards | `RangeRequest`/`TotalRequest`: optional `bytes groups_filter` match (`int32 group_index`, `uint32 group_value`) against a new optional `Entry.groups bytes` set on `SubmitRequest.groups`; index on `(title, board, get_byte(groups, i))` not required at today's sizes |
| L2 | leaderboards | allow `int64 sub` or a `string sub_key` so one account can hold several NEX unique ids on one board; or accept "one score per player per category" as policy for NEX (recommendation: accept, it is UpdateModeNormal's meaning) |
| L3 | leaderboards | optional `SubmitRequest.aggregation_override` ("latest") so DeleteOld is one atomic call instead of Delete + Submit |
| D1 | title-storage | blob tier: `PrepareUpload(ref, owner, collection, name, size) -> {id, put_url, headers}`, `CompleteUpload(id)`, `GetObjectByID(...).get_url` (presigned, S3 behind it); body stays inline for small objects |
| D2 | title-storage | `Visibility VISIBILITY_RECIPIENTS` + `repeated string recipients` on `PutObjectRequest`/`Object`, readable by owner + recipients (+ friends for NEX permission 4 via a second flag) |
| D3 | title-storage | `ListObjectsRequest`: `int64 created_after_ms/created_before_ms`, `string sort = "rating:<slot>"`, `meta_range_json` (numeric `>=`/`<=` on meta keys) |
| D4 | title-storage | `ratings` sibling table: `RateObject(id, slot, rater, value, lock_policy, min, max) -> total, count`, `GetRatings(ids, slots)` |
| M1-M5 | chat | a "title message" kind: `SendTitleMessage(title, network, from, to_account or to_room, subject, body bytes, lifetime_s, flags, parent_id)`, `ListTitleMessages(title, account, range)`, `DeleteTitleMessages(ids)`; not friendship-gated (the console's own block list applies), not shown in the social chat UI |

Until those land, `datastore` and `messaging` stay local in every title, and the erasure
endpoint keeps covering them by rule (`docs/erasure.md`).

## Pilots

| Family | Title | Store | State |
| --- | --- | --- | --- |
| Wii U | Mario Kart 8 (`openpak-mario-kart-8`, db `mk8wiiu`) | rankings | `NEX_CORE_STORAGE=rankings` in production since v1.9.0 |
| Switch | (switch-nex) | rankings | already core-backed since switch-nex v0.12.0/v0.13.0 (card #63): MK8D, Splatoon 2, Mario Tennis Aces and every Ranking/Ranking2 title submit to leaderboards; no flag needed |
| Switch | Super Mario Maker 2 (switch-nex) | datastore | blocked on D1-D4 |

Mario Kart 8 quadlet (on the box, `~/.config/containers/systemd/openpak-mario-kart-8.container`;
the Wii U/3DS title quadlets are not tracked in any repository, a dated `.before-core-rankings`
copy is kept beside it):

```
Environment=NEX_CORE_STORAGE=rankings NEX_LEADERBOARDS_ADDR=openpak-leaderboards:20211 NEX_TITLE_STORAGE_ADDR=openpak-title-storage:20231
```

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
(the local tables were empty); nothing is lost, the title just stops seeing them.

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
