# Changelog — nex

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

Note: the early history below is the upstream project (Pretendo nex-go and title servers);
OpenPak work starts at the port/fork commit.

## v1.11.0 — 2026-11-05
- **DataStore can live in the core** (card openpak/prds#147, #131 phase 8): `NEX_CORE_STORAGE=datastore` (default off) is built. `internal/coredatastore`
  serves the DataStore methods (GetMeta(s), GetMetasMultipleParam, SearchObject, PrepareGet/PreparePost/CompletePost(s), PostMetaBinary, ChangeMeta,
  DeleteObject, RateObject(s)) over title-storage v0.7.0: object id = data id, owner = the PID's account, collection = data type, meta binary = info, the
  rest of the meta (name, permissions and recipient PIDs, period, flag, status, tags, refer id, persistence slot, owner PID) in `meta_json`; permissions
  are core visibility (public / friends / recipients / private / recipients + friends); bodies through the blob tier (S3 POST policy and presigned GET
  on `http://title-storage.openpak.org`); ratings through `InitRating`/`RateObject` and `include_ratings`; ChangeMeta through `UpdateObjectMeta` (body
  and ratings kept); SearchObject through `ListObjects` (scope, exclusions, data types, tags, refer id, time bounds, rating sorts). No DataStore row is
  kept locally; no account link = anonymous reads and refused writes; core down = `Core::SystemError`. Period is kept as metadata, never a core expiry.
  `NewCoreDataStore` refuses to start without `NEX_TITLE_STORAGE_ADDR`/key or nn-account. Design and deviations: docs/core-backed-storage.md.
- **Mario Kart 8 wiring:** with the flag on, its DataStore is the core adapter, its read API (`MK8_API_ADDR`) reads highlights, ghosts, ghost counts,
  thumbnails, ratings and bodies from title-storage, and the MKTV YouTube worker looks up a highlight's owner there. `mk8.mktv_consent` and
  `mk8.mktv_videos` stay local (the feature's own consent and upload state, erased by PID). The title's object cap in title-storage is 8 MiB.
  The read API's Rankings/Courses work with either flag alone.
- `internal/titlestoragepb` regenerated from title-storage v0.7.0.
- Tests: a fake title-storage with visibility, blob tier (a real HTTP bucket the test uploads to and downloads from), ratings and metadata updates;
  the full upload/download/rate/change-meta/search/delete round trip, every permission, no link and core down, upload mismatch, persistence slots,
  meta-only posts, update permission, every search translation and the read API; `TestLiveRoundTrip` runs the same round trip against a real
  title-storage when `NEX_LIVE_TITLE_STORAGE_ADDR` is set.

## v1.10.0 — 2026-11-05
- `internal/coreranking` uses the leaderboards v0.3.0 API (card openpak/prds#131, gaps L1-L3) and drops its workarounds: group bytes are stored in the entry's
  `groups` and the Ranking group filter is the core's `groups_filter` (no more reading up to 10 000 rows and filtering in the adapter); `UpdateModeDeleteOld`
  is one `Submit` with `aggregation_override: "latest"` instead of `Delete` + `Submit`; the info blob no longer carries groups (rows written by v1.9.0 are still
  read through their info tail). Gap L2 is decided as policy: one score per player per category. Flag semantics and the `corestore.pending_*` drain are
  unchanged. Needs leaderboards >= v0.3.0.

## v1.9.0 — 2026-11-05
- **Rankings can live in the core** (card openpak/prds#131, phase 8): per-title flag `NEX_CORE_STORAGE=rankings` (default off). The Ranking protocol's
  eight storage hooks are served by `internal/coreranking`: scores in the leaderboards service (board = category, one score per player per category,
  best kept, `UpdateModeDeleteOld` replaces; pid, unique id, param and groups ride in the entry's info), common data in title-storage (collection
  `ranking_common`, public). Per-network titles (`cross_network` false), registered in both services at start. PIDs map to accounts through
  nn-account's Resolution service (`internal/coreid`, 5 min cache, 30 s for "no link"). A score the core cannot take yet (no link, core down) waits in
  `corestore.pending_*` in the title's database and is moved every ten minutes; nothing is dropped. A flag that is on with a setting missing refuses
  to start. New settings: `NEX_LEADERBOARDS_ADDR`, `NEX_TITLE_STORAGE_ADDR` (keys default to the shared `LEADERBOARDS_INTERNAL_KEY` /
  `TITLE_STORAGE_INTERNAL_KEY`). Design, gaps and rollout order: [docs/core-backed-storage.md](docs/core-backed-storage.md).
- **Mario Kart 8 is the pilot:** with the flag on, its time trials, the rankings contract (`RANKINGS_ADDR`) and the MKTV read API's rankings,
  player standings and course counts come from the core. Tournaments, ghosts and highlights stay local.
- DataStore and messaging are not moved: title-storage and chat lack recipient lists, presigned bodies, rating locks, search ranges, group and
  per-title messages (gap list in the design note).
- Erasure reaches the pending tables by its PID-column rule (`TestScrubDatabaseCorePendingRows`).
- Tests: fake leaderboards/title-storage for every ranking mode, group filter, 1224/1234 order, delete-old, common data join, contract source, empty
  and unknown boards; Postgres-backed pending rows that drain once linked (`NEX_TEST_DSN`); cache TTLs.

## v1.8.4 — 2026-11-05
- **Pikmin 3 histogram job no longer fails on a registered task:** nn-boss answers a duplicate RegisterTask with InvalidArgument and an `ALREADY_EXISTS:` message instead of codes.AlreadyExists, so every hourly run exited 1 after the first. Both forms now count as success. Test: `TestAlreadyRegistered`.

## v1.8.3 — 2026-11-05
- **Account erasure no longer fails in production** (openpak/prds#120): the erase route scanned every `*_DATABASE_URL` in the environment, and the production containers load the shared deploy `.env` with every other service's URL (account, website, demonware on loopback, no `sslmode`). The first such URL made every title answer 502 ("SSL is not enabled on the server" or a refused connection on 127.0.0.1:5432), found by the end-to-end erasure test. A title now erases only from its own database: the configured URL, else `NEX_DATABASE_URL`, else `DATABASE_URL`; the environment scan is only the last resort. Test: `TestOwnDatabaseURLs`.

## v1.8.2 — 2026-11-04
- **Fix Wii U error 106-0502 (Mario Kart 8) and the same bug in Mario vs. Donkey Kong: Tipping Stars:** the login response named the secure
  server with `prudp:` instead of `prudps:`. The console still connected to the secure port but sent an empty CONNECT with no Kerberos
  ticket, which the server rejects. Both now hand out `prudps:` like every other Wii U/3DS title. Regression test per title.

## v1.8.1 — 2026-11-04
- **Permanent state** (openpak/prds#85): Super Mario Bros. 35 Ranking2 common data (name, Mii, binary data per PID and NEX unique id) moved from
  memory to Postgres (`smb35.common_data`, created on start; the `pid` column is what account erasure's schema scan deletes by). Test:
  `SMB35_TEST_DATABASE_URL`.

## v1.8.0 — 2026-11-04
- **Account erasure** (card openpak/prds#120): `DELETE /internal/accounts/{account_id}/data` with `Authorization: Bearer $ACCOUNT_ERASE_KEY`, served by every title
  container on its `/healthz` listener (`NEX_HTTP_ADDR`, base+2, container network only). Env unset: 503; wrong key 401; not a UUID 400; PIDs unresolvable 502.
  Answers `{"service":"nex-<title>","rows":N}` and logs one line `account erased`. Idempotent. The website must call every title container (one container is one title with its own database).
- The account's PIDs come from the account core (`NEX_CORE_ADDR` + `NEX_CORE_KEY`; links in the wiiu, 3ds and switch namespaces) and, when `NEX_ACCOUNT_GRPC_ADDR/_KEY` are set,
  from nn-account's resolution service (a Wii U/3DS id that was never linked). `internal/resolutionpb` is the vendored contract (same copy as nn-friends).
- The databases are scrubbed by schema, not by a title list (every Postgres URL in the container: `NEX_DATABASE_URL`, `<TITLE>_DATABASE_URL`, `PN_*_POSTGRES_URI`, `DATABASE_URL`): every table with a PID column
  (`pid`, `*_pid`, `owner`, `principal_id`, `recipient_id`) loses the account's rows, children first by foreign key; PIDs leave participant/recipient arrays; objects removed take their ratings/records
  (everything in the schema keyed by `data_id`) and their bodies in the bucket (`<key base>/<data id>.bin`, buckets found from the `*_S3_*` variables) with them.
  Kept and credited to nobody (PID 0): `mk8.tournaments` created by the player, the world-record holders on other people's `datastore.course_records`, and for Super Mario Maker and Super Mario Maker 2 the player's
  published (public, completed, not deleted) courses. Private messages (both directions), matchmaking and tracking rows, rankings, profile and common data, login names are deleted. See docs/erasure.md.

## v1.7.0 — 2026-11-03
- Per-RMC logging: one always-on line per RMC request (title, protocol, method, parameter size) on stderr. Never the parameters. NEX_PACKET_LOG=1 still adds the verbose packet trace.

## v1.6.0 — 2026-11-03

- Persistence audit: rewards-emit v0.3.0. The rewards outbox goes to Postgres when `REWARDS_EMIT_DATABASE_URL_BRIDGE` (all NEX units are on openpak.network) or `REWARDS_EMIT_DATABASE_URL` is set; unset keeps the memory queue.
- The session counting clock is saved to the rewards-emit state (`sessions.<metric>`) on every poll and restored at start, so a restart no longer restarts the 3-minute windows.
- Login names (NSA id to PID) are written through to Postgres (`nexhost.login_names`, the title's own database) and looked up on a miss, so a title asking after a restart still resolves. `UseLoginStore(db)` for titles whose database is not `NEX_DATABASE_URL`.
- monster-hunter-4, monster-hunter-xx, monster-hunter-stories, tetris-axis, ice-station-z, resident-evil-the-mercenaries-3d: `UserDataCache` is now mutex-guarded (concurrent map write crash risk). It is a 24 h cache of account lookups, nothing to persist. A fresh lookup now also returns its own result instead of the stale empty entry.

## v1.5.2 — 2026-11-03

- Monster Hunter Generations Ultimate reports online sessions (`game_sessions.monster-hunter-generations-ultimate`), card #80.
  `PlayerOf("switch", pid)` names a Switch player as the 16-hex PID, the subject switch-nex sends.

## v1.5.1 — 2026-11-03

- The session watcher holds one connection only while it polls (max open 1, no idle): Postgres is capped at 100 connections and the 29 new watchers must not use them up.

## v1.5.0 — 2026-11-03

- rewards: online sessions (`WatchSessions`, two players sharing a gathering for 3 minutes) for every title on the
  common Postgres matchmaking (`matchmaking.gatherings`), metric `game_sessions.<title>`. Wii U (namespace `wiiu`):
  bayonetta-2, dr-luigi, monster-hunter-3-ultimate, minecraft-wiiu, pokken-tournament, puyo-puyo-tetris,
  sonic-all-stars-racing-transformed-wiiu, terraria. 3DS (`3ds`): animal-crossing-new-leaf, f1-2011, ice-station-z,
  ironfall-invasion, kid-icarus-uprising, luigis-mansion-2, metroid-prime-federation-force, monster-hunter-4,
  monster-hunter-4-ultimate, monster-hunter-generations, monster-hunter-stories, monster-hunter-xx, pokemon-gen6,
  puyo-puyo-20th, resident-evil-the-mercenaries-3d, steel-diver-sub-wars, super-street-fighter-iv, tetris-axis,
  tri-force-heroes, yo-kai-watch-2, yo-kai-watch-blasters. MK7, MK8, Splatoon and Smash keep their family metrics.
  No matchmaking, so no session metric: miracle-cure, wiiu-chat (own handlers), pikmin-3, super-mario-3d-world,
  super-mario-maker, team-kirby-clash-deluxe, pokemon-rumble-world, tipping-stars. No Pokemon GTS exists in this
  repo, so no `trades_completed`.
- rankings: new shared store `internal/scoreboard` (the Puyo Puyo Tetris / Sonic implementation in one place:
  ranking.scores, categories, common_data; global, nearby, friends, nearby-friends and own boards; common data
  read and write). Tetris Axis, Puyo Puyo!! 20th (Ranking is now registered; StorageManager stays unregistered),
  Pokken (the two stubs are gone), Smash 3DS, Smash Wii U, Yo-kai Watch 2, MH4, MH Stories, MHXX, Ice Station Z and
  RE Mercenaries 3D now keep uploaded scores and serve them on the rankings contract (`RANKINGS_ADDR`). Category
  ids are not documented for these games, so boards are named `Board N`; no capture was available, so the
  response layouts are the shared ones.
- rankings: Super Mario Maker serves "Course stars", "Maker stars" and "Fastest clears" from the datastore
  tables (`CombineRankings` joins SQLRankings sources).
- Pokemon X/Y/OR/AS: Ranking is not registered by the server (Rating `Unk1/Unk2` only); nothing stored or served.

## v1.4.12 — 2026-11-02

- rankings: the OpenPak rankings contract (`internal/rankingsapi`, read-only JSON on the container network) for every
  Wii U / 3DS title that already stores ranking rows, on `RANKINGS_ADDR` (for example `:9090`; unset = off). Network
  `wiiu`; rows carry the stored pid. Shared plumbing in `rankings.go`. Titles: dr-luigi, miracle-cure (`rankings`),
  puyo-puyo-tetris, sonic-all-stars-racing-transformed-wiiu, terraria (`ranking.scores`), mario-kart-7, mario-kart-8
  (time trials; the per-course `/api/v1/courses/{course}/rankings` API stays), pikmin-3 (category boards) and splatoon
  (one board per Splatfest). No new storage.

## v1.4.11 — 2026-11-02

- mario-kart-7: stores what the game uploads. Time-trial rankings on the legacy Ranking protocol (0x70; the
  upstream "legacy ranking" TODO) in Postgres schema `mk7`, ghosts on DataStore with bodies in S3 (`MK7_S3_*`),
  and a read-only JSON API on `MK7_API_ADDR` (courses, rankings, ghosts, objects, players; same shape as MK8).
  Communities stay the common persistent-gathering matchmaking. Response layouts and the course id are inferred
  (the NintendoClients wiki has no MK7 pages); every ranking/DataStore request is logged in hex for the console
  test. See `titles/mario-kart-7/docs/`.

## v1.4.10 — 2026-11-02

- rewards: online sessions (two players sharing a matchmaking gathering for 3 minutes, `REWARDS_SESSION_MIN`) are reported for Splatoon, Mario Kart 7, Mario Kart 8 and Smash 3DS / Wii U. The finished-race/match events are dropped.

## v1.4.9 — 2026-11-01

- rewards: Mario Kart 8 tournament runs, Splatoon Splatfest match uploads and Super Mario Maker course clears and
  uploads are reported to the rewards service (`REWARDS_EVENTS_URL` / `REWARDS_EVENTS_KEY`; unset = off). Async,
  never blocks a NEX call.

## v1.4.8 — 2026-11-01

- deps: nex-protocols-go and nex-protocols-common-go now come from the OpenPak forks
  (github.com/openpak/nex-protocols-go v2.3.8-openpak.2, github.com/openpak/nex-protocols-common-go
  v2.6.2-openpak.2, full upstream history, module paths unchanged, two replace lines). The forks add
  the Ubisoft/Quazal Rendez-Vous definitions and handlers, hook-driven common handlers for ranking 2,
  rating, subscription, storage manager, matchmake referee, persistent store, account management,
  friends, screening, AA user, subscriber and the Pokemon Bank shop, matchmaking Find* and block-list
  methods, ranking UseEmptyBoards() and a TestConnectivity hook. No title is changed; a handler a
  title assigns before NewCommonProtocol is kept.

## v1.4.7 — 2026-11-01

- splatoon (Wii U): Splatfest competition ranking (GetCompetitionRankingScore, UploadCompetitionRankingScore) with a
  per-team vote and win tally in `ranking_splatoon`; Splatfest lobbies opened for the opposing team (upstream
  feat/fests); at each rotation phase boundary, lobbies from the earlier phase are closed, from Rotationator's
  `schedule.json` (optional, `SPLATOON_ROTATION_FILE`); `SPLATOON_TRACE_MATCHMAKING=1` traces request shapes.
  Not console-verified. See titles/splatoon/docs/splatfest.md.

## v1.4.6 — 2026-11-01

- pikmin-3: rankings rewritten. One row per player and category, UpdateMode and OrderBy honoured,
  tie handling (113/123), group filter, and every GetRanking mode (range, near, friends, friends near, own);
  common data stored. UploadScore is logged so the category map can be learnt.
- pikmin-3: `nexd job pikmin-3-histgrm` registers the `histgrm` BOSS tasks and publishes a score
  histogram for all four Pikmin 3 BOSS apps. The file layout is provisional and the upload is guarded
  (see titles/pikmin-3/docs/histgrm.md).

## v1.4.5 — 2026-11-01

- super-mario-maker (Wii U): the internal API also serves GET /v1/makers/{id} and matches tags in q, as the course
  site expects (common/mario-maker docs/api-contract.md).

## v1.4.4 — 2026-11-01

- super-mario-maker (Wii U): GameplayConfig from Pretendo PR 29 (100 Mario bands and counts, ranking timeframe,
  upload notice); course lists (Latest, GetCustomRanking, ScoreRangeCascaded, BestScoreRate); difficulty
  filtering in RecommendedCourseSearchObject from clear and attempt ratings; course queries bounded to course
  data types (Pretendo issue 21). Internal read and bookmark API (SMM_API_ADDR, SMM_API_KEY), Dothub pickup
  list (SMM_DOTHUB_ADDR, off by default), datastore.bookmarks, maker medals, course codes. Not console-verified;
  see titles/super-mario-maker/OPENPAK.md.

## v1.4.3 — 2026-11-01

- mario-kart-8: read-only JSON API for the OpenKart site (openpak/mktv), off unless MK8_API_ADDR is set:
  Mario Kart TV highlights, ghosts per course, course rankings, per-player standings, tournaments with
  season leaderboards, and object bodies (images served as images, everything else as attachments).
  Includes the YouTube id the upload worker stores. Unit tests plus a Postgres-backed test
  (MK8_TEST_DATABASE_URL). Documented in titles/mario-kart-8/docs/read-api.md.

## v1.4.2 — 2026-11-01

- mario-kart-8: tournament create/modify/delete hardened and tested: owner-guarded SQL writes (only the
  creator modifies or deletes; the owner is the authenticated connection), bounded name/description/metadata,
  start-before-end, 50 tournaments per player, community code fixed at create. Postgres-backed unit tests.
- mario-kart-8: Mario Kart TV to YouTube worker (off unless MK8_YT_ENABLED=1; needs the owner-created Google
  credentials) with per-player opt-in and the YouTube video id stored for the MKTV site. Findings on how the
  game uploads to YouTube are in titles/mario-kart-8/docs/mktv-youtube.md.

## v1.4.0 — 2026-11-01

- titles: Bayonetta 2, Sonic & All-Stars Racing Transformed, Terraria and Monster Hunter 3
  Ultimate (Wii U), adapted from Pretendo's servers with their history (59 titles).
- deps: nex-protocols-common-go to b5bf44f (v2.6.2 pre-release): per-title MatchmakeSession
  attribute count and buffer length, JoinCommunity, AutoMatchmakeWithGatheringID_Postpone,
  UpdateMatchmakeSessionAttributes, GetDetailedParticipants. MH3U needs the first two.

## v1.2.2 — 2026-10-10

- logs: the login, ban, friend-list and endpoint lines no longer carry pids, account ids,
  login names, nicknames, tokens or client addresses. Where two lines have to be matched they
  carry a short per-process tag (`redact.Tag`: keyed HMAC prefix, key random per start).
- Net and URL errors are logged without their addresses (`redact.Err`).
- Dumps of request bodies, matchmake sessions and RMC parameters are behind
  `NEX_TRACE_BODIES=1`, for a local debug run only.
- `TestLoginLogsCarryNoPersonalData` drives a successful and a refused login and fails if a
  known id, name or token reaches the log.
- Still open: title servers that log a database or gRPC error by its text (`err.Error()`) can
  still show an id that the error itself contains.

## v1.2.1 — 2026-09-24

- deps: grpc 1.83.2 (Dependabot)
- ci: builds only on `v*.*.*` tags; build scratch in the runner temp, cleared after.

## v1.2.0 — 2026-09-23

- Bans (website/docs/ban-lookup.md): an account banned in the OpenPak admin is refused by
  every title with `RendezVous::AccountDisabled` (0x00030067), the NEX result for a disabled
  account, and its live connections are dropped within a minute.
  - Switch titles (the host): nx-baas answers `/internal/switch/identity` with 403
    `account_banned`/`account_deleted` for a locked-out account; login and every PID lookup
    now return AccountDisabled for it instead of a generic failure (never TokenExpired, which
    would send the console round for a new token).
  - Wii U and 3DS titles: nn-account's `GetNEXPassword` already refused a banned owner;
    `PasswordFromPID` in every title now answers `nexhost.NEXPasswordResult(err)`, which is
    AccountDisabled for that refusal and InvalidUsername otherwise, as before.
  - `nexhost.WatchBans`: every 30 s the secure endpoint re-asks its own
    `AccountDetailsByPID` about each connected PID and cleans up connections answered
    AccountDisabled. The host starts it; every Run-shaped title starts it on its secure
    endpoint. nx-baas and nn-account cache an active answer for at most 30 s, so a ban
    lands within 60 s. A lookup outage keeps the session.
  - No new environment variables.

## v1.1.0 — 2026-09-16

- host: `NEX_SECURE_TRANSPORT` splits the secure listener's transport from the auth
  listener's. Monster Hunter Generations Ultimate is the caller: its auth stage is wss
  behind Traefik's SNI passthrough, its secure stage raw PRUDP over UDP. Unset, the secure
  listener takes `NEX_TRANSPORT` as before.
- `titles/monster-hunter-generations-ultimate`: the auth listener is wss, not UDP, and the
  protocol set now matches `titles/monster-hunter-xx` — this title is that game's Switch
  port and runs its NEX code, down to the same access key. Message Delivery `0x1B`, Ranking
  `0x70` registered without an implementation, `EnableInsecureRegister`, `SendReport`
  accepting and discarding, `mmlegacy`, and the station URL type back to Public.
- `mmlegacy`: the five matchmaking methods `nex-protocols-common-go` v2.6.1 does not
  implement and the 3DS-lineage titles call — `FindByOwner` (0x15.0x16),
  `UpdateMatchmakeSessionAttribute` (0x6D.0x0C), `FindMatchmakeSessionBySingleGatheringID`
  (0x6D.0x31), `BrowseMatchmakeSessionNoHolderNoResultRange` (0x6D.0x34) and
  `FindMatchmakeSessionByParticipant` (0x6D.0x33). Lifted from `titles/monster-hunter-xx`
  with the matchmaking manager passed in instead of read from a package global, so the
  Switch port of that game can register the same set.
- fix: the release workflow parses again. `${{ runner.temp }}` in job-level `env` is not a
  valid context there, and an unparseable workflow has no triggers at all — the `v1.0.1`
  and first `v1.1.0` tags produced no run whatsoever, so no image had been published since
  `v0.8.0` while every host auto-updates against `:latest`.
- fix: the image builds again. `go mod download` cannot resolve the vendored nex-go
  `replace` target unless that tree is copied in first.

## v1.0.1 — 2026-09-15 (tagged, never built)

- nex-go: write raw PRUDPLite responses to bare-TCP Switch clients [a4e46e3]
- release: job-scoped registry auth file [7ba65b8]
- release: build on the self-hosted runner with podman; private-repo minutes are metered [7b4b63b]


## v0.7.0 — 2026-09-11



## v0.6.0 — 2026-09-11



## v0.5.1 — 2026-09-11



## v0.5.0 — 2026-09-11



## v0.4.0 — 2026-09-11



## v0.3.0 — 2026-09-10



## v0.2.1 — 2026-09-09



## v0.2.0 — 2026-09-09



## v0.1.0 — 2026-09-09



## v0.1.0 — 2026-09-09

- change: real module path github.com/openpak/nex [7c2ef78]
- fix: apply the Switch byte-stream profile in the host [0a48923]
- feat: expose the SecureConnection common protocol on Host [0c13f33]
- fix: Switch source key is 32 bytes, not 16, or the console rejects the ticket [d8d9eb2]
- fix: FriendPIDs must not truncate a 64-bit PID into another account [3c52fb4]
- change: default NEX_TRANSPORT to websocket [d9d3f57]
- feat: TLS on the websocket listener; nex-protocols-go 2.3.7 [f17788c]
- feat: websocket transport, one listener with two streams, secure PID [7479047]
- fix: correct the Switch login against the facts; expose DataStore [87b4cda]
- feat: the shared NEX host on Pretendo's nex-go [7f334db]
- seed: empty nex, AGPL-3.0-only, clean-room gate [9160fb5]

