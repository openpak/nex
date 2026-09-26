# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape.

## Sources

| Tag | What |
| --- | --- |
| `earlier/minecraft` | `~/REPOS/minecraft`, the earlier network Minecraft (Switch) server (Shield-licensed). Read for facts only. |
| `earlier/nex` | `~/REPOS/earlier-nex`, their shared NEX core (Shield-licensed). Read for facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2`, `nex-protocols-go/v2`, `nex-protocols-common-go/v2`, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `f4983d87` | `earlier/minecraft` `main.go` records reading it out of the game binary at `0xddd877` **and verifying it cryptographically** against a real measured CONNECT signature — `LitePacketSignature(md5(key), md5(key)‖srvConnSig)` matches the wire byte for byte. Configuration here, never a literal in source. |
| NEX library version | `40000` (4.0.0) is the value `earlier/minecraft` `main.go` passes to `NewSwitchSettings`. Its own comment beside that constant instead calls the title "NEX 4.3.1". **Unresolved contradiction — see below.** | `earlier/minecraft` `main.go` |
| Secure PID | `2` (their `securePID`) | `earlier/minecraft` `main.go` |
| Source key length | 32 bytes (`sessionKeyLen`, and `earlier/nex` `NewSwitchSettings` sets `KerberosKeySize: 32`) | `earlier/minecraft` `main.go`, `earlier/nex` `settings.go` |

### The version contradiction

`earlier/minecraft` `main.go` sets `nexVersion = 40000` and passes it to `NewSwitchSettings`,
and `earlier/nex` uses that field directly (`if s.NexVersion >= 40000` gates the Switch
byte-stream profile). So the working server *runs* at 4.0.0. The comment two lines up says
"MC is NEX 4.3.1". Both cannot be the wire truth and this is recorded, not resolved.

It does not change what this server does. `openpak/nex` needs the version for two things: the
`>= 4.0.0` byte-stream profile (8-byte PIDs + structure header) and `nex-protocols-go`'s
routing to the Switch method names — both trigger at `>= 4.0.0`, which 40000 and 40301 (4.3.1)
both satisfy. `NEX_VERSION=40000` reproduces the value the working server actually ran. A
login capture's `apiVersionGeneral` is what would settle the prose-vs-code disagreement.

### Login method 2 vs 6

`openpak/nex` registers **both** `ValidateAndRequestTicketWithCustomData` (method 2, the pre-4.4
LoginEx shape) and `ValidateAndRequestTicketWithParam` (method 6), so it answers whichever the
console sends; this server needs no version branch. `earlier/nex` `ticketgranting.go` records
method 6 as the NEX 4.6+ shape (SSBU), which puts a 4.0.x/4.3.x title like Minecraft on method
2 — but since both are handled, a capture only confirms rather than decides anything here.

## Protocols Minecraft speaks

Protocol IDs from `pretendo-src`; which of them Minecraft actually registers is from
`earlier/minecraft` `main.go`, whose secure endpoint registers exactly these.

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
| DataStore | `0x73` | **not** registered — Minecraft's server never links it |

Minecraft is a session-join title: matchmaking (including friends-list narrowing), NAT
traversal so two consoles reach each other, and utility for NEX unique IDs. That is the whole
protocol set here.

### Ranking (0x70) — registered by Minecraft, not registered here

`earlier/minecraft` `main.go` *does* register a Ranking handler, unlike ARMS (whose port
left it off because no Ranking client is linked). But the handler it registers,
`earlier/nex` `ranking.go`, is **explicitly measured against Mario Kart 8 Deluxe** — its own
header comment says "Sur Mario Kart 8 Deluxe", and every fact in it (the 132-byte Mii profile
block starting `52 46 00 00`, `CommonDataByPIDs` 0x16, `SubmitScore` 0x11, `SubmitTime` 0x13,
tournament listings) is MK8D's ranking-as-profile-store behaviour, captured from MK8D traffic.
There is **no Minecraft-specific ranking fact** anywhere: no Minecraft payload shape, no
category, no evidence a Minecraft client calls 0x70 at all. Porting that handler would be
reproducing MK8D's ranking, which is fork, not facts.

Two independent reasons not to register it, on top of the missing facts:

1. `pretendo-src`'s common Ranking protocol ships no storage backing; every callback
   (`GetRankingsAndCountByCategoryAndRankingOrderParam`, `InsertRankingByPIDAndRankingScoreData`,
   …) is the operator's to supply. With them unset, the handler returns `Core::NotImplemented`
   — the *same* answer the endpoint gives for a protocol that was never registered. Registering
   it bare would add only a warning log line and change no behaviour a console sees.
2. Inventing an MK8D-shaped backing to fill those callbacks is precisely what the clean-room
   policy forbids and what the team scope ("register none and say so; don't invent") rules out.

So Ranking is left off, exactly as ARMS's is, and this is the open question a Minecraft capture
must answer: **does a Minecraft client ever send a `0x70` call, and if so with what data?**
Until one does, there is nothing here to implement that would not be a guess.

## Wire-shape facts

| Fact | Value for Minecraft | Source | What we do |
| --- | --- | --- | --- |
| Secure station URL scheme | **`prudp`** | `earlier/minecraft` `main.go`: `nex.NewStationURL("prudp")` | `NEX_STATION_SCHEME=prudp`. **Differs from most fleet titles** (ARMS uses `prudps`). See below. |
| Listener shape | two listeners, secure stream 1 | `earlier/minecraft` `main.go` runs `:443` auth + `:60006` secure and the secure station URL carries `sid=1` | `NEX_SECURE_STREAM_ID=1` (the `servers/nex` default) |
| Transport | WebSocket (WSS) | `earlier/minecraft` `main.go` serves both endpoints with `ListenSecure` and opens no UDP socket; `earlier/nex` `NewSwitchSettings` sets `Transport: TransportWebSocket` | `NEX_TRANSPORT=websocket` (the default) |
| Secure station URL fields | `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | `earlier/minecraft` `main.go` | `servers/nex` builds exactly these by default (`stream=10` = `StreamTypeRVSecure`, the tenth nex-go enum member); nothing to override |

