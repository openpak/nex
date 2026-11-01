# Mario Kart 8 read API (for OpenKart)

This title serves a read-only JSON API when `MK8_API_ADDR` is set (production: `:22853`, container network only, never published). Every list is `{total, offset, limit, items}`; `limit` defaults to 20 and is capped at 100. PIDs are strings. Errors are `{"error": "..."}` with 400 (bad parameter), 404, 503 (store down; no detail leaked), 501 (no object store configured).
Configuration: `MK8_API_ADDR` (listen address; unset = off), `MK8_MKTV_DATA_TYPE` (optional), and the `MK8_S3_*` variables for object bodies. The site that consumes it is [openpak/mktv](https://github.com/openpak/mktv) (openkart.openpak.org).


| Endpoint | Returns |
| --- | --- |
| `GET /api/v1/mktv?owner=` | Mario Kart TV highlights, newest first |
| `GET /api/v1/objects/{id}` | one highlight or ghost |
| `GET /api/v1/objects/{id}/body` | the object's bytes from S3; PNG/JPEG are served as images, everything else as an attachment |
| `GET /api/v1/courses` | `[{course, ranked, ghosts}]` |
| `GET /api/v1/courses/{n}/ghosts?owner=` | ghosts for a course |
| `GET /api/v1/courses/{n}/rankings` | time-trial ranking, `{rank, player, time_ms, time, updated}` |
| `GET /api/v1/players/{pid}/rankings` | one player's `{course, rank, of, time}` per course |
| `GET /api/v1/tournaments` | tournaments, newest first |
| `GET /api/v1/tournaments/{id}?season=` | one tournament with the top 100 of a season (current by default) |

## What is a fact and what is inferred

Facts (NintendoClients by Kinnay, MIT: `ranking_mk8d.py`, `matchmaking_mk8d.py`): rankings are the Ranking protocol with the category as the course and ascending scores; time-trial scores are milliseconds; tournaments are `SimpleSearchObject` rows with competition leaderboards (higher score first).

Inferred, because no retail console has uploaded to this server yet (the production DataStore holds no objects): a ghost is a DataStore object with a persistence slot (key scheme `ghosts/<pid>/<slot>.bin`) whose `data_type` is the course; a highlight is a public finished object with no persistence slot that refers to no other object (`MK8_MKTV_DATA_TYPE` narrows it to one `data_type` once it is known); a thumbnail is an object whose `refer_data_id` is the highlight. Every object carries its raw `data_type`, `tags` and metadata size so the site can be corrected without a server change.

`youtube_id` on an object is reserved for the highlight upload feature and is empty until it exists; the site embeds it only when it is an 11-character id.
