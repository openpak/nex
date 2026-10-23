# Provenance

Every non-obvious fact this server depends on, and where it came from. Required by
[clean-room policy](https://docs.openpak.org/contributors/clean-room/): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable. Facts below
marked `research` come from earlier research notes; no code was involved.

## The title

| Fact | Value | Source |
| --- | --- | --- |
| Title ID (base) | `0100C9C00E25C000` | `research`: read off two NSP filenames — file dumps, not a guess |
| Title ID (update) | `0100C9C00E25C800` | same |
| Game server ID | `g211a3f00` | same. The value inside the DNS name below, and **not** the access key: a game server ID and an access key are different things and neither is derivable from the other. That mistake has already been made once in this fleet |
| Online DNS name | `g211a3f00-lp1.s.n.srv.nintendo.net` | `research`, recorded as the hostname a console asks to resolve when it attempts online play |
| Release | June 2021 | public |

## NEX identity

| Fact | Value | Confidence | Source |
| --- | --- | --- | --- |
| Access key | `1cb8027c` (eight hex digits, as every NEX access key is) | **confirmed** | `research` (dated 2026-08-28). Neither the kinnay Game Server List nor the game binary yielded it; it was recovered from one real captured PRUDP-Lite `(connectionSig, client signature)` pair by exhausting the 2^32 8-hex-digit space, because the signature is `HMAC-MD5(MD5(accessKey), MD5(accessKey)+connectionSig)`. A brute force over the whole key space has exactly one answer, so this is a measurement, not an inference. It lives in `example.env`, never in source. |
| NEX version | 4.6.5 (`apiVersionGeneral` 40605) | **guess** | `research`, which is explicit that this is an era guess: Golf (June 2021) sits near Mario Party Superstars (October 2021), whose 4.6.5 is confirmed. Nothing has measured Golf's. |
| Secure server PID | 2 | convention | NEX's own `Quazal Rendez-Vous` account; matches Pretendo's own example |
| Session key length | 32 bytes | corroborated | `research` records 32; `servers/nex` independently sets 32 for Switch. Two sources, same value |
| Station URL | `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | corroborated | `research`: the secure station URL is field for field the same as the one `servers/nex` builds from `constants.StationURLPRUDPS` + `StreamTypeRVSecure` (10) + `type` 2 (public) |
| Station scheme must be `prudps`, not `prudp` | — | **observed, other titles** | `research` records that with `prudp` the handshake completes but CONNECT carries an empty payload: no ticket, no session key, and the title dies when Pia first needs the session. Observed on Splatoon 2 and Smash Ultimate; MK8 is the exception that uses `prudp`. Golf's own value is the default and is untested on hardware |

## Transport

| Fact | Source |
| --- | --- |
| A Switch NEX title is reached over a secure WebSocket, not UDP | Earlier research records both endpoints listening with a TLS certificate and key behind an SNI router on 443 for the DNS name above. Independently, `nex-go` parses PRUDPLite (first byte `0x80`) only when a WebSocket server is running (`prudp_server.go`, `handleSocketMessage`), and PRUDPLite is the Switch packet format — so "websocket transport" and "Lite packets" are one switch, not two |
| Both endpoints belong on one listener, with the secure endpoint on stream 2 | Fleet finding, carried to every Switch title: nex-go cannot bind two endpoints with the same stream ID to one server, so a distinct secure stream and a single listener are the same fact. `servers/nex` still defaults to the Wii U/3DS shape (stream 1, two listeners), so this is set explicitly here |
| Golf is reached by Traefik SNI passthrough, so this server terminates TLS | `ports.md`'s 21xxx range is the passthrough shape, and earlier research likewise has the game server hold its own certificate and key. The alternative — Traefik terminating and forwarding plain `ws` — is a real OpenPak deployment for other titles, so `servers/nex` supports both |
| The secure endpoint must answer SYN with PRUDP minor version 0 | Earlier research documents the failure: if the server answers 5, the console sends CONNECT with `plen=0` and never hands over the ticket. **We need no knob**: `nex-go` copies a minor version onto the ack only in its PRUDPPacketV1 branch (`prudp_endpoint.go`, `handleSyn`/`handleConnect`), so a Lite ack goes out with 0 already |
| Never force a minor version on the *authentication* endpoint | `research`, recorded as a Smash Ultimate regression: its auth stopped receiving logins entirely. We do not force one anywhere |

## Protocol IDs

Standard NEX values. They agree in
[Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) and
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
`servers/nex` implements 2 and 6 and cites them in its own design document. Two fleet
findings from earlier research sharpen that, and both are `servers/nex`'s code to get right rather than this
repository's: **method 6 is the live login path on Switch**, and its response needs a
structure header at version 1 — handed the flat method-2 layout, a console refuses the login
with `Core::InvalidArgument` and shows 2306-0116. And the `userName` field is the console's
**NSA id**, not the NEX PID: Kinnay's wiki reads "Username (your pid)", which is a trap,
because the two are never equal on Switch. A server that cross-checks the name against the
token's PID refuses every real console.

The method names earlier research expected to see on the wire are a weak hint. Recorded because they cost nothing to keep, not
because they are evidence: Matchmake Extension `0x22` UpdateProgressScore, `0x26`
CreateMatchmakeSessionWithParam, `0x27` JoinMatchmakeSessionWithParam, `0x28`
AutoMatchmakeWithParamPostpone, `0x31` FindMatchmakeSessionBySingleID, `0x3C`
GetSimplePlayingSession, `0x44` PrivateRoomCreate, `0x45` ResolveCode; Match Making `0x02`
UnregisterGathering, `0x15` FindBySingleID, `0x29` GetSessionURLs; Match Making Ext `0x01`
EndParticipation; Utility `0x01` AcquireNexUniqueID, `0x07` GetIntegerSettings, `0x08`
GetStringSettings; NAT Traversal `0x02` InitiateProbe, `0x03` RequestProbeInitiationExt,
`0x05` ReportNATProperties. All of them are already implemented by
`nex-protocols-common-go`.

## Ours

| Fact | Source |
| --- | --- |
| Identity, and the friend graph, come from nx-baas `POST /internal/switch/identity` | OpenPak's own `nx-baas` and `servers/nex` `account.go` |
| Port block 22130 — one listener on 22130 carrying both endpoints, healthz 22132, 22131 reserved and unbound | [`ports.md`](https://docs.openpak.org/contributors/ports/), claimed by this repository |
| Matchmaking, utility, NAT traversal handlers, and every table they use | Pretendo's AGPL-3.0 `nex-protocols-common-go/v2` v2.6.1, imported. The schemas are created by the library's own `SetManager`, not written by us |
| `nex-protocols-go` v2.3.5 cannot wire Ranking: `ranking.Protocol.SetHandlerGetStats` takes `constants.StatsFlag`, `ranking.Interface` declares `types.UInt32`, so `common_ranking.NewCommonProtocol` does not compile | read from the two modules here, verified against a v2.3.7 build, and reported to `servers/nex`, which has since bumped the pin to v2.3.7 |
