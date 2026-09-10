# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape. The DataStore backend is adapted from the OpenPak
sibling `servers/arms` (our own AGPL-3.0 code, a designated shared lift candidate), not from
any Shield-licensed source.

## Sources

| Tag | What |
| --- | --- |
| `earlier/acnh` | `~/REPOS/animal-crossing-new-horizons`, the earlier network ACNH server (source-available). Read for facts only. |
| `earlier-nex` | `~/REPOS/earlier-nex`, the earlier network NEX core, notably `matchmaking_acnh.go` (Shield). Read for facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) and its MIT `nintendo/nex/matchmaking.py` |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.7, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `servers/arms`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `v43a10em` | `earlier/acnh` `main.go` `accessKey`. **An 8-character string, not hex** — unlike its siblings it contains `v`, `e`, `m`. The same file cites MK8 `09c1c475`, Splatoon 2 `4eb18d39`, Smash `9587602b`, values that match those recorded independently in `openpak` `servers/arms`, which is the cross-check that the author had the real keys. Configuration here, never a literal in source. |
| Game Server ID | `2EE2E300` | `earlier/acnh` `main.go` comment (`ACNH game server 2EE2E300`). **Not** the access key — see the trap below. |
| NEX library version | 4.0.0 (`40000`) | `earlier/acnh` `main.go` `nexVersion`. Passed as `NEX_VERSION=40000`. |
| Secure PID | `2` | `earlier/acnh` `main.go` `securePID`. The `openpak/nex` default, `NEX_SECURE_PID=2`. |
| Base title ID / application ID | `01006f8002326000` | `earlier/acnh` `main.go` `acnhAppID`. Account-side (presence); this server does not use it. Recorded so it is not hunted for twice. |

**The Game Server ID is not the access key.** ACNH's game server ID is `2EE2E300` and its
access key is `v43a10em`; neither is derivable from the other. The ID is what appears in the
`g<hex>-lp1.s.n.srv.nintendo.net` hostname a console resolves, which makes it the thing most
likely to be mistaken for a key. An access key is an 8-character string and comes from the
public Game Server List, from the game binary, or from brute-forcing a captured PRUDP-Lite pair
against `HMAC-MD5(MD5(key), MD5(key)+connectionSig)`. This mistake has already been made once in
the fleet, which is why both values are written down here side by side.

A wrong access key is the worst failure mode: the PRUDP-Lite packet signature does not match,
the console retries forever, and neither side logs an error (`earlier/acnh` `main.go`, and the
same is recorded for the sibling titles in `openpak` `servers/arms`). If ACNH reaches us and
nothing at all happens, check this first — and check it is `v43a10em`, not the GSID.

## Wire-shape facts

`openpak/nex` reads each of these from the environment; the value below is the one this title
needs and why.

