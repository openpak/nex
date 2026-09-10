# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape.

## Sources

| Tag | What |
| --- | --- |
| `earlier/strikers` | `~/REPOS/mario-strikers`, the earlier network Mario Strikers server (source-available). Read for facts only. |
| `earlier-nex` | `~/REPOS/earlier-nex`, the earlier network closed NEX core. Read for facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.7, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `servers/arms`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `MAGNI5jG` | `earlier/strikers` `main.go` records it extracted from the game binary. **It is an 8-character ASCII string, not hex** — unlike ARMS's `b6b34c51`. Configuration here, never a literal in source. |
| Game Server ID | `0x26CFAF00` | `earlier/strikers` `main.go`. **Not** the access key — see the trap below. |
| NEX library version | 4.0.0 (`40000`) | `earlier/strikers` `main.go` `nexVersion = 40000` |
| securePID | `2` | `earlier/strikers` `main.go` — the conventional value; openpak/nex's default |
| Source key | 32 bytes, `sha256("legacy-src:" + username)` | `earlier/strikers` `main.go` `resolveUser`. The length is the fact (Switch kerberos key size); the derivation is theirs and is not reproduced. openpak/nex's `NEX_SOURCE_KEY_BYTES` default of 32 matches. |
| Lineage | forked from the SSBU server (both Pia NEX titles) | `earlier/strikers` `main.go` |

`NEX_VERSION` is `4.0.0`. Whether the title logs in with method 2 or method 6 is decided by
that version, and is `servers/nex`'s open question, not settled here — evidence needed is the
RMC method ID of the first login.

**The Game Server ID is not the access key.** Strikers' game server ID is `0x26CFAF00` and its
access key is `MAGNI5jG`; neither is derivable from the other. The ID is what appears in the
`g26cfaf00-lp1.s.n.srv.nintendo.net` hostname a console resolves, which makes it the thing most
likely to be mistaken for a key. An access key here is an 8-character string that nex-go MD5s
internally; it comes from the game binary. This mistake has been made before in the fleet,
which is why both values are written down side by side.

A wrong access key is the worst failure mode in this whole server: the PRUDP-Lite packet
signature simply does not match, the console retries forever, and neither side logs an error.
If Strikers reaches us and nothing at all happens, check this first.

**The access key is a non-hex string, and that is load-bearing.** openpak/nex passes
`NEX_ACCESS_KEY` straight to nex-go's `server.AccessKey` (`servers/nex` `host.go`), which MD5s
whatever string it is given — no hex decode — so `MAGNI5jG` needs no host change. Confirmed by
reading `host.go`: `server.AccessKey = h.Config.AccessKey`. A `servers/nex` that ever starts
hex-decoding the key would break this title; recorded so that never happens silently.

## Protocols Strikers speaks

Protocol IDs from `pretendo-src`; which of them Strikers links from `earlier/strikers`
`main.go`, `strikers_init.go`, and `strikers_club.go`.

| Protocol | ID | `earlier/strikers` status | Status here |
| --- | --- | --- | --- |
| TicketGranting | `0x0A` | auth endpoint | `servers/nex` |
| SecureConnection | `0x0B` | secure endpoint | `servers/nex` |
| NATTraversal | `0x03` | real handler | registered (common) |
| MatchMaking | `0x15` | real handler | registered (common) |
| MatchMakingExt | `0x32` | real handler | registered (common) |
| MatchmakeExtension | `0x6D` | common + game-specific club methods | registered (common); club methods **deferred** |
| Utility | `0x6E` | empty-list stub | registered (common) |
| Ranking | `0x70` | registered (`nex.RankingHandler()`) | **not** registered — see below |
| DataStore | `0x73` | empty-list stub, `0x73.8` → NotFound | registered (common, empty Postgres store) |
| MatchmakeReferee | `0x78` | zero-struct / empty-list stub | **not** registered — see below |
| Subscriber | `0x79` | empty-list stub (0x79.14 GetFriendUserStatuses) | **not** registered — see below |
| Ranking2 | `0x7A` | zero-struct / void / empty-list stub | **not** registered — see below |

### What is registered, and why that set

The registered set is exactly the ARMS set (MatchMaking, MatchMakingExt, MatchmakeExtension,
NATTraversal, Utility, DataStore): the protocols with a real `nex-protocols-common-go`
implementation, wired the same proven way. That is the confirmed, non-guessed substrate — real
login, real matchmaking, real NAT traversal, real NEX unique IDs. It builds and it is
framework-backed rather than reverse-engineered.

### DataStore is registered, though Strikers does not store in it

