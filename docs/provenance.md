# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape.

## Sources

| Tag | What |
| --- | --- |
| `earlier/lm3` | `~/REPOS/luigis-mansion-3`, the earlier network Luigi's Mansion 3 server (source only, read for facts). Its own README credits the implementation to @LITTLECHOPT8. Read, never copied. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.7, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `nx-baas`, `ports.md`, and the sibling `servers/mario-party-superstars` (the matchmaking-only worked template) |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| Title ID | `0100DCA0064A6000` | `earlier/lm3` `main.go` (`lm3AppID`) |
| NEX access key | `aab95246` | `earlier/lm3` `main.go` (`accessKey`, also the `LM3_ACCESS_KEY` default). 8 hex. Configuration here, never a literal in source. |
| Game Server ID | `0x20DE2100` | `earlier/lm3` `main.go` (`gameServerID`). **Not** the access key — see the trap below. |
| NEX library version | 4.0.0 (`40000`) | `earlier/lm3` `main.go` (`nexVersion = 40000`) |
| Pia version | 5.17 | `earlier/lm3` `main.go` comment (`// lm3 uses 5.17 pia`) |
| Secure PID | 2 | `earlier/lm3` `main.go` (`securePID = 2`) — the OpenPak default |

**The Game Server ID is not the access key.** Luigi's Mansion 3's game server ID is `0x20DE2100`
and its access key is `aab95246`; neither is derivable from the other. The ID is what appears in
the `g<hex>...srv.nintendo.net` hostname a console resolves, which makes it the value most likely
to be mistaken for a key. An access key is always 8 hex digits and comes from the game binary or
the public Game Server List. This mistake has been made before in the fleet, which is why both
values are written down here side by side. A wrong access key is the worst failure mode in the
whole server: the PRUDP packet signature simply does not match, the console retries forever, and
neither side logs an error.

## Confirmed Switch facts, and what this server sets

The per-title knobs `servers/nex` exposes, and the value this title needs for each. Where the
value is the `servers/nex` default it is called out, because a default that happens to be right
is still a fact that had to be checked.

| Knob | Value | Source / reasoning |
| --- | --- | --- |
| `NEX_ACCESS_KEY` | `aab95246` | above. Required, no default. |
| `NEX_VERSION` | `4.0.0` | `earlier/lm3` `nexVersion = 40000`. |
| `NEX_STATION_SCHEME` | `prudps` | `earlier/lm3` builds its secure station URL with `NewStationURL("prudps")`. This is also the `servers/nex` default; set explicitly by this server for the record. |
| `NEX_SECURE_STREAM_ID` | `1` (two-listener) | `earlier/lm3` runs two separate WSS listeners (`:443` auth, `:60009` secure) and its secure station URL carries `sid=1`. Both say the two-listener shape, stream 1 — the `servers/nex` default. The wrong value here logs `trying to connect to unbound PRUDPEndPoint`. |
| `NEX_SOURCE_KEY_BYTES` | `32` | `earlier/lm3` derives the source key with `sha256.Sum256(...)` and its `sessionKeyLen = 32`; all 32 bytes are handed over. The `servers/nex` default. |
| `NEX_SECURE_PID` | `2` | `earlier/lm3` `securePID = 2`. The `servers/nex` default. |
| `NEX_TRANSPORT` | `websocket` | `earlier/lm3` serves both endpoints with `ListenSecure` (TLS WebSocket) and opens no UDP socket. The `servers/nex` default. |

The secure station URL `earlier/lm3` builds — `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2`,
scheme `prudps` — matches field-for-field what `servers/nex` builds by default (the same match
recorded for the ARMS and Mario Party Superstars siblings). Nothing in the station URL needs
overriding.

### PRUDP minor version

`earlier/lm3` sets `secureSettings.PrudpMinorVersion = 0` on the secure endpoint. This is moot
on nex-go, which echoes the client's own SYN minor version rather than forcing one (the same
finding is recorded in the ARMS sibling's provenance). Nothing to set.

## Protocols Luigi's Mansion 3 speaks

Protocol IDs from `pretendo-src`; which of them `earlier/lm3` `main.go` actually registers, and
what this server does with each.

