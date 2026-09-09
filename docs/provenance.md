# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md): OpenPak reads the
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

## Facts about earlier network's deployment that do **not** carry across

Recorded so nobody re-derives them and then wonders why OpenPak looks different.

- NN runs its NEX endpoints over **TLS (WSS)** — auth on `:443`, secure on `:60011`/`:60012`,
  each terminating its own certificate — because `earlier-nex` is a from-scratch stack whose
  transport is WebSocket. OpenPak runs Pretendo's `nex-go`, which is **PRUDP over UDP**, on
  the block from `ports.md`. Ports, transport and TLS are all different by design. Neither
  arrangement is a fact about the game.
- `MPS_STATION_SCHEME`, `MPS_SECURE_MINOR` and `MPS_LEGACY_PIA` are knobs on NN's own PRUDP
  implementation. `nex-go` handles the equivalents itself, so they have no counterparts here.
- The Pia type-8 keepalive NN ACKs by hand (`pia_keepalive.go`) is a gap in `earlier-nex`'s
  packet-type switch, not a property of the game. Whether `nex-go` needs the same is an open
  question below, not a port of their fix.

## Assumptions this server makes that nothing has confirmed

Each is a place where a capture would settle something the code currently guesses at.

1. **Utility integer settings content.** Nobody knows what Mario Party Superstars expects in
   its integer settings map. NN/mps ships a placeholder table whose *shape* came from Kinnay's
   SMB35 configuration — a different title. This server declines to guess: it answers a
   correctly shaped empty map and takes real values from `MPS_INTEGER_SETTINGS` when a
   capture provides them. **Evidence needed:** the settings map from a working session, or the
   game's behaviour with an empty one.
2. **Persistence slot `0xFFFF` means "not persistent".** DataStore's persistence slot has no
   sentinel in Pretendo's types, whose constructors default it to 0. `0xFFFF` is the
   conventional "none". If this title actually posts to slot 0 for ordinary objects, every
   object would evict the last. **Evidence needed:** the `PersistenceInitParam` of a real
   `PreparePostObject`.
3. **The game calls DataStore at all.** NN/mps registered a stub defensively after SSBU
   soft-locked without one; their connection never survived long enough to log an actual
   DataStore call from this title. **Evidence needed:** a session that gets past
   `GetIntegerSettings` and a log of what it asks DataStore for.
4. **Ranking.** NN/mps answers `0x70` with a generic acknowledgement and its README says it is
   unknown whether the title needs real leaderboard data. **Evidence needed:** whether the
   game calls Ranking, and with what.
5. **The Pia keepalive.** Whether `nex-go` already answers the periodic type-8 packet, or
   whether this title will drop its connection every few seconds the way it did on the NN
   stack. **Evidence needed:** a connection held open past ten seconds, or a `nex-go` packet
   log showing an unanswered type.
6. **Everything `servers/nex` lists as open** — whether the console sends an nx-baas id
   token, whether it fits the token field, what `userName` carries, and whether this title
   logs in with method 2 or method 6. This title's NEX 4.6.5 is past the 4.4.0 line, so
   method 6 (`ValidateAndRequestTicketWithParam`) is the likely one, but that is inference
   from the version, not a capture. NN/mps's `datastore_stub.go` says SSBU uses
   `ValidateAndRequestTicketWithParam` "like MPS", which is the same inference from the same
   place.
