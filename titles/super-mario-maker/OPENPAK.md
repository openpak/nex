# Super Mario Maker (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/super-mario-maker` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=super-mario-maker` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_SMM_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_SMM_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_SMM_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_SMM_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_SMM_POSTGRES_URI` |
| `SecureHost` | `PN_SMM_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SMM_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_SMM_CONFIG_S3_ACCESS_KEY`, `PN_SMM_CONFIG_S3_ACCESS_SECRET`, `PN_SMM_CONFIG_S3_BUCKET`, `PN_SMM_CONFIG_S3_ENDPOINT`, `PN_SMM_CONFIG_S3_KEY_BASE`, `PN_SMM_CONFIG_S3_SECURE`, `PN_SMM_GRPC_API_KEY`, `PN_SMM_GRPC_SERVER_PORT`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-11):** row in nn-account's `servers` table (title ids from Pretendo's nex-viewer title database; Wii U game-server ids from Kinnay's Wii U list, where the id is the low 32 bits of the title id). Deployed as one container from the shared image. Not console-verified.

**Event course metadata (2026-09-11):** upstream refuses to start unless `900000.bin` exists in the S3 bucket (the reserved DataID 900000 that opens Course World). Production holds an **empty placeholder** so the server runs; Course World needs the real event-course metadata blob uploaded over it.

## Features beyond upstream (2026-11-01)

Upstream Pretendo (master e8b7ca1, plus the branches `feat/wiiu-gameplay-config`, `feat/level-delete-grpc`,
`feat/delete-course-script`, all merged) is fully ported. Nothing in its open PRs or issues is unmerged work;
issue 21 (a Course World crash from non-course objects) is fixed here by bounding every course query to
`data_type > 2 AND data_type < 50`. Everything below is OpenPak's, built from the NintendoClients wiki
("Data Store Protocol (SMM)" and "Data Store Codes", MIT, https://github.com/kinnay/NintendoClients) and from
facts other servers measured, and none of it has been seen with a real console yet.

| Feature | Where | How the game reaches it |
| --- | --- | --- |
| Course lists | `nex/datastore/.../course_lists.go` | `LatestCourseSearchObject` (64), `GetCustomRanking` (49, courses and the maker star ranking), `ScoreRangeCascadedSearchObject` (67, 100 Mario), `BestScoreRateCourseSearchObject` (81). Layouts after Protarium's fork of the Pretendo server (AGPL-3.0, commit 6d49108) |
| Difficulty filtering | `difficulty.go`, `course_query.go` | `RecommendedCourseSearchObject` extraData `["1", min, max, "0", "0"]` is an inclusive failure-rate band: 100 - round(clears * 100 / attempts), from the course's rating slots 2 (clears) and 3 (attempts; slot map from Pretendo's smm1-course-archive). Courses nobody attempted have no rate and show only in "All". Bands: easy 0-34, normal 35-74, expert 75-95, super expert 96-100 |
| 100 Mario Challenge | the above, plus `GetApplicationConfig(0)` | The config carries the band edges and course counts (Pretendo PR 29). The game builds its pools from `RecommendedCourseSearchObject` (50 per run) and `ScoreRangeCascadedSearchObject`; it needs 32 courses after dropping its own, so a nearly empty server cannot start a run. Dothub (see below) is off |
| Badges (maker medals) | `GetApplicationConfig(0)`, `api.Medals` | Medals are not stored: the game turns a maker's total stars into medals using the ten star thresholds in the config. The API reports `medals` per maker so the site matches the game |
| Course search | `/v1/courses?q=&maker=&difficulty=&sort=` | Site-side search by name, maker and difficulty. In-game, Course World finds a course by its code through the common DataStore metas, which already works |
| Course codes | `api/code.go` | `CCCC-0000-HHHH-LLLL`: checksum = bytes 3 and 2 of HMAC-MD5(key = MD5("9f2b4678"), little-endian data ID), tested against the wiki's worked example and three real Pretendo course codes |
| Bookmarks | `datastore.bookmarks`, `/v1/bookmarks` | **The way the game reads them is not known** (below). The server stores and serves the list |

### Bookmarks: what is known

The original Bookmark site and the game shared one HTTP service, `wup-ama.app.nintendo.net` ("Dothub", Pretendo
issue 21 and PR 29): the game fetches 100 Mario pickup lists from `/api/v1/pickup/{easy,normal,expert,super_expert}`
(XML list of decimal data IDs, at most 400) and posts clear times to a `cleared_courses` endpoint. The in-game
Bookmarks list is therefore most likely Dothub too, not a DataStore search; no public source documents its path.
`api.Server.DothubHandler` implements the pickup list and logs every other request in full (`dothub unhandled: ...`)
and answers 404, so one console session that opens Bookmarks shows the real path and body. Nothing routes
`wup-ama.app.nintendo.net` to the server yet (DNS ceiling, Traefik router and a leaf signed by the OpenPak CA, as
`wup-agmj` for judd), and `DOTHUB_ENABLED` is 0 until it does.

### Environment

| Variable | Meaning |
| --- | --- |
| `SMM_API_ADDR` | listen address of the internal JSON API (read API for the course site and bookmarks); off when empty |
| `SMM_API_KEY` | shared secret, header `X-Internal-Key`; without it every request is refused |
| `SMM_DOTHUB_ADDR` | listen address of the Dothub subset; off when empty |

### Internal API (`/v1`, key required)

`GET /v1/courses?sort=newest|stars|plays|clears|random&limit&offset&updated_since&q&maker&difficulty`,
`GET /v1/courses/{code-or-data-id}`, `GET /v1/courses/{id}/thumbnail`, `GET /v1/makers`,
`GET /v1/bookmarks?pid=|nnid=`, `PUT|DELETE /v1/bookmarks` with `{"pid"|"nnid", "course"}`.
The course JSON is the contract the site's `internal/upstream` client reads, plus `failures` and
`miiverse_comments`. `style`, `theme` and `description` are empty: they live inside the course file, which the
server does not parse. A maker's name is the name of the profile object the game uploads (the NNID).

### Event courses

`900000.bin` in the `nex-smmwiiu` bucket is still a 2-byte placeholder. Course World opens with it, but the
event courses (930010-930050) do not exist. The real file cannot be copied; it has to be rebuilt from public
format documentation.