### The station scheme is the top thing to verify on hardware

ARMS's port recorded, from ARMS's own working server, that `prudps` is what makes a console
treat the secure server as authenticated and send its kerberos ticket in CONNECT, and that
`prudp` there made CONNECT carry an empty payload and the title die when Pia needed the
session. **Minecraft's working server uses `prudp`** and is a live, operational server. These
two facts contradict each other across titles — which the clean-room policy calls a finding,
not a problem to resolve before writing. This server matches Minecraft's observed value
(`prudp`) because that is the one attested to work *for Minecraft*, but the scheme is the
classic silent-later-failure knob (menus keep working, the title gives up when it needs the
session), so a hardware run is what confirms it. `NEX_STATION_SCHEME` flips it in one line.

## Identity and friends

`servers/nex` resolves a console to an OpenPak account through nx-baas's internal identity API
and hands back the player's friend graph. `host.FriendPIDs` is assigned to the matchmaking
manager's `GetUserFriendPIDs`, which is the friends-list lobby path: a friends-only search is
the ordinary matchmaking search narrowed to gatherings owned by PIDs the player knows. None of
`earlier/minecraft`'s own identity code (its `nx2.` HMAC tokens, bare-PID branch, NSA lookup,
FNV anonymous fallback in `main.go`/`gates.go`) is reproduced — that is the earlier server's account
integration, and OpenPak's is nx-baas.

A Switch logs in under its **NSA id**, not its PID (`servers/nex` `login.go` records this).
None of the matchmaking callbacks here is given a login name — every one takes a PID — so there
is nothing to translate today. `Host.PIDForLoginName` is the hook if a capture ever shows a
Minecraft matchmaking call naming a player by login name instead.

## Ports

`22310` (auth `22310`, secure `22311`, `/healthz` `22312`), the reserved NEX block from
`openpak` `ports.md`. the earlier server's `:443` auth and `:60006` secure are theirs and do not carry
over; the fact that both are TLS WebSocket does, and is why the transport is websocket.

## Facts we did not need

Recorded so nobody looks for them twice: `earlier/minecraft` also carries a monitoring
dashboard (`dashboard.go`), a dead-connection reaper, a presence reporter, PROXY-protocol
support, and the online gates in `gates.go` (verified-email, one-place-at-a-time, account
required). The first four are their operational shape rather than protocol facts —
OpenPak answers "is it up" with `/healthz` and "may this account go online" belongs to the
account/nx-baas side. The gates are the earlier server's account policy, not Minecraft's protocol.
