# Provenance

Every non-obvious fact this server depends on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): the earlier network
server for this title is source-available and was read for facts only — no code, file
layout, type or function names, comments or docs were carried across.

## The title

| Fact | Value | Source |
| --- | --- | --- |
| Title ID (base) | `0100C9C00E25C000` | `earlier/mario-golf-super-rush` `README.md` and `main.go`, which record it as read off two NSP filenames — file dumps, not a guess |
| Title ID (update) | `0100C9C00E25C800` | same |
| Game server ID | `g211a3f00` | same; the value inside the DNS name below, and a separate thing from the access key |
| Online DNS name | `g211a3f00-lp1.s.n.srv.nintendo.net` | same, recorded there as the hostname a console asks to resolve when it attempts online play |
| Release | June 2021 | public |

## NEX identity

| Fact | Value | Confidence | Source |
| --- | --- | --- | --- |
| Access key | `1cb8027c` | **confirmed** | `earlier/mario-golf-super-rush` `main.go` and `README.md` (dated 2026-08-28). Neither the kinnay Game Server List nor the game binary yielded it; it was recovered from one real captured PRUDP-Lite `(connectionSig, client signature)` pair by exhausting the 2^32 8-hex-digit space, because the signature is `HMAC-MD5(MD5(accessKey), MD5(accessKey)+connectionSig)`. A brute force over the whole key space has exactly one answer, so this is a measurement, not an inference. It lives in `example.env`, never in source. |
| NEX version | 4.6.5 (`apiVersionGeneral` 40605) | **guess** | same repo, which is explicit that this is an era guess: Golf (June 2021) sits near Mario Party Superstars (October 2021), whose 4.6.5 is confirmed. Nothing has measured Golf's. |
| Secure server PID | 2 | convention | NEX's own `Quazal Rendez-Vous` account; matches the earlier server and Pretendo's own example |
| Session key length | 32 bytes | corroborated | the earlier server uses 32; `servers/nex` independently sets 32 for Switch. Two sources, same value |
| Station URL | `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | corroborated | the earlier server's secure station URL, field for field the same as the one `servers/nex` builds from `constants.StationURLPRUDPS` + `StreamTypeRVSecure` (10) + `type` 2 (public) |
| Station scheme must be `prudps`, not `prudp` | — | **observed, other titles** | the earlier server records that with `prudp` the handshake completes but CONNECT carries an empty payload: no ticket, no session key, and the title dies when Pia first needs the session. Observed there on Splatoon 2 and Smash Ultimate; MK8 is the exception that uses `prudp`. Golf's own value is that repository's default and is untested on hardware |

## Transport

| Fact | Source |
| --- | --- |
| A Switch NEX title is reached over a secure WebSocket, not UDP | The earlier server for this title listens with TLS certificate and key on both endpoints and is fronted by an SNI router on 443 for the DNS name above. Independently, `nex-go` parses PRUDPLite (first byte `0x80`) only when a WebSocket server is running (`prudp_server.go`, `handleSocketMessage`), and PRUDPLite is the Switch packet format — so "websocket transport" and "Lite packets" are one switch, not two |
| The secure endpoint must answer SYN with PRUDP minor version 0 | The earlier server carries a knob for exactly this and documents the failure: its own library defaulted to 5, and with 5 the console sends CONNECT with `plen=0` and never hands over the ticket. **We need no knob**: `nex-go` copies a minor version onto the ack only in its PRUDPPacketV1 branch (`prudp_endpoint.go`, `handleSyn`/`handleConnect`), so a Lite ack goes out with 0 already |
| Never force a minor version on the *authentication* endpoint | same repository, recorded as a Smash Ultimate regression: its auth stopped receiving logins entirely. We do not force one anywhere |

## Protocol IDs

Standard NEX values. They agree in three places — the earlier server's own protocol map,
[Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki), and
Pretendo's `nex-protocols-go/v2`, where each is a `ProtocolID` constant — so nothing here
rests on one source.

| ID | Protocol | Registered here |
| --- | --- | --- |
| `0x03` | NAT Traversal | yes |
| `0x0A` | Ticket Granting | yes, by `servers/nex` on the authentication endpoint |
| `0x0B` | Secure Connection | yes, by `servers/nex` |
| `0x0E` | Notifications | outbound only — matchmaking pushes events, nothing calls in |
| `0x15` | Match Making | yes |
| `0x32` | Match Making Ext | yes |
| `0x6D` | Matchmake Extension | yes |
| `0x6E` | Utility | yes |
| `0x70` | Ranking | no — see `docs/design.md` |
| `0x73` | DataStore | no — see `docs/design.md` |

Ticket-granting method IDs (1 `Login` / `ValidateAndRequestTicket`, 2 `LoginEx` /
`ValidateAndRequestTicketWithCustomData`, 3 `RequestTicket`, 6 `LoginWithContext` /
`ValidateAndRequestTicketWithParam`) and the rule that a library version `>= 4.0.0` routes
1, 2 and 6 to the Switch names are from
[Kinnay, Authentication Protocol](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol);
`servers/nex` implements 2 and 6 and cites them in its own design document.

The method names the earlier server's monitoring bothered to label are a weak hint at what
its author expected to see on the wire. Recorded because they cost nothing to keep, not
because they are evidence: Matchmake Extension `0x22` UpdateProgressScore, `0x26`
CreateMatchmakeSessionWithParam, `0x27` JoinMatchmakeSessionWithParam, `0x28`
AutoMatchmakeWithParamPostpone, `0x31` FindMatchmakeSessionBySingleID, `0x3C`
GetSimplePlayingSession, `0x44` PrivateRoomCreate, `0x45` ResolveCode; Match Making `0x02`
UnregisterGathering, `0x15` FindBySingleID, `0x29` GetSessionURLs; Match Making Ext `0x01`
EndParticipation; Utility `0x01` AcquireNexUniqueID, `0x07` GetIntegerSettings, `0x08`
GetStringSettings; NAT Traversal `0x02` InitiateProbe, `0x03` RequestProbeInitiationExt,
`0x05` ReportNATProperties. All of them are already implemented by
`nex-protocols-common-go`.

## Status of the source server

| Fact | Source |
| --- | --- |
| The earlier server for this title is "playable" | its `README.md` |
| It registers Secure Connection, Matchmake Extension, Match Making, Match Making Ext, NAT Traversal, Utility and Ranking on the secure endpoint | its `main.go` |
| It does **not** register DataStore (`0x73`) at all, and records that whether Golf needs it is unknown until a real client's calls show it | its `main.go` and `README.md` |
| Its Ranking registration is a generic handler shared with other titles: an in-memory Mario Kart 8 Deluxe-shaped common-data and tournament store, `Core::NotImplemented` for everything else | `earlier/earlier-nex` `ranking.go` |
| Its matchmaking state is in-process and dies with the process | its `main.go`, which creates the matchmaking instance in memory and drops a player's lobbies on disconnect |

## Ours, not carried across

| Fact | Source |
| --- | --- |
| Identity, and the friend graph, come from nx-baas `POST /internal/switch/identity` | OpenPak's own `nx-baas` and `servers/nex` `account.go` |
| Port block 22130 (auth 22130, secure 22131, healthz 22132) | [`ports.md`](../../../ports.md), claimed by this repository |
| Matchmaking, utility, NAT traversal handlers, and every table they use | Pretendo's AGPL-3.0 `nex-protocols-common-go/v2` v2.6.1, imported. The schemas are created by the library's own `SetManager`, not written by us |
| `nex-protocols-go` v2.3.5 cannot wire Ranking: `ranking.Protocol.SetHandlerGetStats` takes `constants.StatsFlag`, `ranking.Interface` declares `types.UInt32`, so `common_ranking.NewCommonProtocol` does not compile. v2.3.7 fixes it | read from the two modules; the v2.3.7 build was tried and reverted here, since the pin belongs to `servers/nex` |
