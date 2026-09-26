# Provenance

Where every non-obvious fact in this server came from, as
[the clean-room policy](https://github.com/openpak/docs/blob/main/clean-room-policy.md) requires. Nothing here was
copied: the facts were read, written down, and the server was built from this page.

"earlier MHGU" below means the earlier network server for this title
(`~/REPOS/monster-hunter-generations-ultimate`, source-available) — read for facts only.
"Reference NEX" means its core, `~/REPOS/earlier-nex`.

"MHXX" means Pretendo's **Monster Hunter XX** server, AGPL-3.0, vendored in this tree at
[`servers/nex/titles/monster-hunter-xx`](../../monster-hunter-xx). It is the
strongest source on this page and the one that settles conflicts: MHGU is the Switch port of
Monster Hunter XX and runs that game's NEX code, and MHXX is a server people play against,
not a reading. It uses **the same access key, `4152f312`, and the same NEX version, 4.4.0** —
independent confirmation of two constants the other stack reports extracting from the binary.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `4152f312` | earlier MHGU `main.go`, package comment and `accessKey`. Not publicly documented — MHGU has no Kinnay wiki entry. They report finding it in the game's ARM32 code by cross-referencing the confirmed game-server-id constant: two independent `MOVW`+`MOVT` sites resolve the neighbouring struct field to the same null-terminated 8-character string. |
| Game server ID | `2896BD04` | Same file. They report capturing it from the real client's own handshake against Nintendo's still-live production server, with their redirect disabled so the real name resolution happened. Also the SNI name suffix `g2896bd04`. |
| NEX library version | 4.4.0 (`apiVersionGeneral` 40400) | earlier MHGU `main.go` `nexVersion`, from the SDK version strings embedded in the game binary: `SDK MW+Nintendo+NEX_DS-4_4_0` and `NEX_UT-4_4_0`. |
| Secure server PID | 2 | The "Quazal Rendez-Vous" special account of the 3DS/Wii U convention — [Kinnay, Kerberos Authentication](https://github.com/kinnay/NintendoClients/wiki/Kerberos-Authentication), Special Accounts. earlier MHGU records that this convention, not the Switch-specific one, is the one MHGU follows. |
| Session key length | 32 bytes | earlier MHGU `main.go`; the same value `servers/nex` already uses. |

## Why this title is not shaped like a Switch title

MHGU is the Switch port of the 3DS's *Monster Hunter XX*, and its online code came across
with it: earlier MHGU's README and package comment record that the NEX paths live in a
32-bit ARM codebase under an AArch64 shim, and that none of the NEX init is reachable from
the AArch64 side.

| Fact | Value | Source |
| --- | --- | --- |
| Transport, **secure** stage | raw PRUDP over UDP | earlier MHGU `main.go`: their ARM32 decompile shows the connection-type-9 secure connect opening a UDP socket, never a WebSocket, and they note Pretendo's MH4U server — same engine lineage — listens raw UDP for auth and secure. MHXX confirms the shape for the 3DS original: `globals.SecureServer.Listen(port)`, nex-go's raw PRUDP listener, PRUDP v1 at library version 4.4.0. **Still unproven on the Switch port** — no packet has ever reached anyone's secure port for this title. |
| Transport, **authentication** stage | wss (TLS, WebSocket) | **Not UDP, and this page said otherwise until 2026-09-16.** earlier MHGU's `main.go` has exactly one auth listener, `authServer.ListenSecure(443, cert, key)` — `ListenSecure` in earlier NEX `server.go` is `http.Server.ListenAndServeTLS` over a WebSocket mux. There is no UDP auth listener in their server at all; the single `UDPServer` they start is bound to the **secure** port. Corroborated twice: they report capturing the retail client's own DNS + TLS handshake against `g2896bd04-lp1.s.n.srv.nintendo.net` (that capture is where the game server ID came from), and OpenPak's own production Traefik already routes that SNI, TLS passthrough, to this title's auth port over TCP (`website/internal/netprofile/testdata/traefik.yml`, the live `dynamic.yml` snapshot). A console's login has been reaching their auth server well enough for them to iterate on its response shape. |
| Why the two halves differ | the Switch port kept the 3DS RMC layer and took the Switch transport for the stage the system reaches by SNI | Reading, ours. It is the only shape consistent with all three observations above; MHXX is raw UDP for both stages because a 3DS has no SNI-routed game-server hostname. Recorded as a reading, not a capture. |
| Kerberos ticket version | 0 | earlier MHGU `main.go`. Version 1 puts a random key in the clear in front of the encrypted body; dropping it takes the ticket from 148 to 124 bytes. |
| The size problem | client raises `2306-0116` before transmitting | earlier MHGU README and `main.go`. `2306-0116` is Core::BufferOverflow, "buffer too large to send" — a client-side check, which matches their observation that no packet ever reached the secure port. |
| **Where `2306-0116` comes from is not settled** | two readings, theirs, in the same repository | Their README blames the kerberos ticket echoed back inside the secure CONNECT, and calls ticket version 0 "the only lever". Their `main.go` `AuthConfig` comment blames something else entirely — the **method 0x6 response shape** — and sets `UseFullConnectionDataForContext: true` to send the fuller `RVConnectionData` shape instead of the flat one, "since the flat shape has produced a persistent, invariant Core::BufferOverflow (2306-0116)". So there are two levers, not one. This server already pulls the second: `login.go`'s method 6 answers the struct-headered `ValidateAndRequestTicketResult`, which is the fuller shape, and its comment records a console rejecting the flat method-2 layout with this same error code. |
| Secure station URL | `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | Scheme, CID, PID, sid and stream agree across both sources; `stream=10` is `StreamTypeRVSecure`. **`type` is a conflict.** earlier MHGU `main.go` sets `3` (BehindNAT\|Public) and records it as their reading of what a 3DS-derived Pia wants — against a title they have never got a packet out of. MHXX sets `constants.StationURLFlagPublic`, i.e. `2`, and is a server people play against. This server follows MHXX: a shipped server for the same engine outranks a reading. Flip it and watch if the console never dials the secure port. |

## Protocols the title uses

| Protocol | ID | Source |
| --- | --- | --- |
| Ticket Granting | `0x0A` | Standard NEX; [Kinnay, Authentication Protocol](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol). |
| Secure Connection | `0x0B` | Standard NEX. |
| NAT Traversal | `0x03` | Standard NEX. |
| Match Making | `0x15` | Standard NEX. earlier MHGU registers it. |
| Match Making Ext | `0x32` | Same. |
| Matchmake Extension | `0x6D` | Same. |
| Utility | `0x6E` | earlier MHGU: confirmed present from the binary's `NEX_UT-4_4_0` SDK string. Not in MHXX — the Switch port added it. |
| DataStore | `0x73` | earlier MHGU: confirmed present from the binary's `NEX_DS-4_4_0` SDK string. Not in MHXX — the Switch port added it. |
| Message Delivery | `0x1B` | MHXX `nex/register_common_secure_server_protocols.go` registers it with `commonmessagedelivery` over a `MessagingManager`. Neither the other stack nor this server had it before 2026-09-16. |
| Ranking | `0x70` | MHXX registers `ranking.NewProtocol()` with **no** common implementation: the protocol exists, every method answers `Core::NotImplemented`, and the call is logged. Registered here the same way — what this title wants from ranking is unmeasured, and the log is how that gets found. |

### Methods `nex-protocols-common-go` does not implement

MHXX supplies five handlers of its own because the common library (v2.6.1) has no file for
them. A title that registers the common protocols and stops there answers
`Core::NotImplemented` to all five. They are now in
[`servers/nex/mmlegacy`](../../../mmlegacy), lifted from MHXX with the matchmaking manager
passed in rather than read from a package global, and registered here.

| Method | ID | What MHXX does |
| --- | --- | --- |
| `FindByOwner` | `0x15.0x16` | SQL over `matchmaking.gatherings` ⋈ `matchmake_sessions` for the owner's joinable sessions, answered as a `GatheringHolder` list. Length 0 defaults to 50 and offset `0xFFFFFFFF` to 0. |
| `UpdateMatchmakeSessionAttribute` | `0x6D.0x0C` | Owner-only (`RendezVous::PermissionDenied` otherwise); refuses more attributes than the session has slots; writes each slot through `UpdateGameAttribute`; answers an empty body. |
| `FindMatchmakeSessionBySingleGatheringID` | `0x6D.0x31` | `GetMatchmakeSessionByID`, session written whole. |
| `BrowseMatchmakeSessionNoHolderNoResultRange` | `0x6D.0x34` | The range rides inside the criteria, so the cap is a local 50; offset `0xFFFFFFFF` normalised to 0; **session key and user password stripped from every result** — a browse is a poster, not an invitation. Answered as bare sessions, not holders. |
| `FindMatchmakeSessionByParticipant` | `0x6D.0x33` | `g.participants && $1` against the requested PID list, capped at 50, answered as `FindMatchmakeSessionByParticipantResult`s. |

Every one of them filters on the same joinable-session predicate: registered, type
`MatchmakeSession`, a real host, open participation, not full, no user or system password.

### Secure Connection behaviour

| Fact | Value | Source |
| --- | --- | --- |
| `Register` (`0x0B.1`) must be allowed | `EnableInsecureRegister()` | MHXX. `nex-protocols-common-go` refuses plain `Register` by default and serves only `RegisterEx` (`0x0B.4`); the 3DS-era Pia calls the plain one. |
| `SendReport` (`0x0B.6`) | accept and discard | MHXX sets a `CreateReportDBRecord` that returns nil. Left unset the common protocol answers `Core::NotImplemented`. |
| `CleanupMatchmakeSessionSearchCriterias` | not set | MHXX's is an empty function, which is what leaving the common hook nil already does. Not carried over. |

DataStore method IDs used here (`GetMeta` `0x8`, `GetMetas` `0x9`, `SearchObject` `0xC`,
`RateObject` `0xF`, `PostMetaBinary` `0x15`, `PreparePostObject` `0x18`, `PrepareGetObject`
`0x19`, `CompletePostObject` `0x1A`, `ChangeMeta` `0x26`, `DeleteObject` `0x4`) come from
`nex-protocols-go/v2/datastore`, which is AGPL-3.0 and a dependency of this server.

## Status of the title on the other stack

earlier MHGU's README and `mhgu_init.go` record that no measured reference of MHGU's online
init exists: they log every DataStore and Utility call and answer it with an empty list
(`count=0`), except `0x73.8` (`GetMeta`), which they answer `0x80690004` — DataStore::NotFound
— so the game continues instead of soft-locking on `Core::NotImplemented`. Reference NEX has
no general DataStore implementation at all; its only `0x73` code is Super Mario Maker 2's.
That is the gap this server fills, and the reason nothing here is a stub.

Their MHGU server also registers no Message Delivery, no Ranking, and none of the five
matchmaking methods above: its matchmaking is Reference NEX's in-memory `NewMatchmaking()`.
So on the question the user asked — what can be taken from the other stack — the honest
answer is that for this title it holds constants and two transport observations, and MHXX
holds the protocol behaviour.

## OpenPak's own

| Fact | Source |
| --- | --- |
| Port block 22120 (auth 22120, secure 22121, healthz 22122) | [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md), claimed by this repository. |
| Identity: BAAS id token -> nx-baas `/internal/switch/identity` -> PID, nickname, friends | [`servers/nex`](../../../README.md) `docs/design.md`; nx-baas's internal game API on ports.md 20070. |
| Login methods, ticket construction, account callbacks | `servers/nex`, which implements them. This server adds none of it. |
| Matchmaking, NAT traversal, unique IDs, DataStore handlers | Pretendo's AGPL-3.0 `nex-protocols-common-go/v2`. |
| Postgres 23000, S3 23010 | `ports.md`. |
