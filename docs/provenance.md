# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied from a Shield-licensed source. Each row is a value or a behaviour that
was read, written down, and then implemented in OpenPak's own shape. The DataStore backend is
OpenPak's own code, shared with `servers/arms` and `servers/mario-tennis-aces` (see the LIFT
CANDIDATE note below).

## Sources

| Tag | What |
| --- | --- |
| `earlier/ssbu` | `~/REPOS/super-smash-bros-ultimate`, the earlier network SSBU server (source-available), on the `earlier-nex` core. Read for facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki), incl. the Game Server List |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.7, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `servers/arms`, `servers/mario-tennis-aces`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `9587602b` | **Two independent sources agree.** `kinnay`'s Game Server List lists it for Super Smash Bros. Ultimate, and `earlier/ssbu` `main.go` carries it as its access-key constant. `servers/arms`'s own provenance cites the same value for SSBU as a sibling cross-check. Configuration here, never a literal in source. |
| Game Server ID | not recorded | `earlier/ssbu` does not carry one, and it is **not** the access key — the ID is the `g<hex>-lp1.s.n.srv.nintendo.net` hostname a console resolves, and neither is derivable from the other. Left unset until a capture or the Game Server List supplies it; it is not needed to serve. |
| NEX version | `4.0.0` (`40000`) — **unverified** | `earlier/ssbu` `main.go` sets `nexVersion = 40000`. But that file is a copied Mario Kart 8 Deluxe template — its own top doc comment still reads "Command mk8 runs the Mario Kart 8 Deluxe…" — so `40000` may be MK8's value carried over unchanged, not an SSBU fact. Kept as a starting guess. The console reports `apiVersionGeneral`; the first capture settles it, and `NEX_VERSION` takes the integer form directly (`40000`). |
| Pia version | 5.19 | `servers/arms` provenance, citing `earlier-nex`'s secure-connection source: the `type=0x0B` public-station shape belongs to Pia 5.19, "SSBU". |

`NEX_VERSION` decides whether the title logs in with method 2 or method 6; `servers/nex`
answers both `ValidateAndRequestTicketWithParam` and `…WithCustomData`, so the method is
client-driven and nothing here has to choose.

## Transport and endpoint shape

