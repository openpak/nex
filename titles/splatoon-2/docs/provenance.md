# Provenance

Where every non-obvious fact in this repository came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): facts about a console and a
protocol are facts, and this file is what makes "we rewrote it from the facts" checkable.

Splatoon 2 is a **NEX** title and this is its NEX server. Splatoon 3 is **NPLN**, a different
protocol served by a different repository — the two are not related here.

Sources, referred to below by these names:

- **Kinnay** — [kinnay/NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki),
  public reverse-engineering documentation.
- **Earlier research notes** — facts recorded in earlier work on this title. Facts only; no code,
  structure, names or comments were carried across.
- **Pretendo** — the AGPL-3.0 `nex-go`, `nex-protocols-go` and `nex-protocols-common-go`
  sources, which OpenPak links against and may therefore also read.
- **ports.md** — OpenPak's own port plan.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `4eb18d39` | Kinnay's Game Server List (which lists MK8 `09c1c475`, SSBU `9587602b` alongside). Configuration here, never a literal in source (`NEX_ACCESS_KEY`) |
| NEX version | `4.0.0` (`40000`) | Earlier research notes: a working server runs this title with `nexVersion = 40000`. Not a capture of the console's `apiVersionGeneral`, but the value that server uses, so more grounded than an era guess |
| Application ID | `0100f8f0000a2000` | Earlier research notes, used for a presence API. Not needed by this server; recorded for completeness |

## Transport and endpoint shape