| Knob | Value | Source and reasoning |
| --- | --- | --- |
| `NEX_STATION_SCHEME` | `prudps` | `earlier/acnh` `main.go` builds the secure station with `NewStationURL("prudps")` and warns at length that `prudp` makes CONNECT arrive empty and the title die when Pia needs the session ("ce piège a coûté des jours sur Smash"). `openpak/nex` default. |
| `NEX_SECURE_STREAM_ID` | `1` (two-listener) | `earlier/acnh` runs two WSS listeners (auth `:443`, secure `:60007`) and its secure station URL carries `sid=1`, `stream=10`, `type=2`, `CID=1`, `PID=2`. Both say two-listener, stream 1. `openpak/nex` builds the identical shape by default (`SetStreamID(1)`, `StreamTypeRVSecure` is the 10th enum member → `stream=10`, `SetType(2)`). Nothing to set. |
| `NEX_SOURCE_KEY_BYTES` | `32` | `earlier/acnh` `main.go` `resolveUser`: `sha256.Sum256(...)` → 32 bytes, commented "DOIT faire 32 octets (taille de clé Kerberos Switch)". `openpak/nex` default. The invariant that matters is that the key the ticket is encrypted with equals the key advertised; `openpak/nex` generates a fresh random 32-byte key per login and advertises it. |
| `NEX_TRANSPORT` | `websocket` | `earlier/acnh` serves both endpoints with `ListenSecure(port, cert, key)` — TLS WebSocket — and opens no UDP socket. Since that server is operational for ACNH, ACNH speaks PRUDP over WebSocket. `openpak/nex` default. |
| PRUDP minor version | not set | `earlier/acnh` forces `PrudpMinorVersion = 0` (from `OPENPAK_PRUDP_MINOR`, default 0) because its core imposed a value; it notes the earlier stack it replaced simply echoed the client's own value. `pretendo-src` `prudp_endpoint.go` echoes the client's SYN minor version, so this is moot on nex-go — nothing to set. |
| Login method | 2 or 6, decided by version | The protocol has one method-ID set with two names (`kinnay`, Authentication Protocol): id 2 `ValidateAndRequestTicketWithCustomData`, id 6 `ValidateAndRequestTicketWithParam`. `openpak/nex` registers **both** and nex-protocols-go routes by library version, so either works. At 4.0.0 the console is expected to call method 2 (WithParam/method 6 is the 4.4.0+ shape). Confirm from the first login's RMC method ID. |

## Protocols ACNH speaks

Protocol IDs from `pretendo-src`; which of them ACNH links, and how it uses them, from
`earlier/acnh` `main.go` and `earlier-nex`.

| Protocol | ID | Status here |
| --- | --- | --- |
| TicketGranting | `0x0A` | `openpak/nex` |
| SecureConnection | `0x0B` | `openpak/nex` — but see the public-station gap below |
| NATTraversal | `0x03` | registered (common) |
| MatchMaking | `0x15` | registered (common) |
| MatchMakingExt | `0x32` | registered (common) |
| MatchmakeExtension | `0x6D` | registered (common) — **two ACNH methods are not covered, see gaps** |
| Utility | `0x6E` | registered (common, methods 1–8) — **methods 9 and 10 are not covered, see gaps** |
| Ranking | `0x70` | **not** registered — see gaps |
| DataStore | `0x73` | registered, real implementation (Postgres + presigned S3) |

## Gaps — what the common libraries do not cover

Verified by reading `pretendo-src` `nex-protocols-common-go@v2.6.1`. `earlier/acnh` and
`earlier-nex` implemented each of these; the shared OpenPak host does not yet, so they answer
`Core::NotImplemented` today rather than a wrong guess. Each is a candidate for a custom handler
here or (better) for convergence into `openpak/nex`, and each needs a live capture to pin down.

### 1. MatchmakeExtension::FindMatchmakeSessionByParticipant (`0x6D.0x33`) — friend visits

