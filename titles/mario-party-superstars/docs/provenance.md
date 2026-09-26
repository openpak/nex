# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): OpenPak reads the
earlier network servers for facts and writes its own code, so the facts have to be
checkable separately from the code.

Sources are abbreviated as:

- **NN/mps** — `~/REPOS/mario-party-superstars`, the earlier network server for this title
  (source-available, read for facts only, no code taken). Its own README states which
  of its values are confirmed and how, and that is what is carried across here.
- **Pretendo** — the AGPL-3.0 libraries this server imports, read as source.
- **Kinnay** — the [NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki),
  public reverse-engineering documentation.
- **Ours** — OpenPak's own repositories, or something measured in this repository.

## Title identity

| Fact | Value | Source | Confidence |
| --- | --- | --- | --- |
| Title ID | `01006FE013472000` | NN/mps README and `main.go`, which record it as confirmed from multiple independent NSP/XCI filename sources (2026-08-18) | Confirmed. Title IDs are public and consistently reported. |
| NEX access key | `e915510f` | NN/mps `example.env` and README | **Confirmed by measurement.** Brute-forced against one real captured PRUDP CONNECT `(connectionSig, client signature)` pair. The PRUDP-Lite packet signature is `HMAC-MD5(MD5(accessKey), MD5(accessKey) + connectionSig)`, a pure function of the key, and access keys are exactly 8 hex digits, so a 2³² search over that one pair identifies the key with no ambiguity. |
| Game Server ID | `28abaa00` | NN/mps README; the console resolves `g28abaa00-lp1.s.n.srv.nintendo.net` when going online | Confirmed, **and not the access key.** See the fleet note below. |
| NEX library version | 4.6.5 (`apiVersionGeneral` 40605) | NN/mps README and `example.env`: read out of the game binary's SDK-build version string table, which carries `SDK MW+Nintendo+NEX-4_6_5-` | Confirmed by binary inspection. |
| Pia version | 5.33.0 | Same string table, same dump | Confirmed by binary inspection. Past the 5.19 that introduced the modern station shape. |

### The fleet-wide trap in the hostname

`g<hex>-lp1.s.n.srv.nintendo.net` encodes the **Game Server ID**, not the access key. NN/mps
originally recorded `28abaa00` as this title's access key on that reading and it was wrong;
the check that settled it was earlier network's own deployed ARMS server, where the Game
Server ID is `25c08801` while the working access key is `b6b34c51` — unrelated values. Any
OpenPak title server that takes its access key from a resolved hostname is taking a guess.
The only sound methods are a brute force against a captured signature, or a published key.

## Protocols this title uses

The set is from NN/mps `main.go`, which registers exactly these on its secure endpoint.
Protocol and method IDs were then independently confirmed against Pretendo's
`nex-protocols-go/v2`, which is the implementation this server runs on.

| Protocol | ID | Source |
| --- | --- | --- |
| Secure Connection | `0x0B` | Pretendo; registered by `servers/nex` for every title |
| Match Making | `0x15` | Pretendo |
| Matchmake Extension | `0x6D` | NN/mps registers it; ID from Pretendo |
| Match Making Ext | `0x32` | Pretendo (`match-making-ext`) |
| NAT Traversal | `0x03` | NN/mps; ID from Pretendo |
| Utility | `0x6E` | NN/mps calls it "Utility (0x6E)"; Pretendo's `utility.ProtocolID` is `0x6E` — the two agree |
| DataStore | `0x73` | NN/mps `datastore_stub.go` declares `protocolDataStore uint16 = 0x73`; Pretendo agrees |
| Ranking | `0x70` | NN/mps registers a generic handler at `0x70`. Not implemented here — see design.md |

### Method IDs that matter

| Method | ID | Source | Why it matters |
| --- | --- | --- | --- |
| `MatchmakeExtension::FindMatchmakeSessionByGatheringIDDetail` | `0x29` (41) | NN/mps records "method 41" from a live observation; Pretendo's constant is `0x29` — the two agree | The game's Join Room screen polls it about once a second against a gathering ID it got from friend presence. NN/mps observed a joiner poll it for three minutes and get `RendezVousSessionVoid` every time. |
| `MatchmakeExtension::FindMatchmakeSessionByParticipant` | `0x33` (51) | NN/mps records "0x6D method 51"; Pretendo's constant is `0x33` — the two agree | Asks which gathering a PID is currently in, so a joiner can follow the host. Answered empty, the game falls back to public auto-matchmaking and creates its own session instead — NN/mps's diagnosis of the "my friend could not see my lobby" bug. |
| `Utility::GetIntegerSettings` | `0x7` | Pretendo | Called right after `SecureConnection::Register`. NN/mps observed the connection dying about a second later, every time, when the reply was malformed. |
| `Utility::GetStringSettings` | `0x8` | Pretendo | Same shape, strings. |
| `DataStore::GetMetaMultiple` | `8` | NN/mps `datastore_stub.go` | Their stub answers `0x80690004` here specifically. |

