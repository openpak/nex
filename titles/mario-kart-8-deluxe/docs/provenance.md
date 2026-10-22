# Provenance

Required by [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md). Every
non-obvious fact this server is built on, and where it came from.

Three kinds of source appear below:

- **Research notes** — earlier research notes on this title. What is taken is numbers, names
  on the wire, and descriptions of behaviour observed against real consoles and against
  Nintendo's own servers before shutdown.
- **Pretendo** — the AGPL-3.0 libraries this server imports, read as source we are licensed
  to use.
- **OpenPak** — our own repositories.

Nothing here has been confirmed by a capture of ours. Everything is marked with what would
confirm it.

## Title identity

| Fact | Value | Source | How to confirm |
| --- | --- | --- | --- |
| NEX access key | `09c1c475` | earlier research notes — a literal constant in a server that ran this title live, **not derived from anything**; a well-known per-title value derivable from the game itself, not a secret | Any PRUDP packet from the console: the packet signature is an HMAC keyed on it, so a wrong key drops every packet at SYN |
| NEX version | `4.0.0` (the console reports the integer `40000`) | earlier research notes: 40000 for Mario Kart 8 Deluxe, which reports 4.0.0 | `apiVersionGeneral` in the login packet |
| Secure server PID | `2` | earlier research notes — also the conventional NEX value | The `PID=` field the console reads out of the secure station URL |
| Login method | ticket-granting method **2**, `ValidateAndRequestTicketWithCustomData` | earlier research notes: method 6 is called by SSBU at boot (MK8 does not), and is the NEX 4.6+ shape | The RMC method ID of the first request on the authentication endpoint |
| `userName` at login | **the console's NSA id on hardware; the account's own PID on an emulator** | earlier research notes: the value branches: a bare number in the account range is the emulator's own PID, a larger one is a real CFW Switch's `baasUserID` resolved through the account service | The `userName` string in a login from hardware, next to one from an emulator |
| Secure station URL | scheme `prudp`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | earlier research notes | The station URL string in the login response, replayed against a console |

`stream=10` is `StreamTypeRVSecure` in Pretendo's `nex-go/v2/constants`, which is what
`servers/nex` already sets. `sid=1` means MK8D uses the **two-listener** shape — a separate
secure port — not the one-listener two-stream shape `servers/nex` describes as the retail
Switch form. A server that ran this title live used authentication on `:443` and secure on `:60003`, both reachable.

## The Switch NEX wire profile

From earlier research notes, taken from observed traffic and the public protocol docs. This is not MK8D-specific —
it is what any NEX 4 Switch title needs, and it is the most valuable thing in this document
for the rest of the fleet.

| Setting | Switch value | Notes |
| --- | --- | --- |
| PRUDP version | 2, "Lite", minor version 5 | Pretendo's `nex-go` calls this `PRUDPPacketLite` and selects it automatically when the first byte is `0x80` and the server is a WebSocket server |
| Transport | **WebSocket**, not UDP | Confirmed independently: a server that ran this title live listened with TLS on both endpoints, i.e. WSS |
| PRUDP payload encryption | **none** | The TLS tunnel provides confidentiality. Wii U/3DS use RC4 here |
| Compression | none | |
| PID size | **8 bytes** | `nex-go` defaults to 4 and has no version gate: `ByteStreamSettings.PIDSize` must be set |
| Structure headers | **on** (version byte + u32 content length before every structure) | `nex-go` defaults to off: `ByteStreamSettings.UseStructureHeader` must be set |
| String length prefix | u16 | `nex-go`'s default; the observed wire format uses a u16 too |
| Fragment size | 1300 | matches `nex-go`'s default |
| Kerberos key size | **32 bytes** | See below |
| Kerberos key derivation | "new": `md5(password)` once, append the PID as u64 LE, `md5` once | Wii U/3DS iterate `md5` `65000 + pid%1024` times. Only used between our own two endpoints, so it does not have to match anything external |
| Kerberos ticket version | 1 | `servers/nex` already sets this |

### The 32-byte source key

Earlier research notes: the source key encrypts the client ticket and is handed back as
`pSourceKey`, so the console decrypts it. It must be 32 bytes (the Switch kerberos key size);
a 16-byte key makes the console reject the ticket.

`servers/nex` `login.go` currently uses `sourceKeyLength = 16`. If those notes are right, no Switch
title on OpenPak can complete a login. This is the single most important fact in this file.
**How to confirm:** a console that receives a ticket and then fails at CONNECT, versus one
that succeeds, with the only change being the source key length.

The source key is sent as a lowercase hex **string**, not raw bytes — it is hex-encoded in both response shapes, and
`servers/nex` already does the same.

## What MK8D calls

Earlier research notes name these protocols and methods, which means they were seen on the wire from
real clients.

