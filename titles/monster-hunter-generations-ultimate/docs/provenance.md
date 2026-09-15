# Provenance

Where every non-obvious fact in this server came from, as
[the clean-room policy](../../../docs/clean-room-policy.md) requires. Nothing here was
copied: the facts were read, written down, and the server was built from this page.

"earlier MHGU" below means the earlier network server for this title
(`~/REPOS/monster-hunter-generations-ultimate`, source-available) — read for facts only.
"Reference NEX" means its core, `~/REPOS/earlier-nex`.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `4152f312` | earlier MHGU `main.go`, package comment and `accessKey`. Not publicly documented — MHGU has no Kinnay wiki entry. They report finding it in the game's ARM32 code by cross-referencing the confirmed game-server-id constant: two independent `MOVW`+`MOVT` sites resolve the neighbouring struct field to the same null-terminated 8-character string. |
| Game server ID | `2896BD04` | Same file. They report capturing it from the real client's own handshake against Nintendo's still-live production server, with their redirect disabled so the real name resolution happened. Also the SNI name suffix `g2896bd04`. |
| NEX library version | 4.4.0 (`apiVersionGeneral` 40400) | earlier MHGU `main.go` `nexVersion`, from the SDK version strings embedded in the game binary: `SDK MW+Nintendo+NEX_DS-4_4_0` and `NEX_UT-4_4_0`. |
| Secure server PID | 2 | The "Quazal Rendez-Vous" special account of the 3DS/Wii U convention — [Kinnay, Kerberos Authentication](https://github.com/kinnay/NintendoClients/wiki/Kerberos-Authentication), Special Accounts. earlier MHGU records that this convention, not the Switch-specific one, is the one MHGU follows. |
| Session key length | 32 bytes | earlier MHGU `main.go`; the same value `servers/nex` already uses. |

## Why this title is not shaped like a Switch title

MHGU is the Switch port of the 3DS's *Monster Hunter XX*, and its online code came across
with it: earlier MHGU's README and package comment record that the NEX paths live in a
32-bit ARM codebase under an AArch64 shim, and that none of the NEX init is reachable from
the AArch64 side.

| Fact | Value | Source |
| --- | --- | --- |
| Transport | raw PRUDP over UDP, both endpoints | earlier MHGU `main.go`: their ARM32 decompile shows the connection-type-9 secure connect opening a UDP socket, never a WebSocket, and they note Pretendo's MH4U server — same engine lineage — listens raw UDP for auth and secure. |
| Kerberos ticket version | 0 | earlier MHGU `main.go`. Version 1 puts a random key in the clear in front of the encrypted body; dropping it takes the ticket from 148 to 124 bytes. |
| The size problem | client raises `2306-0116` before transmitting | earlier MHGU README and `main.go`. `2306-0116` is Core::BufferOverflow, "buffer too large to send" — a client-side check, which matches their observation that no packet ever reached the secure port. The ticket is echoed back inside the secure CONNECT, so ticket size is the only lever they found. |
| Secure station URL | `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=3` | earlier MHGU `main.go`. `stream=10` is `StreamTypeRVSecure`. `type=3` is BehindNAT\|Public (nex-go `constants.StationURLFlag*`); they record it as what a 3DS-derived Pia wants, against the `0x0B` newer Switch titles use. **This one is their reading, not a capture** — see design.md's open questions. |

## Protocols the title uses

| Protocol | ID | Source |
| --- | --- | --- |
| Ticket Granting | `0x0A` | Standard NEX; [Kinnay, Authentication Protocol](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol). |
| Secure Connection | `0x0B` | Standard NEX. |
| NAT Traversal | `0x03` | Standard NEX. |
| Match Making | `0x15` | Standard NEX. earlier MHGU registers it. |
| Match Making Ext | `0x32` | Same. |
| Matchmake Extension | `0x6D` | Same. |
| Utility | `0x6E` | earlier MHGU: confirmed present from the binary's `NEX_UT-4_4_0` SDK string. |
| DataStore | `0x73` | earlier MHGU: confirmed present from the binary's `NEX_DS-4_4_0` SDK string. |

DataStore method IDs used here (`GetMeta` `0x8`, `GetMetas` `0x9`, `SearchObject` `0xC`,
`RateObject` `0xF`, `PostMetaBinary` `0x15`, `PreparePostObject` `0x18`, `PrepareGetObject`
`0x19`, `CompletePostObject` `0x1A`, `ChangeMeta` `0x26`, `DeleteObject` `0x4`) come from
`nex-protocols-go/v2/datastore`, which is AGPL-3.0 and a dependency of this server.

## Status of the title on the other stack

earlier MHGU's README and `mhgu_init.go` record that no measured reference of MHGU's online
init exists: they log every DataStore and Utility call and answer it with an empty list
(`count=0`), except `0x73.8` (`GetMeta`), which they answer `0x80690004` — DataStore::NotFound
— so the game continues instead of soft-locking on `Core::NotImplemented`. Reference NEX has
no general DataStore implementation at all; its only `0x73` code is Super Mario Maker 2's.
That is the gap this server fills, and the reason nothing here is a stub.

## OpenPak's own

| Fact | Source |
| --- | --- |
| Port block 22120 (auth 22120, secure 22121, healthz 22122) | [`ports.md`](../../../../../ports.md), claimed by this repository. |
| Identity: BAAS id token -> nx-baas `/internal/switch/identity` -> PID, nickname, friends | [`servers/nex`](../../../README.md) `docs/design.md`; nx-baas's internal game API on ports.md 20070. |
| Login methods, ticket construction, account callbacks | `servers/nex`, which implements them. This server adds none of it. |
| Matchmaking, NAT traversal, unique IDs, DataStore handlers | Pretendo's AGPL-3.0 `nex-protocols-common-go/v2`. |
| Postgres 23000, S3 23010 | `ports.md`. |