### Wire shapes taken as facts

- `Utility::GetIntegerSettings` takes **one** `UInt32` index and returns `Map<UInt16, Sint32>`
  — not a list. Source: Kinnay's Utility Protocol page, recorded in NN/mps. NN's generic
  handler misread the index as a list length and answered a body the client could not parse.
  Pretendo's `nex-protocols-go` reads it correctly (its map value type is `UInt32`, which is
  the same four bytes on the wire).
- `DataStore::NotFound` is `0x80690004`. Source: NN/mps `datastore_stub.go`. Independently
  confirmed against Pretendo's `nex.ResultCodes.DataStore.NotFound = 0x00690004` plus the
  error bit — the same code.
- The secure station URL is `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2`.
  Source: NN/mps `main.go`. `stream=10` is `StreamTypeRVSecure` in Kinnay's stream-type list,
  which is what `servers/nex` already sets — the two agree, so nothing here contradicts the
  shared host.
- Session key length 32, secure server PID 2. Source: NN/mps `main.go`; the same values
  `servers/nex` already uses.

## Transport and wire shape — confirmed, and a fleet correction

These were first filed here as "earlier network deployment details that do not carry across".
That was wrong: they are facts about the game, and `servers/nex` now has knobs for all of
them. Recorded precisely, because this title is one of the fleet's better-documented ones.