| Fact | Value | Source |
| --- | --- | --- |
| Transport | PRUDP over WebSocket, TLS | `earlier/ssbu` serves both endpoints with `ListenSecure(port, cert, key)` — TLS WebSocket — and never opens a UDP socket. `servers/nex`'s default is already `websocket`, so nothing is set. |
| Listener shape | **Two** listeners, authentication and secure on separate ports | `earlier/ssbu` `example.env` sets `AUTH_PORT=443` and `SECURE_PORT=60005`, two ports both listening, and its secure station URL carries `sid=1`. Two-listener shape, `NEX_SECURE_STREAM_ID=1`, the `servers/nex` default. |
| Secure endpoint stream ID | 1 | `earlier/ssbu`'s secure station URL carries `sid=1`. |
| Station URL | scheme `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | `earlier/ssbu` `main.go`. `servers/nex` independently builds exactly this shape (`StationURLPRUDPS`, `SetConnectionID(1)`, `SetPrincipalID(2)`, `SetStreamID(1)`, `StreamTypeRVSecure` = the tenth enum member = `stream=10`, `SetType(2)`). Every default is already right; nothing is overridden. |
| Station scheme | `prudps` | `earlier/ssbu` `main.go` (`NewStationURL("prudps")`). `prudps` makes the console treat the secure server as authenticated and send its kerberos ticket in CONNECT; with `prudp` CONNECT carries an empty payload and the title dies when Pia needs the session. `servers/nex` default. |
| Secure server PID | 2 | `earlier/ssbu` `securePID = 2`. The conventional NEX value and `servers/nex`'s default. |
| Session key length | 32 bytes | `earlier/ssbu` `sessionKeyLen = 32`. `servers/nex` hardcodes 32. |
| Source key length | **32 bytes, and it matters** | `earlier/ssbu` `resolveUser` records, in as many words, that the source key MUST be 32 bytes (the Switch kerberos key size) — a 16-byte key makes the console reject the ticket. This is direct evidence *for this title* for the 32-byte value that `servers/nex` defaults to (`NEX_SOURCE_KEY_BYTES=32`), so nothing is set. Contrast SMB35, for which a reference server sends 16. |
| PRUDP minor version 0 on the secure endpoint | not needed here | `earlier/ssbu` `main.go` sets `secureSettings.PrudpMinorVersion = 0` and documents a measurement: with the retail default answered (their `option=5`) the console sends CONNECT with `plen=0` and no ticket; answering `0`, like the retail server, gets a full 104-byte ticket. Moot on nex-go, which answers with the minor version the client sent (`prudp_endpoint.go`, "we can just support what the client wants", `pretendo-src`). Recorded because it is the sharpest wire fact in the SSBU tree. |

## Protocols SSBU speaks

Registered on the secure endpoint by `earlier/ssbu`'s server, which is the observed set. IDs
are `kinnay`'s and match `pretendo-src`'s constants.

| Protocol | ID | Status here |
| --- | --- | --- |
| TicketGranting | `0x0A` | `servers/nex` |
| SecureConnection | `0x0B` | `servers/nex` |
| NATTraversal | `0x03` | registered |
| MatchMaking | `0x15` | registered (`nex-protocols-common-go`) |
| MatchMakingExt | `0x32` | registered (`nex-protocols-common-go`) |
| MatchmakeExtension | `0x6D` | registered (`nex-protocols-common-go`) |
| Utility | `0x6E` | registered, **real** — was a stub on earlier, see below |
| DataStore | `0x73` | registered, **real** — was a stub on earlier, see below |
| Ranking | `0x70` | **not** registered — see below |

**Ranking.** `earlier/ssbu` `main.go` registers a Ranking handler, but that `main.go` is a
copied MK8 template (its doc comment still names mk8) and the handler it registers is
`earlier-nex`'s MK8 one, whose default branch answers `Core::NotImplemented` to every standard
Ranking method. So whatever SSBU called on `0x70` there was refused, and the server is recorded
as bringing SSBU's online menu up regardless — i.e. Ranking was not the blocker. SSBU's online
modes are matchmaking (quickplay, background matchmaking, Battle Arenas), not a NEX leaderboard.
It is therefore left off here; not registering it yields the same `Core::NotImplemented` SSBU
already tolerated. If a capture ever shows a real `0x70` call, `common_ranking.NewCommonProtocol`
is three lines and `servers/mario-tennis-aces` has the worked Postgres store to lift.

## The two protocols Reference stubbed, made real here

`earlier-nex` has no DataStore and its Utility init is stubbed. `earlier/ssbu`'s
`init_replay.go` answers **every** method of DataStore (`0x73`) and Utility (`0x6E`) with a
minimal empty-list response (`count = 0`), with one hand-written exception: `GetMeta` (`0x73.8`)
returns `DataStore::NotFound` (`0x80690004`). Its own README says this in as many words: "online
play is not complete out of the box". The methods it stubbed are the ones SSBU calls during its
OnlineCore bring-up.

**DataStore (`0x73`).** `pretendo-src`'s `nex-protocols-common-go/v2/datastore` implements the
protocol properly and asks only where objects live. `datastore.go`/`search.go` answer that with
Postgres (metadata) and presigned S3 (bodies). Made real, where earlier answered empty:

| Method | Was, on earlier | Is, here |
| --- | --- | --- |
| `GetMeta` (8) | `NotFound` `0x80690004` | the object's real metadata, ratings included, or a genuine `NotFound` |
| `GetMetas` (9), `GetMetasMultipleParam` (36) | empty list | the objects that exist |
| `PostMetaBinary` (21) | empty list (an id nothing stored) | a row, and the id of that row |
| `PreparePostObject` (24), `CompletePostObject` (26), `CompletePostObjects` (37) | empty list | a real object, with a presigned upload URL when a bucket is configured |
| `PrepareGetObject` (25) | empty list | a presigned download URL for a stored object |
| `SearchObject` (12) | empty list | objects matching owner, data type, tags, refer id and time range, ordered and paged |
| `ChangeMeta` (38) | empty list | the period, meta binary or data type actually changes |
| `DeleteObject` (4) | empty list | the object stops being found |
| `RateObject` (15) / `RateObjects` (40) | empty list | a rating slot's running total |

**Utility (`0x6E`).** `pretendo-src`'s `nex-protocols-common-go/v2/utility` with
`NewUtilityManager` over the same Postgres implements the NEX-unique-ID getters SSBU asks for
during bring-up (e.g. `GetAssociatedNexUniqueIDWithMyPrincipalID`, `AcquireNexUniqueID`). Where
Reference answered every one with an empty list, they now return real, persistent unique IDs.

## Wire behaviours observed, and what we do

| Fact | Source | What we do |
| --- | --- | --- |
| **Pia type-8 keepalive.** `earlier/ssbu` `type8.go`: SSBU's Pia 5.19 sends an undocumented PRUDP packet type 8 as a keepalive on the secure stream (empty payload, NeedsAck, ~every 3s). The base PRUDP switch knows only types 0–4, so with no handler it went un-ACKed and SSBU treated the connection as dead and **soft-locked entering the online menu**. ACKing it unblocked the menu. | `earlier/ssbu` `type8.go` | **Not built, noted as the top hardware risk.** nex-go's `packetHandlers` has no type-8 entry either, and its `PRUDPEndPoint.RegisterCustomPacketHandler` is exported (`Host.Secure` reaches it) — but whether nex-go's current build already ACKs it, and whether SSBU still soft-locks on our stack, needs one capture to confirm before we register a handler blind. This is the single most likely reason a first live run stalls at the online menu; see `design.md`. |
| DataStore `GetMeta` (8) answered `NotFound` `0x80690004` | `earlier/ssbu` `init_replay.go`; the value is `nex.ResultCodes.DataStore.NotFound` in `pretendo-src` | We return the same code when the object genuinely does not exist, and real metadata when it does. |
| `SecureConnection::TestConnectivity` (`0x0B.5`) is called by Switch titles and wants an empty success | `servers/nex` (a sibling title was measured calling it 23 times in six hours) | Answered for everyone inside `servers/nex`; nothing to do here. |

## Identity

`earlier/ssbu`'s login path (`main.go`, `gates.go`) resolves an `nx2.`-prefixed HMAC token, a
bare decimal PID, or an NSA id looked up against its account service, falling back to an FNV
hash for anonymous players — and its own comments flag the bare-PID branch as an
impersonation hole (PIDs are sequential from 1800000001). None of that is reproduced. OpenPak
resolves identity in `servers/nex` through nx-baas's `POST /internal/switch/identity`, which
proves a BAAS id token's `openpak` claim; this server never sees a username. The friend graph
comes back on the same call and is what `GetUserFriendPIDs` is wired to (friends-only DataStore
searches, and matchmaking's friend lookup).

## Error codes

| Code | Meaning | Source |
| --- | --- | --- |
| `0x80690004` | `DataStore::NotFound`, facility 105 | `earlier/ssbu` `init_replay.go`; the same value is `nex.ResultCodes.DataStore.NotFound` in `pretendo-src`, which is what this server returns |

## Ports

`22350` (auth `22350`, secure `22351`, `/healthz` `22352`) is this title's NEX block, reserved
in `openpak` `ports.md`. the earlier server's `:443` auth and `:60005` secure are theirs and do not carry
over; the fact that both are TLS WebSocket does, which is why the transport is `websocket`.

## LIFT CANDIDATE: the DataStore backend

`datastore.go` and `search.go` are the same DataStore backend `servers/arms` and
`servers/mario-tennis-aces` carry, adapted only in schema name (`ssbu`) and S3 key bases. It is
agreed to converge into the shared `github.com/openpak/nex/datastore` subpackage behind a
one-call `store.Wire(common, host)`. That subpackage is **not built yet**, so it is inlined here
rather than blocking on it. When it lands, delete these two files. The one thing that must change
on the way across is scoping: this uses a per-title Postgres schema; the shared store uses a
title key so one database can hold several titles.

## servers/nex changes needed

None to serve SSBU as scoped here. One is *latent*: if a capture shows SSBU depends on the
type-8 Pia keepalive on our stack, the fix is a `RegisterCustomPacketHandler(8, …)` that ACKs —
and whether that belongs in `servers/nex` (it is per-console Pia behaviour, not per-title) or in
this main is the open question. It is left unbuilt deliberately; see `design.md`.

## Facts we did not need

Recorded so nobody looks for them twice: `earlier/ssbu` also carries a monitoring dashboard
(`dashboard.go`), online-gate checks against its own account service (`gates.go`), and a
per-title kerberos password shared between its two endpoints. The first two are their
operational shape, not protocol facts. The third does not apply: `servers/nex` generates both
special accounts' passwords per process, because nothing outside that process derives a key
from them.