| Fact | Value | Source |
| --- | --- | --- |
| Transport | PRUDP over WebSocket, TLS | Earlier research notes: a working server listens with a certificate and key, auth on 443 and secure on 60004 |
| Listener shape | **Two** listeners, authentication and secure on separate ports | Same notes: two ports, both listening. This is the ARMS/Mario Golf two-listener shape in `servers/nex`'s README, so `NEX_STREAM_ID` and `NEX_SECURE_STREAM_ID` are both 1 |
| Secure endpoint stream ID | 1 | Earlier research notes: the secure station URL carries `sid=1` |
| Station URL | scheme `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | Earlier research notes. `stream=10` is the RVSecure stream type, which `servers/nex` sets from `constants.StreamTypeRVSecure` |
| Secure server PID | 2 | Same. The conventional NEX value, and `servers/nex`'s default |
| Source key length | 32 bytes | Earlier research notes (`sessionKeyLen = 32`), the Switch kerberos key size. `NEX_SOURCE_KEY_BYTES=32` |

## Login: method 2, not method 6

| Fact | Source |
| --- | --- |
| This title logs in with **`LoginEx` (method `0x2`)** = `ValidateAndRequestTicketWithCustomData` on Switch, **not** `ValidateAndRequestTicketWithParam` (`0x6`) as SSBU does | Earlier research notes: S2 logs in with LoginEx 0x2, unlike SSBU, which uses ValidateAndRequestTicketWithParam 0x6. This is the "method 2 vs 6" split, decided by the older NEX 4.0.0 generation |

`servers/nex` (openpak/nex v0.2.1) already wires **both** ticket-granting methods —
`validateAndRequestTicketWithCustomData` (2) and `validateAndRequestTicketWithParam` (6) — and
nex-protocols-go routes method 2 to the WithCustomData handler when the version is ≥ 4.0.0. So
this title needs no login change in the host: it hits method 2, and the host answers the older
flat `RVConnectionData` shape with `pSourceKey` appended, which is what a NEX 4.0.0 title
expects. Recorded because it is the one place Splatoon 2 differs from every method-6 sibling.

## Protocols the title speaks

The set registered on the secure endpoint by a working server, which is the observed set. IDs are
Kinnay's and match Pretendo's constants.

| Protocol | ID | Implemented here by |
| --- | --- | --- |
| Secure Connection | `0x0B` | `servers/nex` (it registers this itself) |
| Match Making | `0x15` | `nex-protocols-common-go` |
| Matchmake Extension | `0x6D` | `nex-protocols-common-go`, plus method 60 below |
| Match Making Ext | `0x32` | `nex-protocols-common-go` |
| NAT Traversal | `0x03` | `nex-protocols-common-go` |
| Ranking | `0x70` | `nex-protocols-common-go` over our Postgres store, plus fest methods below |
| Utility | `0x6E` | `nex-protocols-common-go`, plus methods 9 and 10 below |
| DataStore | `0x73` | `nex-protocols-common-go` over our Postgres store |

## DataStore is a full implementation

Earlier research found only an empty-success stub for every `0x73` method, so there were no
per-method behaviours to read. This is a missing implementation, not a set of missing facts.
Pretendo's `nex-protocols-common-go/v2/datastore` implements the whole protocol
and asks only where objects live; `datastore.go` answers that with Postgres (+ presigned S3 for
object bodies). See [`design.md`](design.md) for the method-by-method table of what is real.

## Methods above the standard protocols (splatfest.go)

Splatoon 2 calls a few method IDs that are above each protocol's standard set, so
`nex-protocols-go` answers them `Core::NotImplemented`. Each is answered through nex-go's
`Protocol.Patches` hook so the standard switch never sees them (no double response).

| Method | ID | Answer & why | Source |
| --- | --- | --- | --- |
| `Utility::UpdateCurrentUser` | `0x6E.10` | **Empty success. Gates network-ready.** `NotImplemented` here dropped S2 to `cNetworkReadyFailed` and it disconnected (`PlayReport lobby_error_event{error_from:3}`). The empty body was *measured* from a real Switch match (`resp_0x6e_m10.bin` = 0 bytes), overriding an earlier disassembly that guessed a ~31-byte struct | Earlier research notes |
| `Utility::AcquireTagId` | `0x6E.9` | A benign non-zero `u64` derived from the caller PID. In-session sharing; never a bring-up gate | Earlier research notes |
| `Ranking::GetFestivalResult` | `0x70.18` | Empty list. Splatfest query; `NotImplemented` = `2306-0103` comm error. S2's official servers are dead, so a live fest can never be proxied — an empty list is the best-grounded answer | Earlier research notes |
| `Ranking` fest ranking | `0x70.19` | Empty list. Fired at the 8-player match start while a splatfest is active. Same family as 18/25 | Earlier research notes |
| `Ranking::GetEventMatchResult` | `0x70.25` | Empty list. Called every ~6 min while a splatfest is active | Earlier research notes |
| `MatchmakeExtension` custom | `0x6D.60` | Empty list. A "get simple playing session" custom method; for a normal match an empty list is valid and unblocks the post-match sequence. (Pretendo's standard `GetSimplePlayingSession` is `0x6D.1F` — 60 is a different, custom ID) | Earlier research notes |

Method 60's identity: Pretendo's `matchmake-extension/protocol.go` records `GetSimplePlayingSession`
as `0x1F` (31). S2 calls **60**, above the standard set, so it is a title-custom method and gets
the empty-list answer rather than the common handler.

## Hardware bring-up risks (not fixable in this repo alone)

| Risk | Detail | Source |
| --- | --- | --- |
| **Public station type** | Splatoon 2 predates Switch Pia 5.19: a working server answers `SecureConnection::Register` with a public station of **type `0x03` and no `Pa` param**, where SSBU's answers `0x0B` + `Pa`. Pretendo's common `register`/`registerEx` sets `Public\|BehindNAT`; whether that satisfies S2 is unverified. A wrong station shape fails late (menus work, the session fails when Pia needs it). This lives in `nex-protocols-common-go`/`servers/nex`, not here | Earlier research notes |
| **PRUDP secure minor version 0** | A working server forces `PrudpMinorVersion = 0` on the secure endpoint. `servers/nex`'s README says nex-go already answers with the minor version the client sent, so there should be nothing to configure — but this is the ARMS/MTA capstone risk and unverified for S2 | Earlier research notes; `servers/nex` README |
| **`NEX_VERSION`** | `4.0.0` is a working server's runtime value, not a capture of `apiVersionGeneral`. First capture settles it; `NEX_VERSION` takes the integer form (`40000`) directly | Earlier research notes |

## OpenPak's own facts

| Fact | Value | Source |
| --- | --- | --- |
| NEX port block | 22320: auth 22320, secure 22321, `/healthz` 22322 | ports.md, reserved for this repository (not edited by this work) |
| Account identity, friend graph | `Host.PlayerByPID` / `Host.FriendPIDs` over nx-baas `/internal/switch/identity` | `servers/nex` |
| DataStore/Ranking storage | Postgres schemas `datastore` and `ranking`, created at startup; object bodies in optional S3 | this repository |
| Matchmaking, tracking and utility tables | created by `nex-protocols-common-go` when its managers are set | Pretendo source; this repository creates only the `datastore` and `ranking` schemas |

## Convergence note (for the shared-subpackage pass)

`servers/nex`'s `datastore` subpackage (`github.com/openpak/nex/datastore`) is **not built yet**.
This server therefore registers Pretendo's common DataStore protocol directly and keeps its own
`datastore.go`/`ranking.go`/`schema.go`. Those three files are the same storage shape as
`servers/mario-tennis-aces` — deliberately, so the convergence pass can lift the common parts
into the shared subpackage and have both titles import it. The Splatoon 2-specific code is
`splatfest.go` and the login/config facts above; nothing there belongs in the shared package.

| Game-server id / secure host | `g2df33d01-lp1.s.n.srv.nintendo.net` (`NEX_SECURE_HOST`, set on production 2026-09-11) | Kinnay's Game Server List, Switch table: Splatoon 2 | 24E30D00, 2C4BFF00, 2DF33D01 (Testfire, World premiere, Main server). The host shape `g<id>-lp1.s.n.srv.nintendo.net` follows the same shape as the other Switch NEX hosts |
