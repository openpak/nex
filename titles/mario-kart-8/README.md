# mario-kart-8

Mario Kart 8 (Wii U, `000500001010EB00`) on the shared NEX host. Written in Go for OpenPak on
nex-go and the common protocol set; the title-specific behaviour is ported from Pretendo's Python
server (`mario-kart-8-python`, AGPL-3.0), which is the one Pretendo runs.

| Fact | Value | Source |
| --- | --- | --- |
| Game-server id | `1010EB00` | Kinnay's Wii U game-server list; also `ConfigurePNValidation` here |
| Title ids | `000500001010EB00` / `EC00` / `ED00` | Pretendo nex-viewer title database |
| NEX | 3.5.4, PRUDP v1 over UDP, structure headers | Pretendo's `server_config.py` (`NEX_VERSION = 30504`) |
| Access key | `25dbf96a` | Kinnay's list and Pretendo's config; configuration here (`NEX_ACCESS_KEY`), never a literal in source |
| Build string | `branch:origin/project/wup-amk build:3_5_16_2011_0` | Kinnay's Wii U game-server list |

## What it serves

- **Common:** ticket granting (passwords from nn-account over gRPC), secure connection, NAT
  traversal, MatchMaking / MatchMakingExt / MatchmakeExtension on the library's Postgres
  matchmaking manager, friend lists from nn-friends.
- **Tournaments** (MatchmakeExtension 36–41, `SimpleSearchObject`): create, update, delete,
  search by conditions, search by ids, join with a participant count. Wire layouts follow
  NintendoClients' `matchmaking_mk8d.py`; the Go library's types for these are placeholders.
  Tournament metadata (the game's `0x5a5a` chunk blob) is parsed for validation and the name.
- **Rankings:** the standard Ranking protocol on Postgres (all categories ascending; time
  trials) plus the competition methods 14–16 (`ranking_mk8d.py` layouts). Pretendo kept team
  totals in Redis; here they are sums over the scores table.
- **DataStore:** ghosts and Mario Kart TV through the common DataStore protocol on the same
  Postgres + MinIO store the Switch titles use (`MK8_S3_*`).

## Deliberately not ported

- Pretendo's `AmkjService` gRPC (maintenance mode, whitelist, kick, status sync) and its
  restriction collection: OpenPak moderation lives in the core and the website.
- Pretendo's S3 key scheme (`ghosts/<pid>/<slot>.bin`, `mktv/<id>.bin`); a fresh deployment
  uses the common library's `mk8/objects/<dataID>.bin`.
- The MK8-specific `DataStoreChangeMetaParam` / `DataStoreSearchParam` variants Pretendo
  defined for NEX 3.5; the Go library's version-aware types are trusted for them. If a console
  shows a ChangeMeta or SearchObject decode error in the log, this is where to look.

Not console-verified. `OPENPAK.md` in the other Wii U titles explains how consoles find a server
(nn-account's `servers` table); the same row exists for this one.