`earlier/strikers` answers every `0x73` call with an empty list and `0x73.8` with
`DataStore::NotFound` (`0x80690004`) — a probe stub, not a store. Strikers' real online data
(clubs, roster) travels over **MatchmakeExtension** (`0x6D` game-specific methods), not
DataStore. So strictly, Strikers has no DataStore storage.

It is registered anyway, over an empty Postgres store (the ARMS pattern), because:

- The game issues `0x73` init calls; leaving `0x73` unregistered answers them
  `Core::NotImplemented`, and `earlier/strikers` bothered to register a stub rather than let
  that happen — evidence that NotImplemented on init is bad in this engine.
- The common DataStore over an empty store returns the same empty list / NotFound the stub
  did, but through the real, reviewed handlers.
- It reuses our own `servers/arms` DataStore code verbatim (schema `strikers`, S3 key bases),
  the agreed lift-candidate for the shared `github.com/openpak/nex/datastore` subpackage. The
  shared subpackage is **not built yet**; this is registered directly, and should collapse into
  the shared store on convergence (the two servers now carry identical DataStore code save the
  schema/key-base strings).

If a capture ever confirms Strikers never touches a DataStore object, this could drop to a bare
stub. Until then the empty store is the lower-risk, fleet-uniform choice. See `design.md`.

### What is deferred, and why

`Ranking` (`0x70`), `MatchmakeReferee` (`0x78`), `Subscriber` (`0x79`) and `Ranking2` (`0x7A`)
have **no** `nex-protocols-common-go` implementation, and the Strikers-specific MatchmakeExtension
methods (`0x6D` method 54+: create/join/list club, roster, online-init structs) are above the
common set (which tops out at method 53). `earlier/strikers` answers all of these with
hand-rolled stubs that its own source marks unconfirmed — "no measured reference", "TODO fill
real fields", "zeros only parse". They are capture-driven work, not settled facts, so they are
**not** ported blind:

- **Ranking (`0x70`).** `common_ranking.NewCommonProtocol` needs the operator to supply a full
  leaderboard backend (score insert, category queries, common-data blobs). Strikers registers
  the protocol, but what categories and score shapes it uses online is unknown, and inventing a
  Postgres leaderboard for a title whose leaderboard shape is unobserved is speculation. Register
  it with a real backend once a `0x70` capture shows the categories and `RankingScoreData`.
- **MatchmakeReferee `0x78`, Subscriber `0x79`, Ranking2 `0x7A`.** No common implementation
  exists. `earlier/strikers` registers per-method struct/list/void stubs it derived by reversing
  the game's decoder tables (no measured bytes). That per-method classification is unconfirmed
  and is close to their expression; recorded as facts here, not reproduced. A generic
  "empty list for everything" fallback is actively wrong for the struct-returning methods (it is
  one byte short of a struct header → `Core::BufferOverflow`), which is why no blanket stub is
  added either.
- **MatchmakeExtension club methods.** Method IDs recorded below as facts. The byte-exact `Club`
  structure, the roster layout, and the struct/list/bool-list per-method classification are
  `earlier/strikers`'s reverse-engineering of the game binary; the method *numbers* are facts,
  the elaborate handler *expression* is theirs and is not carried across.

The consequence, stated plainly: with only the common substrate registered, a real Strikers
console will reach login and standard matchmaking, then hit `Core::NotImplemented` the first
time it calls a club method or one of the no-common protocols. Full online play is the next
round's work, driven by OpenPak captures. This is the same measured-then-implement method
`earlier/strikers` itself used.

## Strikers-specific method IDs (facts, for the next round)

Recorded from `earlier/strikers` so a capture round has them to hand. These are protocol/method
numbers — facts about what the game calls — not their implementations.

| Protocol.method | What `earlier/strikers` calls it | Response kind it inferred |
| --- | --- | --- |
| `0x6D.55` (0x37) | create club | single Club struct (decoder 0x15970) |
| `0x6D.56/58/59` (0x38/3a/3b) | update / join / get one club | single Club struct |
| `0x6D.57/60/71/73` | find / list clubs | `list<Club>` (decoder 0x1f534) |
| `0x6D.62/65` | online-init struct | single struct (decoder 0x1b930) |
| `0x6D.76/80` (0x4c/0x50) | other single-struct methods | single struct |
| `0x6D.83` (0x53) | club member roster | member list (decoder 0x16090) |
| `0x6D.84–89` | (club queries) | `bool + list<u32>` (decoder 0x195f0) |
| `0x73.8` | DataStore GetMeta-style lookup | answered `NotFound` `0x80690004` |
| `0x6E.10/11`, `0x6E.14/15` | Utility | 10/11 struct; 14/15 void |
| `0x78.1` | MatchmakeReferee | scalar `u64` |
| `0x78.2/3/4/7/12/14` | MatchmakeReferee | single struct |
| `0x7A.9/11/12/14` | Ranking2 | single struct; `0x7A.13` void |
| `0x79.14` | Subscriber GetFriendUserStatuses | empty list |