| Protocol | ID | Methods MK8D was observed calling |
| --- | --- | --- |
| Ticket Granting | `0x0A` | 2 `LoginEx` / `ValidateAndRequestTicketWithCustomData` |
| Secure Connection | `0x0B` | 1 `Register`, 4 `RegisterEx`, 7 `ReplaceURL` |
| NAT Traversal | `0x03` | 2 `InitiateProbe`, 3 `RequestProbeInitiationExt`, 5 `ReportNATProperties` |
| Match Making | `0x15` | 2 `UnregisterGathering`, `0x15` `FindBySingleID`, `0x29` `GetSessionURLs` |
| Match Making Ext | `0x32` | 1 `EndParticipation` |
| Matchmake Extension | `0x6D` | `0x22` `UpdateProgressScore`, `0x26` `CreateMatchmakeSessionWithParam`, `0x27` `JoinMatchmakeSessionWithParam`, `0x28` `AutoMatchmakeWithParamPostpone`, and the title-specific ones below |
| Utility | `0x6E` | 1 `AcquireNexUniqueID`, 7 `GetIntegerSettings`, 8 `GetStringSettings` |
| Ranking | `0x70` | 1 `UploadScore`, 4 `UploadCommonData`, 6 `GetCommonData` |

**DataStore (`0x73`) does not appear.** Nothing observed suggests MK8D ever asks for one. See [`design.md`](design.md).

### MK8D's own matchmake-extension methods

Method IDs above `0x36` are the title's own subclass and are not in Pretendo's
`nex-protocols-go`. Earlier research notes named them from measured traffic:

| Method | Observed name | Shape it records |
| --- | --- | --- |
| `0x39` | tournament search | criteria in, list out |
| `0x3A` | tournaments by ID list | request is `u32 count` then `count × u32`; **the same method ID is SSBU's pre-matchmaking probe**, which expects a bare `u32 = 2`, so the two are told apart by request length |
| `0x3C` | playing session | which gathering a given friend PID is in |
| `0x3F` | tournament join | id in, empty response |
| `0x41` | friends query | observed answer is an empty list |
| `0x44` | private room create | response is a `{ String code, DateTime }` structure |
| `0x45` | resolve room code | response is a bare `u32` gathering ID |

None of these are implemented here. **How to confirm any of them:** an RMC capture of the
request and the response, from a console using the feature.

## Observed behaviour worth knowing

| Observation | Source | Why it matters here |
| --- | --- | --- |
| MK8D never sends `ReportNATProperties` in a form that yields a NAT type; NAT has to be inferred from the `natm`/`natf` fields of the station URLs the client registers | earlier research notes | NAT-aware matchmaking cannot rely on the report |
| MK8D and Splatoon 2 send `Pa=<private address>` and expect station order `[lan, public]`; Animal Crossing needs the opposite | earlier research notes | Pretendo's common `GetSessionURLs` uses the MK8D order, so no override is needed |
| A client that crashes leaks its gathering; a live server needed a reaper and an on-disconnect purge after every listed player turned out to be a dead connection | earlier research notes | Pretendo's common match-making already hooks `OnConnectionEnded` and ends participation there |
| `UpdateProgressScore` is called during the session-keep loop and only needs an ack; refusing it stalls the loop | earlier research notes | Pretendo's common protocol implements it |
| An empty settings map satisfies MK8D's `GetIntegerSettings`; PAC-MAN 99 needs a real one | earlier research notes | Why this server returns empty maps rather than leaving the callback unset |
| MK8D responses are single-fragment; SSBU's 15.7 KB DataStore init is 13 fragments and needed pacing | earlier research notes | No fragmentation tuning needed for this title |
| A NEX 4 title's structure response for method 6 must be the `ValidateAndRequestTicketResult` structure with a version-1 header, not the flat method-2 layout; getting it wrong gives the console `2306-0116` | earlier research notes | `servers/nex` already writes both shapes correctly |

## OpenPak facts

| Fact | Value | Source |
| --- | --- | --- |
| Port block | 22150 (auth 22150, secure 22151, `/healthz` 22152) | [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md), claimed in this change |
| Identity | `POST /internal/switch/identity` on nx-baas, guarded by `X-Internal-Key` | `servers/nex/docs/design.md`, `nx-baas` |
| Friend graph | the `friends` array of that reply, via `Host.FriendPIDs` | `servers/nex/account.go` |
| Postgres | the matchmaking, utility and ranking schemas are created on first run | Pretendo's common protocols do this themselves; `ranking.go` here does the same for its two tables |

## Library facts

| Fact | Source |
| --- | --- |
| `nex-go` selects PRUDPLite when the first byte is `0x80` and the server has a WebSocket listener | `prudp_server.go`, `handleSocketMessage` |
| `nex-go` routes a packet to the endpoint bound to its destination virtual-port stream ID and drops anything addressed to an unbound one | `prudp_server.go`, `processPacket` |
| `PIDSize` and `UseStructureHeader` have no version gate — they are read straight off `ByteStreamSettings` | `byte_stream_out.go`, `byte_stream_in.go` |
| `nex-protocols-go` **v2.3.5** has a `ranking.Protocol` that does not satisfy `ranking.Interface` (`SetHandlerGetStats` takes `constants.StatsFlag`, the interface declares `types.UInt32`), so `nex-protocols-common-go`'s ranking cannot be wired at that version. **Fixed in v2.3.7**, which this module requires | Compiled both ways; `servers/mario-golf-super-rush` dropped Ranking over this at v2.3.5 |
