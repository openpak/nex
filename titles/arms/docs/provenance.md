# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape.

## Sources

| Tag | What |
| --- | --- |
| `research` | earlier research notes: facts recorded about the title's behaviour. No code. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.5, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `b6b34c51` | **Public source, independently confirmed:** [Kinnay's Game Server List](https://github.com/kinnay/NintendoClients/wiki/Game-Server-List) lists it for ARMS. Earlier research separately records reading the same value out of the ARMS update's main NSO `.rodata` at `0x42bda6`. Two independent sources agree. Configuration here, never a literal in source. |
| Game Server ID | `25C08801` | Kinnay's Game Server List. **Not** the access key — see the trap below. |
| NEX library version, update | 4.3.5 (`40305`), from `SDK MW+Nintendo+NEX-4_3_5-OL` in the update's `.rodata` | `research` |
| NEX library version, base game | 4.0.3 (`40003`) | `research` |
| Pia version | 5.7.0 in the update, 5.2.12 in the base game | `research` |
| Base title ID | `01009b500007c000`; update `01009B500007C800` | `research` |

`NEX_VERSION` is `4.3.5` because the update is what goes online. Whether the title logs in
with method 2 or method 6 is `servers/nex`'s open question 4 and is decided by that version,
not by anything here.

**The Game Server ID is not the access key.** ARMS's game server ID is `25C08801` and its
access key is `b6b34c51`; neither is derivable from the other. The ID is what appears in the
`g<hex>-lp1.s.n.srv.nintendo.net` hostname a console resolves, which makes it the thing most
likely to be mistaken for a key. An access key is always 8 hex digits and comes from the public
Game Server List, from the game binary, or from brute-forcing a captured pair against
`HMAC-MD5(MD5(key), MD5(key)+connectionSig)`. This mistake has already been made once in the
fleet, which is why both values are written down here side by side.

That the two independent sources for `b6b34c51` agree — a public wiki and a binary read — is
also a cross-check on the sources themselves: the same list gives Mario Kart 8 Deluxe
`09c1c475` and Super Smash Bros. Ultimate `9587602b`, the values recorded for the
sibling titles.

A wrong access key is the worst failure mode in this whole server: the PRUDP-Lite packet
signature simply does not match, the console retries forever, and neither side logs an error
(`research`). If ARMS reaches us and nothing at all happens, check this first.

## Protocols ARMS speaks

Protocol IDs from `pretendo-src`; which of them ARMS actually links from
`research`.

| Protocol | ID | Status here |
| --- | --- | --- |
| TicketGranting | `0x0A` | `servers/nex` |
| SecureConnection | `0x0B` | `servers/nex` |
| NATTraversal | `0x03` | registered |
| MatchMaking | `0x15` | registered |
| MatchMakingExt | `0x32` | registered |
| MatchmakeExtension | `0x6D` | registered |
| Utility | `0x6E` | registered |
| Ranking | `0x70` | **not** registered — see below |
| DataStore | `0x73` | registered, real implementation |

Ranking: earlier research records that **no `RankingProtocolClient` is linked
in either ARMS build**, and that the `Ranking::*` strings in the binary are NEX error-name
table entries rather than evidence of a client. We leave it off. If a capture ever shows a
`0x70` call, `common_ranking.NewCommonProtocol` is three lines.

DataStore: linked by the ARMS **update** only; the base game has no DataStore client at all
(`research`).

## Wire-shape facts, and which of them still matter

Three wire details need no setting on nex-go, because nex-go already answers the way ARMS wants:

| Detail | Finding | What we do |
| --- | --- | --- |
| Station scheme `prudps` | `prudps` makes the console treat the secure server as authenticated and send its kerberos ticket in CONNECT; with `prudp` CONNECT carries an empty payload and the title dies when Pia needs the session | `servers/nex` already builds the secure station URL with `constants.StationURLPRUDPS`. Nothing to set. |
| Secure PRUDP minor version 0 | the retail secure server answered PRUDP minor version 0, and if the server answers 5 the console sends CONNECT with `plen=0` | Moot on nex-go: `prudp_endpoint.go` sets `ack.MinorVersion` from the client's own SYN — *"we can just support what the client wants"* (`pretendo-src`). |
| Legacy Pia public station | ARMS's Pia 5.7.0 predates the 5.19 that introduced the `type=0x0B` + `Pa` public-station shape, so it wants the legacy `type=0x03`, no `Pa` | `pretendo-src`'s common SecureConnection builds the public station with `StationURLFlagPublic\|StationURLFlagBehindNAT` = `0x03`, which is the legacy shape already. Nothing to set. |

Two more facts from the same research that we **do** act on:

- **Two listeners, secure stream 1 — not the single-listener shape.** ARMS uses two
  separate WSS listeners, one for authentication and one for secure, and the secure
  station URL it hands out carries `sid=1`. Both say two-listener. A fleet-wide instruction to
  set `NEX_SECURE_STREAM_ID=2` was issued and then retracted: it is per-title, and of the
  titles checked only Super Mario Bros. 35 uses the single-listener form. ARMS keeps the
  `servers/nex` default of `1`. Recorded because the wrong value here is the difference between
  a console connecting and a log line reading `trying to connect to unbound PRUDPEndPoint`.
- **Transport is WebSocket, not UDP.** Both endpoints are TLS WebSocket
  and no UDP socket is opened, so ARMS speaks PRUDP over
  WebSocket. `servers/nex`'s `Transport` defaults to `udp`, so `NEX_TRANSPORT=websocket` is
  the one non-default this title needs. TLS termination is the front proxy's.
- **The secure station URL shape agrees exactly.** The title expects
  `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2`, scheme `prudps`.
  `servers/nex` independently builds `SetConnectionID(1)`, `SetPrincipalID(2)`,
  `SetStreamID(1)`, `SetStreamType(constants.StreamTypeRVSecure)`, `SetType(2)`,
  `StationURLPRUDPS` — and `StreamTypeRVSecure` is the tenth member of nex-go's enum, so
  `stream=10` matches. Every default is already right for ARMS; nothing needs overriding.

Two facts from earlier research that we have **not** resolved and that a capture must settle:

- **Pia keepalive, PRUDP packet type 8.** Undocumented, empty payload, `NeedsAck`, roughly
  every three seconds on the secure stream. The NEX SDK's own types stop at `TYPE_RAW=7`, so
  an unhandled type-8 keepalive went un-ACKed and Super Smash Bros. Ultimate treated the
  connection as dead and soft-locked entering the online menu. The research records that **whether ARMS's Pia 5.7.0 sends it at all is unknown**.
  nex-go's `packetHandlers` map has no entry for type 8 either. `RegisterCustomPacketHandler`
  is the hook if it turns out to be needed; we have not added one because we do not know that
  it is, and an unnecessary handler is a thing to maintain.
- **`SecureConnection::Register` (`0x0B.1`) versus `RegisterEx` (`0x0B.4`).** `pretendo-src`
  refuses plain `Register` unless `EnableInsecureRegister()` is called, and `servers/nex` does
  not expose that. See `design.md`.

## Identity

OpenPak resolves identity in `servers/nex` through nx-baas's
`POST /internal/switch/identity`, which proves a BAAS id token's `openpak` claim, and this server
never sees a username. The friend graph comes back on the same call, which is what
`GetUserFriendPIDs` is wired to.

## Ports

`22140` (auth `22140`, secure `22141`, `/healthz` `22142`) is the next free NEX block, claimed
in `openpak` `ports.md`. Both endpoints are TLS WebSocket, which is why `NEX_TRANSPORT=websocket`.

## Error codes

| Code | Meaning | Source |
| --- | --- | --- |
| `0x80690004` | `DataStore::NotFound`, facility 105 | `research`; the same value is `nex.ResultCodes.DataStore.NotFound` in `pretendo-src`, which is what this server returns |
| `2618-562` | `SessionKeepFailed` — what ARMS shows when Pia cannot recognise itself in a session, e.g. because the NEX PID is not the PID the game knows itself by | `research` |

## Facts we did not need

A per-title kerberos password shared between the two endpoints does not apply:
`servers/nex` generates both special accounts' passwords per process, because nothing outside
that process ever derives a key from them.
