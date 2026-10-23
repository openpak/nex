# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[clean-room policy](https://docs.openpak.org/contributors/clean-room/): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape.

## Sources

| Tag | What |
| --- | --- |
| `notes` | Earlier research notes on this title. Facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2`, `nex-protocols-go/v2`, `nex-protocols-common-go/v2`, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `f4983d87` | `notes` record reading it out of the game binary at `0xddd877` **and verifying it cryptographically** against a real measured CONNECT signature — `LitePacketSignature(md5(key), md5(key)‖srvConnSig)` matches the wire byte for byte. Configuration here, never a literal in source. |
| NEX library version | `40000` (4.0.0) is the value the notes ran with. A note beside it instead calls the title "NEX 4.3.1". **Unresolved contradiction — see below.** | `notes` |
| Secure PID | `2` | `notes` |
| Source key length | 32 bytes (Kerberos key size 32 in the Switch profile) | `notes` |

### The version contradiction

The notes set the NEX version to 40000, and that value gates the Switch
byte-stream profile (`>= 40000`). So earlier research *ran* at 4.0.0. A comment beside it says
"MC is NEX 4.3.1". Both cannot be the wire truth and this is recorded, not resolved.

It does not change what this server does. `openpak/nex` needs the version for two things: the
`>= 4.0.0` byte-stream profile (8-byte PIDs + structure header) and `nex-protocols-go`'s
routing to the Switch method names — both trigger at `>= 4.0.0`, which 40000 and 40301 (4.3.1)
both satisfy. `NEX_VERSION=40000` reproduces the value earlier research actually ran. A
login capture's `apiVersionGeneral` is what would settle the prose-vs-code disagreement.

### Login method 2 vs 6

`openpak/nex` registers **both** `ValidateAndRequestTicketWithCustomData` (method 2, the pre-4.4
LoginEx shape) and `ValidateAndRequestTicketWithParam` (method 6), so it answers whichever the
console sends; this server needs no version branch. The notes record
method 6 as the NEX 4.6+ shape (SSBU), which puts a 4.0.x/4.3.x title like Minecraft on method
2 — but since both are handled, a capture only confirms rather than decides anything here.

## Protocols Minecraft speaks

Protocol IDs from `pretendo-src`; which of them Minecraft actually registers is from
the notes, which register exactly these on the secure endpoint.

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
| DataStore | `0x73` | **not** registered — no DataStore client is linked |

Minecraft is a session-join title: matchmaking (including friends-list narrowing), NAT
traversal so two consoles reach each other, and utility for NEX unique IDs. That is the whole
protocol set here.

### Ranking (0x70) — registered by Minecraft, not registered here

The notes *do* register a Ranking handler, unlike ARMS (whose port
left it off because no Ranking client is linked). But that handler
is **explicitly measured against Mario Kart 8 Deluxe**, and every fact in it (the 132-byte Mii profile
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
| Secure station URL scheme | **`prudp`** | `notes`: the secure station URL uses `prudp` | `NEX_STATION_SCHEME=prudp`. **Differs from most fleet titles** (ARMS uses `prudps`). See below. |
| Listener shape | two listeners, secure stream 1 | `notes`: `:443` auth + `:60006` secure, and the secure station URL carries `sid=1` | `NEX_SECURE_STREAM_ID=1` (the `servers/nex` default) |
| Transport | WebSocket (WSS) | `notes`: both endpoints are served as TLS WebSocket and no UDP socket is opened | `NEX_TRANSPORT=websocket` (the default) |
| Secure station URL fields | `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | `notes` | `servers/nex` builds exactly these by default (`stream=10` = `StreamTypeRVSecure`, the tenth nex-go enum member); nothing to override |

### The station scheme is the top thing to verify on hardware

ARMS's port recorded, from earlier ARMS research, that `prudps` is what makes a console
treat the secure server as authenticated and send its kerberos ticket in CONNECT, and that
`prudp` there made CONNECT carry an empty payload and the title die when Pia needed the
session. **Earlier Minecraft research used `prudp`** against a live, operational server. These
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
the notes' own identity code (HMAC tokens, bare-PID branch, NSA lookup, anonymous fallback)
is reproduced; OpenPak's is nx-baas.

A Switch logs in under its **NSA id**, not its PID (`servers/nex` `login.go` records this).
None of the matchmaking callbacks here is given a login name — every one takes a PID — so there
is nothing to translate today. `Host.PIDForLoginName` is the hook if a capture ever shows a
Minecraft matchmaking call naming a player by login name instead.

## Ports

`22310` (auth `22310`, secure `22311`, `/healthz` `22312`), the reserved NEX block from
`openpak` `ports.md`. The earlier `:443` auth and `:60006` secure do not carry
over; the fact that both are TLS WebSocket does, and is why the transport is websocket.
