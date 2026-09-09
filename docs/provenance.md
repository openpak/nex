# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape.

## Sources

| Tag | What |
| --- | --- |
| `earlier/arms` | `~/REPOS/arms`, the earlier network ARMS server (source-available). Read for facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.5, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `b6b34c51` | `earlier/arms` `example.env` and `main.go`, which record it as read out of the ARMS update's main NSO `.rodata` at `0x42bda6` and confirmed three independent ways. Configuration here, never a literal in source. |
| NEX library version, update | 4.3.5 (`40305`), from `SDK MW+Nintendo+NEX-4_3_5-OL` in the update's `.rodata` | `earlier/arms` `main.go` |
| NEX library version, base game | 4.0.3 (`40003`) | `earlier/arms` `main.go` |
| Pia version | 5.7.0 in the update, 5.2.12 in the base game | `earlier/arms` `main.go` |
| Base title ID | `01009b500007c000`; update `01009B500007C800` | `earlier/arms` `main.go` |

`NEX_VERSION` is `4.3.5` because the update is what goes online. Whether the title logs in
with method 2 or method 6 is `servers/nex`'s open question 4 and is decided by that version,
not by anything here.

A wrong access key is the worst failure mode in this whole server: the PRUDP-Lite packet
signature simply does not match, the console retries forever, and neither side logs an error
(`earlier/arms` `main.go`). If ARMS reaches us and nothing at all happens, check this first.

## Protocols ARMS speaks

Protocol IDs from `pretendo-src`; which of them ARMS actually links from `earlier/arms`
`main.go` and `arms_stubs.go`.

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

Ranking: `earlier/arms` registers it but records that **no `RankingProtocolClient` is linked
in either ARMS build**, and that the `Ranking::*` strings in the binary are NEX error-name
table entries rather than evidence of a client. We leave it off. If a capture ever shows a
`0x70` call, `common_ranking.NewCommonProtocol` is three lines.

DataStore: linked by the ARMS **update** only; the base game has no DataStore client at all
(`earlier/arms` `arms_stubs.go`).

## Wire-shape facts, and which of them still matter

`earlier/arms` carries four environment knobs for wire details it could not confirm. Three of
them do not exist on nex-go, because nex-go already answers the way ARMS wants:

| Reference knob | Their finding | What we do |
| --- | --- | --- |
| `ARMS_STATION_SCHEME=prudps` | `prudps` makes the console treat the secure server as authenticated and send its kerberos ticket in CONNECT; with `prudp` CONNECT carries an empty payload and the title dies when Pia needs the session | `servers/nex` already builds the secure station URL with `constants.StationURLPRUDPS`. Nothing to set. |
| `ARMS_SECURE_MINOR=0` | the retail secure server answered PRUDP minor version 0 where earlier-nex defaulted to 5, and with 5 the console sends CONNECT with `plen=0` | Moot on nex-go: `prudp_endpoint.go` sets `ack.MinorVersion` from the client's own SYN — *"we can just support what the client wants"* (`pretendo-src`). |
| `ARMS_LEGACY_PIA=1` | ARMS's Pia 5.7.0 predates the 5.19 that introduced the `type=0x0B` + `Pa` public-station shape, so it wants the legacy `type=0x03`, no `Pa` | `pretendo-src`'s common SecureConnection builds the public station with `StationURLFlagPublic\|StationURLFlagBehindNAT` = `0x03`, which is the legacy shape already. Nothing to set. |
| `ARMS_DATASTORE_STUB=1` | their DataStore is an empty-list stub | Replaced by a real implementation. See `design.md`. |

Two more facts that came out of the same file and that we **do** act on:

- **Transport is WebSocket, not UDP.** `earlier/arms` serves both endpoints with
  `ListenSecure(port, cert, key)` — TLS WebSocket — and never opens a UDP socket. Since that
  server is recorded as fully operational for ARMS multiplayer, ARMS speaks PRUDP over
  WebSocket. `servers/nex`'s `Transport` defaults to `udp`, so `NEX_TRANSPORT=websocket` is
  the one non-default this title needs. TLS termination is the front proxy's.