| Fact | Value for this title | Failure mode when wrong | Source |
| --- | --- | --- | --- |
| Transport | **WebSocket**, not UDP | Silence. A console failing a handshake against the wrong transport looks exactly like a console that never arrived | NN/mps runs both endpoints with `ListenSecure`, a WSS listener. `servers/nex` now defaults to `websocket` for exactly this reason |
| Listener shape | **Two listeners** — authentication and secure on separate ports | nex-go logs `trying to connect to unbound PRUDPEndPoint` for every packet to a stream nothing is bound to. Checkable, not silent | NN/mps `main.go`: `AUTH_PORT` and `SECURE_PORT` are separate, `:443` and `:60011`/`:60012` |
| Secure stream ID | **1** | As above | NN/mps station URL sets `sid=1`. This is the two-listener shape: `NEX_SECURE_STREAM_ID=1`, the default. **Not 2.** |
| Station scheme | **`prudps`** | **Late.** Under `prudp` the handshake completes but CONNECT carries an *empty* payload — no ticket, no session key — so menus keep working and the title dies when it first needs the session | NN/mps `example.env`, recorded as a measurement with that failure mode |
| Kerberos **source key** length | **32 bytes** | **Silent.** The login simply does not work, with nothing logged on either side | NN/mps `main.go` `resolveUser` derives it with SHA-256 and hands back all 32 bytes, commented "It MUST be 32 bytes (the Switch kerberos key size)" — see below |
| Secure station URL | `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | — | NN/mps `main.go`. `stream=10` is `StreamTypeRVSecure`; `PID=2` is the conventional secure PID. Both are what `servers/nex` already defaults to |
| Session key length | 32 | — | NN/mps `main.go`; same as the host's default |

The three with a failure mode are pinned as constants in `server.go` rather than inherited
from the host's defaults. A default is a reasonable starting point for a title that has not
measured; it is the wrong thing to rely on for a value whose failure is silent or late, and
whose measurement lives in this repository.

On the source key: the comment in NN/mps calls 32 "the Switch kerberos key size", and that
generalisation is **wrong** — `servers/nex` records a public reference server working against
a retail Super Mario Bros. 35 with 16. The length is not a protocol constant; what must hold
is that the key encrypting the ticket is the key advertised in the response. So 32 is a fact
about *this* title's working server, not about Switch, and it is pinned in the code rather
than inherited because the failure is silent: the login simply does not work, with nothing
logged on either side.

**Fleet correction, confirmed here.** A fleet-wide instruction went out that every Switch NEX
title needs `NEX_SECURE_STREAM_ID=2` and a single listener. That is wrong for this title and
was retracted. Mario Party Superstars is a **two-listener, stream-1** title, and the evidence
is its own working server: separate auth and secure ports, `sid=1` in the station URL it hands
the console. The per-title table in `servers/nex`'s README now agrees. Nothing in this
repository contradicts the shared host.

### What genuinely does not carry across

- NN's **port numbers** (`:443`, `:60011`/`:60012`) and its own TLS termination. OpenPak uses
  the block from `ports.md` and terminates TLS at the front proxy by default. Ports are a
  deployment choice; the transport above is not.
- `MPS_SECURE_MINOR` and `MPS_LEGACY_PIA` are knobs on `earlier-nex`'s own PRUDP
  implementation. `nex-go` handles the equivalents itself.
- The Pia type-8 keepalive NN ACKs by hand (`pia_keepalive.go`) works around a gap in
  `earlier-nex`'s packet-type switch — it only knows types 0–4. That is a property of their
  transport, not of the game, and `nex-go` is a different transport. Whether it has the same
  gap is an open question below, not a fix to port.

## DataStore: what this title is known to use

Nothing. This is a finding, not a gap in the research.

the earlier network server registers a DataStore stub, and its own comment says why: Super
Smash Bros. Ultimate soft-locked with no DataStore handler registered, so a stub was added
here **by analogy, before ever seeing an MPS-specific DataStore call logged** — their
connection never survived long enough to make one. No object type, field, permission or
persistence slot has ever been observed for this title.

So there is nothing title-specific to record here yet, and nothing to build an object store
against. When a session survives long enough to call DataStore, what it asks for belongs in
this section, and the store it needs belongs in `servers/nex` (fleet decision), not here.

One question this title raised and could not answer: DataStore's persistence slot has no
"none" sentinel in Pretendo's types, whose constructors default it to 0 — which is also a
legitimate slot. Guess wrong and every object silently evicts the last. `servers/nex` chased
it and found that **no server on either side has ever exercised the persistence path**: Super
Mario Maker 2's implementation, the most DataStore-heavy one anywhere, implements
title-specific methods only and defers generic persistence explicitly. So it cannot be settled
by reading anyone's source. It needs a real `PreparePostObject` off a console, and the shared
store holds the assumption behind one named constant until then.

One correction worth keeping while it is fresh: the SSBU failure was that `earlier-nex`
**silently dropped** calls to an unregistered protocol. `nex-go` answers
`Core::NotImplemented` instead, which is what a stub would have said. The defensive stub is
therefore not needed on this stack — the reason for it does not exist here.

## Assumptions this server makes that nothing has confirmed

Each is a place where a capture would settle something the code currently guesses at.

1. **Utility integer settings content.** Nobody knows what Mario Party Superstars expects in
   its integer settings map. NN/mps ships a placeholder table whose *shape* came from Kinnay's
   SMB35 configuration — a different title. This server declines to guess: it answers a
   correctly shaped empty map and takes real values from `MPS_INTEGER_SETTINGS` when a
   capture provides them. **Evidence needed:** the settings map from a working session, or the
   game's behaviour with an empty one.
2. **Whether the game calls DataStore at all.** See the DataStore section above: nobody has
   ever observed one. Nothing is registered for it. **Evidence needed:** a session that gets
   past `GetIntegerSettings`, and a log of what it asks DataStore for — object types, fields,
   permissions and persistence slots.
3. **Ranking.** NN/mps answers `0x70` with a generic acknowledgement and its README says it is
   unknown whether the title needs real leaderboard data. **Evidence needed:** whether the
   game calls Ranking, and with what.
4. **The Pia keepalive.** Whether `nex-go` already answers the periodic type-8 packet, or
   whether this title will drop its connection every few seconds the way it did on the NN
   stack. **Evidence needed:** a connection held open past ten seconds, or a `nex-go` packet
   log showing an unanswered type.
5. **Everything `servers/nex` lists as open** — whether the console sends an nx-baas id
   token, whether it fits the token field, what `userName` carries, and whether this title
   logs in with method 2 or method 6. This title's NEX 4.6.5 is past the 4.4.0 line, so
   method 6 (`ValidateAndRequestTicketWithParam`) is the likely one, but that is inference
   from the version, not a capture. NN/mps's `datastore_stub.go` says SSBU uses
   `ValidateAndRequestTicketWithParam` "like MPS", which is the same inference from the same
   place.
