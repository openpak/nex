# Pikmin 3 rankings and the `histgrm` BOSS task

## What is established, and how sure we are

| Claim | Source | Confidence |
| --- | --- | --- |
| Scores travel over NEX Ranking (UploadScore, GetRanking, GetCommonData/UploadCommonData, GetCachedTopXRanking) | Pretendo `pikmin-3` registers the common Ranking protocol with the insert/range hooks; the game's access key and rankings table come from there | high |
| BOSS tasks `histgrm` exist for USA `9jRZEoWYLc3OG9a8`, EUR `4krJA4Gx3jF5nhQf`, JPN `VWqUTspR5YtjDjxa`, JPN trial `XcawL2u1CU624gg3` | nn-boss `list-known-boss-apps.ts` (Pretendo's known-apps list, taken from real consoles' SpotPass traffic) | high |
| The game asks to fetch "global ranking data" over SpotPass before showing rankings and cannot show them until it has it | Nintendo's manual and Pikipedia (parental controls "receive world rankings"); the in-game string `Lyt_SystemWindow_00_BossNoData` ("Global ranking data has not yet been received") | high |
| `histgrm` is the SpotPass half of that: a score distribution (histogram) | the task name and the above; nothing documents it | medium |
| The `histgrm` file format | not documented anywhere we could reach. Pretendo's pikmin-3 repo has no issue or PR on it; the SpotPass Archival Project dumped **no Wii U data** ("unable to archive data for any Wii U titles"); the 3DS archive on archive.org has no Pikmin 3 | **unknown** |
| Category ids per Challenge-mode course / Bingo Battle stage | not documented | **unknown** |

The server logs every upload (`UploadScore pid= category= score= orderBy= updateMode= groups= param=`),
so the category map is learnt from the first console session: play one stage per mode and read the log.

## What the server does (Ranking)

One row per (player, category) in `rankings`. UpdateMode Normal keeps the better score
(by the category's OrderBy; a tie keeps the older), DeleteOld always overwrites. GetRanking modes:
Range (offset 0 = first place, capped at rank 1000), Near (window centred on the caller, sliding at
the ends), FriendRange/FriendNear (ranked among the caller and their nn-friends friends; no friend
service means only the caller), User (the caller's own rank). OrderCalculation 113 shares ranks on ties
(1, 2, 2, 4), 123 numbers ordinally (older update, then lower pid). GroupIndex/GroupNum filter on
`Groups[i]`. TimeScope is ignored (all time). Common data is stored per (pid, unique id).

## The histgrm job

`nexd job pikmin-3-histgrm` (same image as the title): registers the `histgrm` task for each of the four
app ids (idempotent; this is what makes the tasksheet stop 404ing), reads `rankings`, buckets every
category into 32 bins, and uploads one file per app id. Flags: `-register` (register tasks only),
`-dry-run`. It skips the upload when the output is unchanged since the last upload (hash in `/data`).
It lives in the title, not its own repo: it needs the title's table and the nex image already
builds, publishes and rolls; a separate repo would add an image, CI, and a copy of the schema.

**The layout it writes is provisional** (see `histgrm/histgrm.go`): our own big-endian, versioned
structure so the pipeline can be proven. The job refuses to upload until
`HISTGRM_ACK_UNVERIFIED_FORMAT=1` is set, because a wrongly shaped file may hang the game's ranking screen.
Settling the layout needs the game: take Pikmin 3's code (RPX, from the owner's own copy), find the
BOSS `histgrm` consumer (the code that opens the downloaded file for the ranking screen), and
read the parse. `HISTGRM_FILE_NAME` / `HISTGRM_FILE_TYPE` (default `histgrm` / `AppData`) are guesses too.
Then replace `Build`/`Parse` and drop the guard.

## Go live (waiting on the BOSS keys)

Deployed 2026-11-01: image rolled, quadlet `openpak-pikmin-3-histgrm.container` and
`.timer` (in ~/.config/systemd/user, not the quadlet dir) installed, **timer not enabled**, `histgrm` tasks registered on nn-boss.

1. Set `PN_BOSS_CONFIG_BOSS_WIIU_AES_KEY` and `PN_BOSS_CONFIG_BOSS_WIIU_HMAC_KEY` in
   `~/openpak/deploy/.env` (console-dumped; nn-boss md5-checks them); `systemctl --user restart openpak-nn-boss`.
2. Verify the layout (above), update `histgrm/histgrm.go`, tag nex, let the image roll.
3. `systemctl --user start openpak-pikmin-3-histgrm.service`; the log should show one `uploaded, data ID` per app id.
   (Set `HISTGRM_ACK_UNVERIFIED_FORMAT=1` only for a deliberate test with the provisional layout.)
4. `systemctl --user enable --now openpak-pikmin-3-histgrm.timer`.
