# Account erasure

Card openpak/prds#120. When an account is deleted, the website finaliser calls this server before the core drops the
account's links.

```
DELETE http://<title container>:<NEX_HTTP_ADDR port, base+2>/internal/accounts/{account_id}/data
Authorization: Bearer $ACCOUNT_ERASE_KEY
-> 200 {"service":"nex-<title>","rows":N}
```

One container is one title with its own database, so the route is on **every** title container's `/healthz` listener
(never published to the host, never routed by Traefik) and the website calls them all. Status codes: 401 wrong or missing key,
400 `account_id` is not a UUID, 503 `ACCOUNT_ERASE_KEY` unset, 502 the account's PIDs or the database could not be read
(erasing "nothing" for an unknown PID set would be a lie; the finaliser retries). A second call removes 0 rows.
One line is logged per erasure: `account erased account=<uuid> rows=<n> service=nex-<title>`.

## Environment

| Variable | Purpose |
| --- | --- |
| `ACCOUNT_ERASE_KEY` | the shared bearer key; unset disables the route |
| `NEX_CORE_ADDR`, `NEX_CORE_KEY` | account core gRPC and its internal key (also `ACCOUNT_GRPC_ADDR` / `ACCOUNT_INTERNAL_KEY`): `ListAccountLinks` gives the account's `wiiu`, `3ds` (decimal PID) and `switch` (PID as 16 hex digits) subjects |
| `NEX_ACCOUNT_GRPC_ADDR`, `NEX_ACCOUNT_GRPC_KEY` | already set on Wii U/3DS titles; also asked for the account's Wii U/3DS PID when it has no link (shadow PNID) |
| the title's database variables | every variable ending `DATABASE_URL`, `POSTGRES_URI`, `POSTGRES_URL` or `DB_URL` that holds a Postgres URL (not `REWARDS_*`, not `*_TEST_*`) is scrubbed |
| the title's bucket variables | `*_S3_ENDPOINT/URL`, `*_S3_BUCKET`, `*_S3_ACCESS_KEY/KEY`, `*_S3_SECRET_KEY/ACCESS_SECRET/SECRET` (a group without an endpoint or credentials takes the unprefixed `S3_*`) |

## What is erased, by rule (the schema decides, not a title list)

| Rule | Tables it reaches (examples) |
| --- | --- |
| A row with a PID column belongs to the player and is deleted, children first (foreign keys) | rankings (`ranking.scores`, `ranking.common_data`, `rankings`, `mk8.ranking_*`, `mk7.ranking_*`, `splatoon` fest results), DataStore objects and their metadata (`*.datastore_objects`, `datastore.objects`, `meta_binaries`, `notification_metas`), `mk8.tournament_scores`, MKTV consent and videos, `common_data(s)`, `utility.unique_ids`, `nexhost.login_names`, `wiiu-chat` calls, bookmarks, messaging (`messaging.*messages` sent by or addressed to the PID, with their bodies), matchmaking (`matchmaking.gatherings` owned or hosted by the PID, sessions and join messages of those), `tracking.*` logs |
| A PID inside an array is removed from the array | `matchmaking.gatherings.participants`, `permission_recipients`, `del_permission_recipients`, `del_recipients` |
| An object that is deleted takes everything in its schema keyed by `data_id` and its bucket body | `*.datastore_ratings`, `datastore.ratings`, `object_ratings`, `buffer_queues`, `object_custom_rankings`, `course_records`, `<key base>/<id>.bin` |
| Messages to a gathering whose id equals the PID are not touched | `recipient_type = 1` only |

## What is kept, and why

| What | Decision |
| --- | --- |
| `mk8.tournaments` created by the player | kept, `owner_pid = 0`: players joined them (like published mods) |
| `datastore.course_records` first/best holder on other people's courses | kept, the holder becomes 0; the record is the other maker's |
| Super Mario Maker, Super Mario Maker 2 (`datastore.objects`): public, completed, not deleted objects | kept, owner 0: published courses other players have saved. Everything else of the player in those tables is deleted |
| `ranking.categories` and other tables with no player in them | untouched |

## Not covered here

- Scores of Switch Ranking titles served by `switch-nex` and anything in the core leaderboards: those services erase them.
- A title with `NEX_CORE_STORAGE=rankings` keeps its scores in leaderboards and title-storage (both erase on `account_deleted`); only its `corestore.pending_*` rows (players with no link yet) are here, and the PID-column rule reaches them ([core-backed-storage.md](core-backed-storage.md)).
- The rewards outbox (events naming the account) belongs to the rewards service.
- MKTV highlights already uploaded to the OpenPak YouTube channel (`mk8.mktv_videos.youtube_id`): the row is deleted, the video on YouTube is not.
- In-memory session state (live gatherings, `UserDataCache`): it ends with the connection, which `WatchBans` drops once the core refuses the account.
- Bodies in a bucket whose key is not `<digits>.bin` (replays of Super Smash Bros. for Wii U use their own keys).