This is the core of ACNH online: to visit a friend's island the game locates the friend via
`0x6D.0x33` (`earlier-nex` `matchmaking_acnh.go`). Common `matchmake-extension@v2.6.1` has no
handler for it (its file list ends at `join_matchmake_session_with_param`, `update_*`,
`get_*_community`, `get_playing_session`; no `find_matchmake_session_by_participant`). Wire
shape, from `kinnay` `matchmaking.py` (which `earlier-nex` also cites as its only source):
request `FindMatchmakeSessionByParticipantParam { list<PID> PrincipalIDs; u32 Options;
MatchmakeBlockListParam BlockList }`; response is a list of **pairs**
`{ PID PrincipalID; MatchmakeSession Session }` — not bare sessions (a bare-session response
shifts the client's read and ACNH shows 2306-0116). The whole session must be returned,
`ApplicationBuffer` and `MatchmakeParam` included, or the visitor is rejected. `earlier-nex`
records from a reference-server capture that the 32-byte session key is present in this reply
(Find/51 = 481 bytes, 449 without it) and that ACNH's Pia uses it to seed the P2P session
**before** JOIN — omit it and the visit stalls at "Getting ready to depart" (2618-0502).

### 2. MatchmakeExtension::UpdateMatchmakeSession (`0x6D.0x2C`) — writing a Dodo Code

ACNH sends the host's five-character Dodo Code **only** via a partial session update
(`0x6D.0x2C`), and nowhere else (`earlier-nex` `matchmaking_acnh.go`). Common
`matchmake-extension@v2.6.1` has no `update_matchmake_session` handler. **Partial mitigation
confirmed:** the common library *does* store and search on the code word when it is set at
`CreateMatchmakeSession` time — `create_matchmake_session.go` writes the `codeword` column and
`find_matchmake_session_by_search_criteria.go` filters `ms.codeword=$2`. So if ACNH ever set the
code at create time, browse-by-code would work unaided. The open question is whether ACNH sets
it at create or only via `0x2C`; the facts say `0x2C`, so treat Dodo-code hosting as not working
until a capture proves otherwise. Wire shape of `UpdateMatchmakeSessionParam` and the flag bits
(`OpenParticipation = 0x2`, `Codeword = 0x4000`; a real code arrives with `flags = 0x4002`) are
in `kinnay` `matchmaking.py` and `earlier-nex` `matchmaking_acnh.go`.

### 3. Utility 9 / 10, Ranking 25

`earlier/acnh` `utility.go` overlays two non-standard Utility methods on the core handler
(base NEX Utility stops at method 8): `0x6E.9 AcquireTagId(list<u64>) -> u64` (a non-zero id
derived from the caller PID suffices) and `0x6E.10 UpdateCurrentUser(param) -> void` (answered
with an empty success — a real Splatoon 2 capture showed 0 bytes against a static analysis that
guessed ~30, and "la mesure a tranché"). `earlier/acnh` also overlays `0x70.25
GetEventMatchResult` on Ranking, answered with an empty list. Common `utility@v2.6.1` implements
only methods 1–8; common `ranking@v2.6.1` implements `get_ranking`, `get_cached_top_x_ranking(s)`,
`upload_score`, `upload_common_data`, `get_common_data`, none of them method 25. **Ranking is
left unregistered** here (its only known ACNH call is the unimplemented method 25, so registering
common ranking would need a leaderboard backend and still not answer it); which standard ranking
methods, if any, ACNH also calls is unknown and a capture must say.

### 4. Public P2P station shape — an `openpak/nex` change

`earlier/acnh` registers SecureConnection with `SwitchPia519Config()` so a peer's **public**
station is advertised as `type=0x0B` (`Public|BehindNAT|Switch`, the Pia 5.19+ shape) with a
`Pa` public address, and records that the Wii-U-era `type=0x03` breaks ACNH: its Pia does not
recognise the peer's public station, the hole-punch does not build, and the visit stalls at
"Getting ready to depart" (2618-0502) even though the server-side punch succeeds. **`openpak/nex`
cannot currently produce `0x0B`:** its SecureConnection is Pretendo's common one, and
`pretendo-src` `secure-connection/register_ex.go:92` hardcodes `publicStation.SetType(
StationURLFlagPublic | StationURLFlagBehindNAT)` = `0x03` (nex-go `constants` has only
`BehindNAT=1`, `Public=2`). This is the sibling `servers/arms`'s "nothing to set" *inverted*:
`0x03` is right for arms's Pia 5.7.0 and wrong for ACNH's Pia 5.x. **`openpak/nex` needs a way to
advertise the Pia-5.19 public-station flag** (a per-title toggle on the shared SecureConnection,
or replacing the common register handlers). Recorded here as the concrete change the next round
needs; see `docs/design.md`.

## DataStore

`earlier-nex` has **no** DataStore implementation, so `earlier/acnh` `main.go`
(`setupDataStoreDiscovery`) runs `0x73` in "discovery" mode — it logs each call's method and
parameter length and answers empty-success — because ACNH's DataStore usage was never mapped.
Its own comment: replacing that with `NotImplemented` "couperait ce qui marche aujourd'hui."

This server implements the real thing instead, over `pretendo-src`
`nex-protocols-common-go/v2/datastore` backed by Postgres (metadata) and presigned S3 (object
bodies), adapted from `openpak` `servers/arms` (whose `datastore.go`/`search.go` are a
title-agnostic transcription of NEX's `DataStoreMetaInfo`). The only ACNH-specific values are
the schema name `acnh` and the S3 key bases `acnh/objects` / `acnh/notifications`.

Because ACNH's `0x73` usage is unmapped, a first live capture must record which methods it calls
and with what data types and tags. A method the common library does not model will now return a
NEX error rather than the old empty-success; whether that regresses anything real is unknown
until the capture exists. See `docs/design.md`.

## Identity and friends

`earlier/acnh` (`main.go` `resolveUser`, `gates.go`, `friends.go`) resolves an `nx2.`-signed
token, a bare decimal PID, or a real-Switch NSA id (via `/api/nsa`) to an account PID, with a
stack of online gates, and pulls the friend graph from `openpak-account`
`GET /internal/identity?pid=X -> {"friends":[{"pid":...}]}`. Its own comments flag the bare-PID
branch as an authentication hole (PIDs are sequential from 1800000001, so sending another
member's number plays as them).

None of that is reproduced. OpenPak resolves identity in `openpak/nex` through nx-baas
(a BAAS id token's `openpak` claim), and this server never sees a username; `host.FriendPIDs`
returns the friend graph from the same source and is wired to both the matchmaking manager's
`GetUserFriendPIDs` (populates the "visit a friend" menu) and DataStore's (friends-only
search perms). `openpak/nex` drops a friend whose PID does not fit 32 bits rather than
truncating it, which would name a different account.

## Ports

`22180` (auth `22180`, secure `22181`, `/healthz` `22182`) is this title's block, reserved in
`openpak` `ports.md`. the earlier server's `:443` auth and `:60007` secure are theirs and do not carry
over — but that both are TLS WebSocket does, which is why `NEX_TRANSPORT=websocket`.

## Error codes

| Code | Meaning | Source |
| --- | --- | --- |
| `2618-0502` | "Getting ready to depart" — Pia P2P session/hole-punch did not complete. The recurring symptom of the friend-visit gaps above (wrong station order, missing session key, `type=0x03` public station). | `earlier/acnh` / `earlier-nex` `matchmaking_acnh.go` |
| `2306-0116` | login/structure mismatch, and also a `FindByParticipant` response of bare sessions instead of `{PID, session}` pairs | `earlier-nex` `matchmaking_acnh.go`; `openpak/nex` `login.go` records the login side |
| `0x80690004` | `DataStore::NotFound`, facility 105 — what this server returns for a missing object | `pretendo-src` `nex.ResultCodes.DataStore.NotFound` |

## Facts we did not need

Recorded so nobody hunts for them twice: `earlier/acnh` also carries a monitoring dashboard
(`dashboard.go`), a presence reporter (`presence.go`, "playing ACNH now"), a dead-connection
reaper and admin kick, and a per-title shared kerberos password between its two endpoints. The
first are operational shape, not protocol facts, and OpenPak answers "is it up" with `/healthz`.
The shared kerberos password does not apply: `openpak/nex` generates both special accounts'
passwords per process, because nothing outside that process derives a key from them. The
`earlier/acnh` behaviours `PublicStationFirst` (GetSessionURLs order `[public, LAN]` with
`Pa=public`) and `JoinRespExistingCount` (announce participant count before adding the visitor),
both derived from reference-server captures, are noted here as measured behaviours whose
equivalents in `pretendo-src`'s common matchmaking are **unverified** — a capture comparing the
common library's GetSessionURLs / JoinMatchmakeSession output against ACNH's expectation is the
evidence the next round needs.
