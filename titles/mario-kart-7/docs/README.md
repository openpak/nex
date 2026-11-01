# Mario Kart 7 server notes

What the server stores, what it knows for a fact and what it guesses. FACT = from a public
source or from the library; INFERRED = chosen by us, no console has uploaded to this server yet.

Sources: NintendoClients wiki by Kinnay (MIT): Ranking protocol page, game server list (title id
`0001DB00`, access key `6181dff1`, NEX 2.4.3 as set in `nex/secure.go`). **The wiki has no Mario
Kart 7 page, no legacy Ranking page and no MK7 DataStore page**; everything MK7-specific below
is therefore INFERRED, and the legacy Ranking layouts come from the nex-protocols-go library
(`ranking/legacy`, Pretendo).

## What is stored

| Data | Where | Fed by |
| --- | --- | --- |
| Time-trial scores, one best row per (course, player) | Postgres `mk7.ranking_scores` | Ranking (0x70) UploadScore / UploadScores / ...WithLimit |
| Ranking common data (player blob) | `mk7.ranking_common_data` | Ranking UploadCommonData |
| Ghost metadata and ratings | `mk7.datastore_objects`, `mk7.datastore_ratings` | DataStore (0x73) |
| Ghost bodies | S3 `mk7/objects/<data_id>.bin` | console uploads through presigned URLs |
| Communities | Postgres (common matchmaking tables, persistent gatherings, up to 8 per player) | MatchmakeExtension create/search/join, handled by nex-protocols-common-go |

Schema is created at start (`database/init_postgres.go`). S3 settings: `MK7_S3_ENDPOINT`,
`MK7_S3_BUCKET`, `MK7_S3_ACCESS_KEY`, `MK7_S3_SECRET_KEY`, `MK7_S3_INSECURE=1` for plain HTTP
(same pattern as `MK8_S3_*`); unset = the three body-moving DataStore calls answer
NotImplemented, everything else works. Read API: `MK7_API_ADDR` (see `read-api.md`).

## Ranking

FACT: protocol 0x70 with the legacy method set (library `ranking/legacy`); Mario Kart 7 is NEX 2.4.3, so
category is a 32-bit field and the library's NEX >= 2 method numbering applies. The new-style
Ranking (methods 9 GetRanking ... on the wiki page) shares protocol 0x70 and is **not** served:
method numbers collide, so one title can answer only one of them. The upstream
`TODO - Add legacy ranking protocol!` is what this replaces.

Implemented methods: UploadScore (1), UploadScores (2), DeleteScore (3), DeleteAllScore (4),
UploadCommonData (5), DeleteCommonData (6), GetTopScore (0xA), GetCommonData (0xB),
GetScore (0xE), GetSelfScore (0xF), GetTotal (0x10), UploadScoreWithLimit (0x11),
UploadScoresWithLimit (0x12). The library's unnamed methods (0x7, 0x8, 0x9, 0xC, 0xD, 0x13)
are not implemented: they answer Core::NotImplemented, and their raw parameters are logged.

INFERRED:
- Category = course. Score list index 0 = time in milliseconds, lower is better; a slower upload
  never replaces a better one. Index 1 and the two "unknown" fields of UploadScore are stored as sent.
- Course ids are therefore "whatever number the game sends as the category". Nothing maps them to
  course names yet; the site shows numbers until a capture gives the table.
- GetScore: Range lists from `offset`; Near centres on the caller; User and the friend modes return the
  caller alone (no friend list on this title). Sort by `orderParam.ScoreIndex`, descending if
  `ScoreOrder` is Descending.
- GetTopScore returns the single best entry; GetTotal reads its 32-bit field as the category.
- Response layouts: score queries answer `List<RankingData>` (rank 1-based), GetTotal a UInt32,
  GetCommonData a Buffer, uploads/deletes an empty success. No public document confirms any of these.

## Ghosts

FACT: DataStore handlers are the common ones (PreparePostObject, CompletePostObject,
PrepareGetObject, GetMeta, SearchObject, ...); the object body goes to S3 directly.
INFERRED: a ghost is an object with a persistence slot (not 0xFFFF) whose `data_type` is the
course; the owner's ranking time on that course is shown next to it in the API.

## Communities

Mario Kart 7 communities are persistent gatherings (MatchmakeExtension), capped at 8 per
player here (`PersistentGatheringCreationMax`, from upstream). They are matchmaking records, not
leaderboards, so the read API has no tournaments for MK7 (`/api/v1/tournaments` is 404). The
search/creation methods are the common matchmaking implementation and stay as upstream wrote them.

## Console test needed

1. Play a time trial on any course, finish, and let the game upload. Read the log
   (`podman logs openpak-nex-mario-kart-7`): `Ranking::UploadScore method=1 params=<hex>` and the
   `params:` lines show the exact bytes. Confirm: category = course, scores[0] = time (ms).
2. Open the ranking screen for that course. Success = the game shows the entry; a hang or error
   dialog means a response layout above is wrong: send the `Ranking::GetScore params=` line and
   the game's behaviour so the layout can be fixed.
3. Download a rival's ghost: check `datastore_objects` for the upload and that the S3 key
   `mk7/objects/<id>.bin` exists.
4. Any `Unsupported Ranking method ID` / NotImplemented line in the log names a method the game
   needs that is not implemented.
