# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): it is what makes "we
rewrote it from the facts" checkable instead of a claim.

Sources are named by these short labels:

| Label | What it is | Licence |
| --- | --- | --- |
| **kinnay/SMB35** | [`github.com/kinnay/SMB35`](https://github.com/kinnay/SMB35), an archived, publicly hosted Super Mario Bros. 35 server that worked against a retail client. Read directly (`source/main.py`, `source/config.py`). | AGPL-3.0 — compatible with ours, so this is allowed input under the policy's rule 3 |
| **NintendoClients** | [`kinnay/NintendoClients`](https://github.com/kinnay/NintendoClients) — the wiki (Game Server List, Eagle Protocol) and the generated protocol modules `nintendo/nex/matchmaking_eagle.py` and `ranking2_eagle.py` | MIT / public documentation — policy rule 2 |
| **Pretendo** | `nex-go/v2`, `nex-protocols-go/v2`, `nex-protocols-common-go/v2` read as source | AGPL-3.0 — policy rule 3 |

Where a fact is recorded in earlier research notes and in a public source, the public source is cited:
it is first-hand, and it is checkable by anyone.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `0a69c592` | NintendoClients wiki, Game Server List (Switch section). Also `s.configure("0a69c592", 40600, 0)` in kinnay/SMB35 `source/main.py`. |
| Game server ID | `21F12900` | NintendoClients wiki, Game Server List. Used in the console-facing hostname (`g21f12900…`). |
| NEX / `apiVersionGeneral` | `40600` = library version 4.6.0 | kinnay/SMB35 `source/main.py`, the second argument of `s.configure`. |
| Service shutdown date | 31 March 2021 (delisted 30 March 2021) | Nintendo's own announcement; the game was a time-limited release from the start. |

## Transport

| Fact | Value | Source |
| --- | --- | --- |
| Transport | PRUDP over **WebSocket** (secure, `wss`), not UDP | kinnay/SMB35 `main.py`: `settings.load("switch")` then `prudp.serve_transport(s, "", 20000, context)` with a TLS context. |
| Auth and secure share one listener | Yes: `rmc.serve_prudp(s, servers1, transport, 1)` and `rmc.serve_prudp(s, servers2, transport, 2)` on the same transport | kinnay/SMB35 `main.py`. |
| Auth stream ID | 1 | same |
| Secure stream ID | 2 | same |
| Secure server PID | `257049437023956657` | kinnay/SMB35 `main.py`, `SERVER_PID`. Used as the ticket target, the StationURL `PID`, and the source PID of pushed notifications. |
| Secure StationURL | scheme `prudps`, address `0.0.0.1`, port `1`, `PID=<secure PID>`, `CID=1`, `type=2`, `sid=2`, `stream=10` | kinnay/SMB35 `main.py`, `validate_and_request_ticket_with_param`. The address and port are placeholders: the console does not open a second socket, it switches virtual port on the one it has. |
| Login method | 6, `ValidateAndRequestTicketWithParam` | kinnay/SMB35 implements `validate_and_request_ticket_with_param` and no other login method. |
| Login source key | freshly generated per login and handed to the client, rather than derived from a password | kinnay/SMB35 `main.py`: `result.source_key = key.hex()`. This is the token-auth inversion `servers/nex` implements. |
| Session key length | 32 bytes | kinnay/SMB35 `generate_ticket`: `secrets.token_bytes(32)`. |
| Source key length | **16 bytes**, hex-encoded to a 32-character string | kinnay/SMB35 `main.py`: `key = secrets.token_bytes(16)`, `result.source_key = key.hex()`. Note this is *not* the session key, which is 32 in the same file — the two lengths differ on purpose. |

All of the transport facts are honoured, through `servers/nex` configuration rather than any
code here: `NEX_SECURE_STREAM_ID=2`, `NEX_SECURE_PID=257049437023956657`,
`NEX_STATION_SCHEME=prudps`, `NEX_VERSION=40600`, and the websocket transport that is now the
host's default. See [`example.env`](../example.env).

**The source key length is per title, and this title's is 16.** `servers/nex` defaulted to 32
on the grounds that 16 made a console reject a ticket; that evidence came from Mario Kart 8
Deluxe, where the key is derived as `sha256(...)` and is therefore 32 bytes wide because
sha256 is, not because a console asked for it. Two titles, two same-title values. The real
invariant is that the key encrypting the ticket must equal the key advertised in `source_key`
— RC4 takes any length — so "16 rejects the ticket" was a key mismatch misattributed to
length. `servers/nex` v0.2.0 made it `NEX_SOURCE_KEY_BYTES`; this title sets 16.

## Protocols the game calls

Taken from the list of servers kinnay/SMB35 registers on its secure connection
(`main.py`, `servers2`).
Protocol IDs are confirmed against Pretendo's `nex-protocols-go/v2` constants.

| Protocol | ID | Where it comes from here |
| --- | --- | --- |
| Secure Connection | 0xB | Pretendo common, registered by `servers/nex` |
| Match Making | 0x15 | Pretendo common |
| Matchmake Extension | 0x6D | Pretendo common |
| Match Making Ext | 0x32 | Pretendo common |
| Message Delivery | 0x1B | Pretendo common |
| Utility | 0x6E | Pretendo common, with our own `GetIntegerSettings` |
| Ranking2 | 0x7A | written here — Pretendo has no common implementation |
| Matchmake Referee | 0x78 | written here — Pretendo has no common implementation |

**DataStore is not in that list.** kinnay/SMB35 registers no DataStore server; the game never calls it. See
[`design.md`](design.md).

## Method IDs

Confirmed against Pretendo's `nex-protocols-go/v2` protocol constants, which agree with the
methods kinnay/SMB35 implements.

| Protocol | Method | ID |
| --- | --- | --- |
| Ranking2 | GetCommonData | 2 |
| Ranking2 | PutCommonData | 3 |
| Ranking2 | GetRanking | 5 |
| Ranking2 | GetCategorySetting | 7 |
| Ranking2 | GetEstimateScoreRank | 0xA |
| MatchmakeReferee | StartRound | 1 |
| MatchmakeReferee | GetStartRoundParam | 2 |
| MatchmakeReferee | EndRound | 3 |
| MatchmakeReferee | EndRoundWithoutReport | 4 |
| MessageDelivery | DeliverMessage | 1 |
| Utility | AssociateNexUniqueIDWithMyPrincipalID | 3 |
| Utility | GetAssociatedNexUniqueIDWithMyPrincipalID | 5 |
| Utility | GetIntegerSettings | 7 |
| MatchmakeExtension | AutoMatchmakeWithParamPostpone | 0x28 |

## Error codes

| Name | Value | Source |
| --- | --- | --- |
| `Ranking2::InvalidArgument` | `0x00710002` | Pretendo `nex-go/v2` result codes; the name kinnay/SMB35 raises for missing common data |
| `MatchmakeReferee::InvalidArgument` | `0x006F0002` | Pretendo `nex-go/v2` result codes |
| `MatchmakeReferee::NotParticipatedGathering` | `0x006F0004` | same; raised by kinnay/SMB35 when a named player is not in the gathering |
| `MatchmakeReferee::RoundNotFound` | `0x006F0007` | same; raised by kinnay/SMB35 for an unknown round ID |
| `Core::InvalidArgument` | Pretendo constant | raised by kinnay/SMB35 for an unknown integer-settings index |

## Wire structures that differ from Pretendo's

Super Mario Bros. 35 uses the **Eagle variant** of the matchmaking protocol module, and two
of its structures carry fields Pretendo's generated types do not have. Confirmed by reading
NintendoClients `nintendo/nex/matchmaking_eagle.py` beside Pretendo's Go types.

| Structure | Eagle variant | Pretendo's type |
| --- | --- | --- |
| `MatchmakeRefereeStartRoundParam` | personalDataCategory u32, gid u32, pids list\<pid\>, **reportSummaryMode u8, eventID u32** | first three fields only |
| `MatchmakeRefereePersonalRoundResult` | pid, personalRoundResultFlag u32, roundWinLoss u32, ratingValueChange s32, buffer qbuffer, **reportSummaryMode u8, eventID u32** | first five fields only |
| `AutoMatchmakeParam` | …, targetGids, **blockList (MatchmakeBlockListParam)** | up to targetGids |

Everything else checks out field for field: `MatchmakeSession` for NEX ≥ 4.0.0, and every
Ranking2 structure in `ranking2_eagle.py` (`Ranking2CommonData`, `Ranking2Info`,
`Ranking2CategorySetting`, `Ranking2GetParam`, `Ranking2EstimateScoreRankInput` and
`…Output`) match Pretendo's types exactly.

The extra fields are all **trailing**, and the affected structures are the last thing in
their message, so the leading fields still decode. How this server handles that is in
[`design.md`](design.md).

## Matchmaking flow

From kinnay/SMB35 `main.py` (`MatchmakeExtensionServer.auto_matchmake_with_param_postpone`
and `MatchMaker.join`):

1. The client calls `AutoMatchmakeWithParamPostpone` with a `MatchmakeSession` it proposes
   and a list of search criteria. `maxParticipants` is the client's number — the server
   never hardcodes 35 and never counts to it.
2. The server searches sessions against each criterion, picks one at random if any match,
   otherwise creates the proposed session.
3. The caller joins. Existing participants get a notification.
4. The joiner gets the Eagle handoff (below).
5. When a match begins, a client calls `MatchmakeReferee::StartRound` naming the gathering
   and the participants; the server checks they really are participants, allocates a round
   ID, and notifies each named player. Nothing on the server decides a lobby is ready.

## Notification events

| Type | Meaning | Source |
| --- | --- | --- |
| 3001 | participant joined (to existing participants) | kinnay/SMB35 `MatchMaker.join`; sent by Pretendo's common matchmaking already |
| 3007 / 3008 | participant disconnected / left | Pretendo common matchmaking |
| 4000 | ownership migration | Pretendo common matchmaking |
| 109000 | gathering destroyed | kinnay/SMB35 `MatchMaker`; Pretendo common matchmaking |
| 116000 | round started | kinnay/SMB35 `MatchmakeRefereeServer.start_round` — sent to every PID named in the round |
| 200000 | Eagle relay handoff | kinnay/SMB35 `MatchMaker.join` |

## The Eagle relay handoff

Eagle is a relay Nintendo used for four titles — Tetris 99, Super Mario Bros. 35, PAC-MAN 99
and F-Zero 99 — instead of Pia peer-to-peer (NintendoClients wiki, *Eagle Protocol*: "a
handful of games uses a relay server instead of Pia, which is called eagle"). NEX only forms
the lobby.

| Fact | Value | Source |
| --- | --- | --- |
| Notification type | 200000 | kinnay/SMB35 `MatchMaker.join` |
| Notification revision | 1 — the revision that has `m_mapParam`; revision 0 has no map, and the map is where the URL and token travel | Pretendo's `NotificationEvent` writes `MapParam` only when `StructureVersion >= 1` and the library version is ≥ 4.0.0 |
| Source PID | the secure server's PID (`257049437023956657` at retail) | kinnay/SMB35: `event.pid = SERVER_PID` |
| `param1` | the gathering ID | kinnay/SMB35 |
| Map keys | `url`, `token`, both string Variants (Variant type ID 4) | kinnay/SMB35; Variant type IDs from Pretendo `nex-go/v2` `init.go`, which matches NintendoClients' Variant table |
| URL shape | `wss://<host>:<port>/<gathering id>` | kinnay/SMB35: `"wss://smb35.ymar.dev:20001/%i" % gid` |
| Token | base64 of a JSON envelope `{"payload": {...}, "signature": "...", "version": 1}` — the payload is a nested **object**, not an encoded string | kinnay/SMB35 `MatchMaker.join` |
| Token payload fields | `expires_at` (epoch seconds as a decimal string), `server_env` (`"lp1"`), `server_id` (the **gathering** ID, decimal string), `user_id` (the PID as **sixteen** hex digits, zero-padded) | kinnay/SMB35 |
| Signature | HMAC-SHA256 over the payload's JSON as serialised, base64 | kinnay/SMB35 |
| Token lifetime | 3 hours (`time.time() + 10800`) | kinnay/SMB35 |
| Transport | WebSocket (`ws`/`wss`), also TCP and KDP; default port 443 secured, 30000 unsecured; retail URL shape `wss://<server>.g.<env>.e.srv.nintendo.net:443/<game id>/ess-d7d-<server id>` | NintendoClients wiki, *Eagle Protocol* |

The signing key is shared between this server and the relay and read by nothing else, so it
is ours to choose — but it must be the *same* on both, which is why it is configuration and
not generated per process.

## Utility integer settings

`Utility::GetIntegerSettings` index 0, taken verbatim from kinnay/SMB35 `source/config.py`
(`INTEGER_SETTINGS1`):

```
0:60  1:30  2:90  3:1   4:0   5:0   6:0   7:0   8:0
9:0  10:5  11:3  12:1  13:30 14:30 15:180 16:0
```

Index 10 (`INTEGER_SETTINGS2`) is empty in the same file, above the comment "What integers
should we use here?" — nobody has identified a value for it. Any other index is
`Core::InvalidArgument`.

The indices are unnamed on the wire and no document says what any of them means, so they are
carried across unchanged rather than reasoned about.

## Ranking2 values

From kinnay/SMB35 `Ranking2Server`:

- `get_ranking` always returns an empty board: `data = []`, `lowest_rank = 10000`,
  `num_entries = 0`, `season = 0`.
- `get_category_setting` returns the same setting for every category: `min_score = 0`,
  `max_score = 999999999`, `lowest_rank = 10000`, `reset_month = 4095`, `reset_day = 0`,
  `reset_hour = 0`, `reset_mode = 2`, `max_seasons_to_go_back = 3`, `score_order = 1`.
- `get_estimate_my_score_rank` returns zeroes with the requested category echoed back.
- `put_common_data` / `get_common_data` are the only real storage, keyed by PID and NEX
  unique ID; a missing entry is `Ranking2::InvalidArgument`.

## Observations from earlier research notes

Recorded because they are corroborating rather than load-bearing:

1. WSS for both endpoints and the same Eagle token layout, independently of kinnay's source.
2. An Eagle handoff was observed being **sent and never honoured** by a real PAC-MAN 99
   client, and it was not known whether the console processes pushed notifications at all. That
   is a warning about what is unproven, not an implementation detail — see the open questions
   in [`design.md`](design.md).

## Ports

`22160` from [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md): the next free NEX block of ten at the time this
repository was written. Auth `+0`, secure `+1`, `/healthz` `+2`, per the shared host's
convention.
