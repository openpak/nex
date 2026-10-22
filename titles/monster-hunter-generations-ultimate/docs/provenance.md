# Provenance

Where every non-obvious fact in this server came from, as
[the clean-room policy](https://github.com/openpak/docs/blob/main/clean-room-policy.md) requires. Nothing here was
copied: the facts were read, written down, and the server was built from this page.

"MHXX" means Pretendo's **Monster Hunter XX** server, AGPL-3.0, vendored in this tree at
[`servers/nex/titles/monster-hunter-xx`](../../monster-hunter-xx). It is the
strongest source on this page and the one that settles conflicts: MHGU is the Switch port of
Monster Hunter XX and runs that game's NEX code, and MHXX is a server people play against.
It uses **the same access key, `4152f312`, and the same NEX version, 4.4.0** —
independent confirmation of two constants extracted from the game binary.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `4152f312` | Earlier research notes. Not publicly documented — MHGU has no Kinnay wiki entry. Found in the game's ARM32 code by cross-referencing the confirmed game-server-id constant: two independent `MOVW`+`MOVT` sites resolve the neighbouring struct field to the same null-terminated 8-character string. |
| Game server ID | `2896BD04` | Same notes. Captured from the real client's own handshake against Nintendo's still-live production server, with redirection disabled so the real name resolution happened. Also the SNI name suffix `g2896bd04`. |
| NEX library version | 4.4.0 (`apiVersionGeneral` 40400) | Earlier research notes, from the SDK version strings embedded in the game binary: `SDK MW+Nintendo+NEX_DS-4_4_0` and `NEX_UT-4_4_0`. |
| Secure server PID | 2 | The "Quazal Rendez-Vous" special account of the 3DS/Wii U convention — [Kinnay, Kerberos Authentication](https://github.com/kinnay/NintendoClients/wiki/Kerberos-Authentication), Special Accounts. Earlier research notes record that this convention, not the Switch-specific one, is the one MHGU follows. |
| Session key length | 32 bytes | Earlier research notes; the same value `servers/nex` already uses. |

## Why this title is not shaped like a Switch title

MHGU is the Switch port of the 3DS's *Monster Hunter XX*, and its online code came across
with it: earlier research notes record that the NEX paths live in a
32-bit ARM codebase under an AArch64 shim, and that none of the NEX init is reachable from
the AArch64 side.

| Fact | Value | Source |
| --- | --- | --- |
| Transport, **secure** stage | raw PRUDP over UDP | Earlier research notes: the ARM32 decompile shows the connection-type-9 secure connect opening a UDP socket, never a WebSocket, and Pretendo's MH4U server — same engine lineage — listens raw UDP for auth and secure. MHXX confirms the shape for the 3DS original: `globals.SecureServer.Listen(port)`, nex-go's raw PRUDP listener, PRUDP v1 at library version 4.4.0. **Still unproven on the Switch port** — no packet has been seen reaching a secure port for this title. |
| Transport, **authentication** stage | wss (TLS, WebSocket) | **Not UDP, and this page said otherwise until 2026-09-16.** The auth stage is a single TLS listener on 443 serving a WebSocket mux (`http.Server.ListenAndServeTLS`); there is no UDP auth listener, and the only UDP listener is bound to the **secure** port. Corroborated twice: the retail client's own DNS + TLS handshake against `g2896bd04-lp1.s.n.srv.nintendo.net` was captured (that capture is where the game server ID came from), and OpenPak's own production Traefik already routes that SNI, TLS passthrough, to this title's auth port over TCP (`website/internal/netprofile/testdata/traefik.yml`, the live `dynamic.yml` snapshot). |
| Why the two halves differ | the Switch port kept the 3DS RMC layer and took the Switch transport for the stage the system reaches by SNI | Reading, ours. It is the only shape consistent with all three observations above; MHXX is raw UDP for both stages because a 3DS has no SNI-routed game-server hostname. Recorded as a reading, not a capture. |
| Kerberos ticket version | 0 | Earlier research notes. Version 1 puts a random key in the clear in front of the encrypted body; dropping it takes the ticket from 148 to 124 bytes. |
| The size problem | client raises `2306-0116` before transmitting | Earlier research notes. `2306-0116` is Core::BufferOverflow, "buffer too large to send" — a client-side check, which matches the observation that no packet ever reached the secure port. |
| **Where `2306-0116` comes from is not settled** | two readings | One reading blames the kerberos ticket echoed back inside the secure CONNECT, and calls ticket version 0 "the only lever". The other blames the **method 0x6 response shape** and sends the fuller `RVConnectionData` shape instead of the flat one, since the flat shape produced a persistent, invariant Core::BufferOverflow (2306-0116). So there are two levers, not one. This server already pulls the second: `login.go`'s method 6 answers the struct-headered `ValidateAndRequestTicketResult`, which is the fuller shape, and its comment records a console rejecting the flat method-2 layout with this same error code. |
| Secure station URL | `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | Scheme, CID, PID, sid and stream agree across sources; `stream=10` is `StreamTypeRVSecure`. **`type` is a conflict.** Earlier research notes read `3` (BehindNAT\|Public) as what a 3DS-derived Pia wants, against a title no packet had been seen from. MHXX sets `constants.StationURLFlagPublic`, i.e. `2`, and is a server people play against. This server follows MHXX: a shipped server for the same engine outranks a reading. Flip it and watch if the console never dials the secure port. |

## Protocols the title uses

| Protocol | ID | Source |
| --- | --- | --- |
| Ticket Granting | `0x0A` | Standard NEX; [Kinnay, Authentication Protocol](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol). |
| Secure Connection | `0x0B` | Standard NEX. |
| NAT Traversal | `0x03` | Standard NEX. |
| Match Making | `0x15` | Standard NEX. |
| Match Making Ext | `0x32` | Same. |
| Matchmake Extension | `0x6D` | Same. |
| Utility | `0x6E` | Confirmed present from the binary's `NEX_UT-4_4_0` SDK string. Not in MHXX — the Switch port added it. |
| DataStore | `0x73` | Confirmed present from the binary's `NEX_DS-4_4_0` SDK string. Not in MHXX — the Switch port added it. |
| Message Delivery | `0x1B` | MHXX `nex/register_common_secure_server_protocols.go` registers it with `commonmessagedelivery` over a `MessagingManager`. This server did not have it before 2026-09-16. |
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

## OpenPak's own

| Fact | Source |
| --- | --- |
| Port block 22120 (auth 22120, secure 22121, healthz 22122) | [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md), claimed by this repository. |
| Identity: BAAS id token -> nx-baas `/internal/switch/identity` -> PID, nickname, friends | [`servers/nex`](../../../README.md) `docs/design.md`; nx-baas's internal game API on ports.md 20070. |
| Login methods, ticket construction, account callbacks | `servers/nex`, which implements them. This server adds none of it. |
| Matchmaking, NAT traversal, unique IDs, DataStore handlers | Pretendo's AGPL-3.0 `nex-protocols-common-go/v2`. |
| Postgres 23000, S3 23010 | `ports.md`. |
