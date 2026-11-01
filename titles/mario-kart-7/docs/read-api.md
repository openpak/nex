# Mario Kart 7 read API (for OpenKart)

Read-only JSON on `MK7_API_ADDR` (production `:22855`, container network only, never published).
Same shape as the Mario Kart 8 API (`titles/mario-kart-8/docs/read-api.md`): lists are
`{total, offset, limit, items}`, `limit` default 20 and capped at 100, PIDs are decimal strings,
errors are `{"error": "..."}` with 400/404/503/501.

| Endpoint | Returns |
| --- | --- |
| `GET /api/v1/courses` | `{items:[{course, ranked, ghosts}]}` |
| `GET /api/v1/courses/{n}/rankings` | fastest first, `{rank, player, time_ms, time, updated}` (also `score` = time_ms) |
| `GET /api/v1/courses/{n}/ghosts?owner=` | ghost objects of a course |
| `GET /api/v1/objects/{id}`, `/body` | ghost metadata; raw bytes from S3 as an attachment (501 without S3) |
| `GET /api/v1/players/{pid}/rankings` | `{items:[{course, rank, of, time_ms, time}]}` |

An object is `{id, owner, name, size, data_type, slot, tags, meta_size, meta_hex, thumbnail, rating,
youtube_id, created, updated}` plus, for ghosts, `course` (= `data_type`), and `time_ms`/`time`
when the owner has a ranking time on that course. No `name` on rankings: the player's display
name is in the common data blob, whose layout is unknown (see `README.md`).

`course` is the ranking category the game sends (INFERRED to be the course; a console capture
settles it). There are no tournaments (communities are matchmaking records).