| Protocol | ID | Status here |
| --- | --- | --- |
| TicketGranting | `0x0A` | `servers/nex` |
| SecureConnection | `0x0B` | `servers/nex` — see the open question on Register vs RegisterEx |
| NATTraversal | `0x03` | registered (common) |
| MatchMaking | `0x15` | registered (common) |
| MatchMakingExt | `0x32` | registered (common) |
| MatchmakeExtension | `0x6D` | registered (common) |
| Utility | `0x6E` | registered (common), empty settings — see `design.md` |
| Ranking | `0x70` | **not** registered — see below |
| Station / presence | `0x79` | registered as an **acknowledge-only stub** — see below |
| DataStore | `0x73` | **not** registered — Luigi's Mansion 3 makes no DataStore call |

**Ranking (`0x70`).** `earlier/lm3` registers `nex.RankingHandler()`, but nothing observes the
game actually *calling* Ranking — the same situation the ARMS sibling recorded (registered
defensively, no client linked). A real `common_ranking` needs its own Postgres backing, so
building one on speculation is the wrong trade. Left off. If a capture shows a `0x70` call,
`common_ranking.NewCommonProtocol` plus a database is the add.

**Station / presence (`0x79`).** This is Luigi's Mansion 3's own protocol; there is no common
implementation of it. `earlier/lm3` handles four methods on it:

| Method | Name (per `earlier/lm3` comments) | Behaviour |
| --- | --- | --- |
| 5 | UpdateApplicationBuffer | publish-only; the retail server answers empty |
| 13 | UpdateState | publish-only ("here is my open room"); retail answers empty |
| 14 | InitializeStation | "I am connecting, who is here?" → a list of station/friend records |
| 15 | GetStationLocation | "where are my friends?" → a list of station/friend records |

This server registers a **stub**: it acknowledges all four (an unacknowledged `0x79` call would
get `Core::NotImplemented`, the answer most likely to hard-stall the session), answering 5 and 13
empty and 14/15 with an empty record list (`u32` count = 0). It publishes nothing and reports no
stations. The populated record layout was reverse-engineered by `earlier/lm3` from one retail
capture (its comments quote the observed bytes); OpenPak has taken no such capture of its own, so
reproducing that layout in the clean room is deferred until we have one. See `design.md`.

**MatchMaking 0x15 methods 1 and 3.** `earlier/lm3` intercepts these two and returns empty
success rather than letting its matchmaking handler answer them. Why it needs to is not
established, and the OpenPak stack (`common_match_making`) answers the standard methods itself, so
this server does not reproduce the interception. Recorded as a thing to watch if a first live run
misbehaves early in matchmaking.

## Identity and friends

`earlier/lm3` resolves identity itself: a signed `nx2.` HMAC token, a bare decimal PID (which
its own comment flags as an impersonation hole — emulator PIDs are sequential from 1800000000),
or an NSA id looked up against `openpak-account`, with an FNV hash of the username for anonymous
players. None of that is reproduced. OpenPak resolves identity in `servers/nex` through nx-baas's
internal identity API, which proves a BAAS id token, and the friend graph comes back on the same
call — which is what `host.FriendPIDs` (wired to the matchmaking manager here) reads.

The friend graph is what the game's "with friends" screen and friend-room joining are built on.
On OpenPak the standard matchmaking friend-only lobbies use it directly; the richer presence view
(a friend's open room shown before you join) is part of the deferred `0x79` work.

## Ports

`22190` (auth `22190`, secure `22191`, `/healthz` `22192`) is this title's block, claimed in
`openpak` `ports.md`. `earlier/lm3`'s `:443` auth and `:60009` secure are theirs and do not carry
over — but the fact that both are TLS WebSocket does, and is why the transport is `websocket`.

## Facts we did not need

Recorded so nobody goes looking for them twice: `earlier/lm3` also carries a large NAT-check /
STUN responder and a PRUDP station relay (`serveNNCS`, `nncsRelay`, `nncsSinkhole`) for punching
two firewalled NATs together, a monitoring dashboard, a presence reporter to their account
service, a signed-token identity gate with a leaked-token denylist, and a ZNC token HTTP handler
on `:443`. The NAT relay is their operational answer to peers with no port forwarding; on OpenPak
NAT traversal is `common_nat_traversal` plus whatever the deployment's own relay story is, not a
per-title concern baked into the game server. The identity gate does not apply — OpenPak proves a
BAAS token in `servers/nex`, and this server never sees a username. The dashboard, presence
reporter and ZNC handler are their operational shape rather than protocol facts.
