# Splatoon (Wii U): Splatfest, rotation and matchmaking on OpenPak

What the game server does for the five Splatoon 1 features Pretendo lists, what it takes from elsewhere, and
what a console still has to confirm. Nothing here is console-verified yet.

## Upstream comparison (PretendoNetwork/splatoon)

| Upstream | Here |
| --- | --- |
| master (PR 24, libraries and readme) | Same server code as master; the only differences are the host shell and the ban watcher. |
| PR 22 (ranking functions) | Reimplemented, not ported: its types extract into value receivers (the request is never filled in), `Copy` copies the wrong field, the vote and win queries count team 1 for both sides and the upsert SQL has missing commas. Same table names (`ranking_splatoon.results`, `.user_scores`). |
| PR 19 (UploadCompetitionRankingScore) | Superseded by the same implementation. |
| PR 23 (swap station URL order) | Not taken: marked experimental and untested upstream, and it reverses the connection's URL list globally. |
| `feat/fests` (open Splatfest lobbies; a self-test goroutine) | Open-lobby part ported (below). The self-test is a development aid and not taken. |
| `work/splatfests-again-nmmr` | Same open-lobby hook, on the current library. |
| Issue 3 (118-0551 joining the opposing team) | Addressed by the open-lobby hook, unconfirmed. |
| Issue 12 (stale connections after the matchmaking rewrite) | Not addressed; it is in nex-go. |

## Competition ranking (Ranking 0x70, methods 0x10 and 0x12)

`nex/competition.go`, `nex/competition_types.go`; layouts from the NintendoClients wiki page "Ranking Protocol
(Splatoon)". Tables in the title database, created on start:

- `ranking_splatoon.results`: one row per uploaded match.
- `ranking_splatoon.user_scores`: one row per (player, festival), best score kept, team and latest team score updated.

Tally: `votes[team]` is the players on that team, `wins[team]` is the sum of `TeamScore` over the team's uploads.
GetCompetitionRankingScore answers one entry per requested festival id (`FestivalIDs`), score list ordered by score,
paged by the request's ResultRange. Methods 0x11 (by period list) and 0x13 (delete) are not implemented (their layouts are undocumented) and answer NotImplemented.

Assumptions to confirm on a console: whether `TeamScore` is a per-match increment (summed here) or a running total;
what the two "Unknown" fields in the score data and the first and third fields of the upload mean (this server
writes the list position and the team id into the first two and ignores the upload ones); the order of the team lists.

## Splatfest matchmaking

The game forms a lobby per team, then brings in the other team through AutoMatchmakeWithParam. This server keeps
the common library's attribute-range search (the game puts its team and rank in the search criteria) and adds
upstream's fix: after AutoMatchmakeWithParam for game mode 12 the caller's lobby is opened for participation.
There is no server-side team assignment, because the team is part of the game's own criteria; the record of who is on which team is the ranking
upload. Set `SPLATOON_TRACE_MATCHMAKING=1` to log the shape of every request (game mode, attribute values, criteria
count, no pids) when the attribute layout needs to be read from a real fest.

## Stage rotation in matchmaking

Consoles take their stages from the BOSS rotation file (Rotationator makes it). The server reads Rotationator's
JSON twin of that file (`/rotation/schedule.json`, `SPLATOON_ROTATION_FILE`) and, when a phase begins, closes
participation on regular (game mode 0) and ranked (1) lobbies that started in an earlier phase. Players then form new
lobbies whose members share the new phase. Without the file this does nothing. Go-live steps: rotationator
`docs/deploy.md`, "Game-server side of the rotation". The server does not choose stages; the host console does, from the rotation.

## Advanced matchmaking

Rank- and region-aware selection is the common library's: attribute ranges, nearest neighbour on attribute 1 and
broaden range, as the criteria ask. Attribute 4 of every criterion is cleared (upstream's rule). Another session is
improving the library; nothing title-specific is added beyond the trace above, until the attribute layout is read from a console.

## Results

Judd (match results the game posts) has a read API; OpenSplat (`splatnet` repo) shows its tally next to this
server's competition ranking at `/splatfest`.

## Console tests needed

1. Finish a Splatfest match: a row appears in `ranking_splatoon.user_scores`, the results screen shows team numbers.
2. Four players of each team: the opposing team joins (no 118-0551).
3. Cross a rotation boundary with a lobby open: the lobby stops accepting players, a new one forms on the new stages (needs the BOSS keys and the schedule file).