- **The secure station URL shape agrees exactly.** `earlier/arms` builds it with
  `CID=1`, `PID=2` (their `securePID`), `sid=1`, `stream=10`, `type=2`, scheme `prudps`.
  `servers/nex` independently builds `SetConnectionID(1)`, `SetPrincipalID(2)`,
  `SetStreamID(1)`, `SetStreamType(constants.StreamTypeRVSecure)`, `SetType(2)`,
  `StationURLPRUDPS` — and `StreamTypeRVSecure` is the tenth member of nex-go's enum, so
  `stream=10` matches. Every default is already right for ARMS; nothing needs overriding.

Two facts from `earlier/arms` that we have **not** resolved and that a capture must settle:

- **Pia keepalive, PRUDP packet type 8.** Undocumented, empty payload, `NeedsAck`, roughly
  every three seconds on the secure stream. The NEX SDK's own types stop at `TYPE_RAW=7`, so
  an unhandled type-8 keepalive went un-ACKed and Super Smash Bros. Ultimate treated the
  connection as dead and soft-locked entering the online menu. `earlier/arms` registers an
  ACK handler for it and records that **whether ARMS's Pia 5.7.0 sends it at all is unknown**.
  nex-go's `packetHandlers` map has no entry for type 8 either. `RegisterCustomPacketHandler`
  is the hook if it turns out to be needed; we have not added one because we do not know that
  it is, and an unnecessary handler is a thing to maintain.
- **`SecureConnection::Register` (`0x0B.1`) versus `RegisterEx` (`0x0B.4`).** `pretendo-src`
  refuses plain `Register` unless `EnableInsecureRegister()` is called, and `servers/nex` does
  not expose that. See `design.md`.

## Identity

The earlier login path (`earlier/arms` `main.go`, `gates.go`) resolves a `nx2.`-prefixed
HMAC token, or a bare decimal PID, or an NSA id looked up against `openpak-account`, and
falls back to an FNV hash of the username for anonymous players. Its own comment records the
bare-PID branch as a known authentication hole: PIDs are sequential from 1800000001, so
sending someone else's number plays as them.

None of that is reproduced. OpenPak resolves identity in `servers/nex` through nx-baas's
`POST /internal/switch/identity`, which proves a BAAS id token's `openpak` claim, and this server
never sees a username. The friend graph comes back on the same call, which is what
`GetUserFriendPIDs` is wired to.

## Ports

`22140` (auth `22140`, secure `22141`, `/healthz` `22142`) is the next free NEX block, claimed
in `openpak` `ports.md`. the earlier server's `:443` auth and `:60006` secure are theirs and do not carry
over — but the fact that both are TLS WebSocket does, and is why `NEX_TRANSPORT=websocket`.

## Error codes

| Code | Meaning | Source |
| --- | --- | --- |
| `0x80690004` | `DataStore::NotFound`, facility 105 | `earlier/arms` `arms_stubs.go`; the same value is `nex.ResultCodes.DataStore.NotFound` in `pretendo-src`, which is what this server returns |
| `2618-562` | `SessionKeepFailed` — what ARMS shows when Pia cannot recognise itself in a session, e.g. because the NEX PID is not the PID the game knows itself by | `earlier/arms` `main.go` |

## Facts we did not need

Recorded so nobody goes looking for them twice: `earlier/arms` also carries a monitoring
dashboard, a presence reporter that reports "playing ARMS now" to their account service, a NAT
endpoint bridge file, and a per-title kerberos password shared between its two endpoints. The
first three are their operational shape rather than protocol facts. The fourth does not apply:
`servers/nex` generates both special accounts' passwords per process, because nothing outside
that process ever derives a key from them.