`earlier/strikers` also notes a Pia 5.x **type-8 keepalive** it ACKs so the online menu does
not soft-lock, and a `PrudpMinorVersion = 0` set on its secure endpoint. Both are moot on
nex-go for the same reasons recorded in `servers/arms`'s provenance: nex-go answers the client's
own SYN minor version, and has no type-8 handler that Super Smash Bros. Ultimate needed only
because the Reference core lacked one. `RegisterCustomPacketHandler` is the hook if a Strikers
capture shows a type-8 packet actually arriving.

## Wire-shape facts (secure station URL)

`earlier/strikers` `main.go` builds the secure station URL with scheme `prudps`, `CID=1`,
`PID=2`, `sid=1`, `stream=10`, `type=2` (public). `servers/nex` independently builds exactly
this from its defaults (`StationURLPRUDPS`, `SetConnectionID(1)`, `SetPrincipalID(2)`,
`SetStreamID(1)`, `SetStreamType(StreamTypeRVSecure)` — the tenth enum member, so `stream=10` —
`SetType(2)`). Every default is already right for Strikers; nothing needs overriding. The
`sid=1` also confirms the **two-listener** shape (auth on `22300`, secure on `22301`), so
`NEX_SECURE_STREAM_ID` stays `1`.

**Transport is WebSocket.** `earlier/strikers` serves both endpoints with `ListenSecure`
(TLS WebSocket) and opens no UDP socket. `servers/nex`'s `Transport` default is now `websocket`,
so nothing to set. TLS termination is the front proxy's.

## Identity

`earlier/strikers`'s login path resolves an `nx2.`-prefixed HMAC token, a bare decimal PID, or
an NSA id, and its own comments record the bare-PID branch as an authentication hole (sequential
PIDs from 1800000001). None of that is reproduced: openpak resolves identity in `servers/nex`
through nx-baas's `POST /internal/switch/identity`, which proves a BAAS id token's `openpak` claim,
and this server never sees a username. `userName` at login is the console's NSA id, not the PID —
recorded as a confirmed Switch fact. The friend graph comes back on the same call, which is what
`host.FriendPIDs` (assigned to the matchmaking manager and to DataStore) is wired to.

## Ports

`22300` (auth `22300`, secure `22301`, `/healthz` `22302`) is this title's NEX block, reserved
in `openpak` `ports.md`. the earlier server's `:443` auth and `:60008` secure are theirs and do not carry
over — but the fact that both are TLS WebSocket does.

## Error codes

| Code | Meaning | Source |
| --- | --- | --- |
| `0x80690004` | `DataStore::NotFound`, facility 105 | `earlier/strikers` `strikers_init.go`; the same value is `nex.ResultCodes.DataStore.NotFound` in `pretendo-src`, which is what this server returns from an empty store |
| `0x80010002` | `Core::NotImplemented` | what an unregistered protocol/method answers; `earlier/strikers` registers stubs specifically to avoid it on `0x73`/`0x78`/`0x7A` |
| `0x8001000F` | `Core::BufferOverflow` | `earlier/strikers` records this as what a struct-returning method answered with an empty list produces — why no blanket empty-list stub is safe |
| `2618-562` | `SessionKeepFailed` — Pia cannot recognise itself in a session, e.g. because the NEX PID is not the PID the game knows itself by | `earlier/strikers` `main.go` |

## Facts we did not need

Recorded so nobody looks for them twice: `earlier/strikers` also carries a monitoring dashboard
(`dashboard.go`), a connection reaper, a proxy-protocol mode for a shared `:443`, an in-memory
club store persisted to `clubs.json`, and a per-title kerberos password shared between its two
endpoints. The first four are their operational shape rather than protocol facts. The last does
not apply: `servers/nex` generates both special accounts' passwords per process, because nothing
outside that process ever derives a key from them.

## `servers/nex` changes needed

None. Every Strikers config value maps onto a `servers/nex` default or an existing env knob: the
ASCII access key is passed through unchanged, `NEX_VERSION=4.0.0`, `NEX_SECURE_STREAM_ID=1`
(two-listener), `prudps`, secure PID 2, 32-byte source key, websocket transport. The one thing a
future capture could force is exposing `EnableInsecureRegister()` if Strikers turns out to call
`SecureConnection::Register` (`0x0B.1`) rather than `RegisterEx` (`0x0B.4`) — the same open
question `servers/arms` records, unresolved for Strikers too. Evidence needed: the method ID of
the first `0x0B` call.
