# Changelog — nex

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

Note: the early history below is the upstream project (Pretendo nex-go and title servers);
OpenPak work starts at the port/fork commit.

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

