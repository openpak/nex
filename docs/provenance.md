# Provenance

Every non-obvious fact in this repository and where it came from, per
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md). Facts were read, written
down here, and then OpenPak's own version was written from these notes. No third-party code,
comment, doc text, structure or naming was copied.

**Record what did not make sense, not only what did.** Twice in one round an observation
logged here as unexplained turned out to be the cause of something another repository had
measured, and in both cases neither half was enough on its own: the ticketless-CONNECT
behaviour sat here as noise until a title server measured that `prudp` produces an empty
CONNECT payload, and the "source key must be 32" claim only fell once a second title reported
16 working against retail. Neither connection could have been made from inside one repository.
So an oddity with no explanation still earns a row, marked as such.

Sources are abbreviated:

- **K** — Kinnay's NintendoClients wiki (public).
- **P** — Pretendo wiki (public).
- **PL** — Pretendo's AGPL libraries, read as source we may depend on.
- **O** — our own repositories.
- **KS** — [kinnay/SMB35](https://github.com/kinnay/SMB35), a public AGPL-3.0 reference implementation
  known to work with a retail Switch client (`source/main.py`).

## Protocol

| Fact | Value | Source |
| --- | --- | --- |
| TicketGranting protocol ID | `0x0A` | K, PL |
| Method IDs 1/2/3/6 carry two name sets; the Switch names apply from NEX 4.0.0 | `ValidateAndRequestTicket`, `…WithCustomData`, `RequestTicket`, `…WithParam` | K "Authentication Protocol"; dispatch confirmed in PL `nex-protocols-go` |
| Method 6 request is a `ValidateAndRequestTicketParam` structure: platformType, userName, extraData, ignoreApiVersionCheck, apiVersionGeneral, apiVersionCustom | — | K; type present in PL |
| Method 6 response is a `ValidateAndRequestTicketResult` structure with **no leading `%retval%`** | sourcePid, bufResponse, serviceNodeUrl, currentUtcTime, returnMsg, sourceKey | K |
| **Method 6's response must be wrapped in a structure header with version 1**, which requires the Switch byte-stream profile above to be on — setting the version alone writes no header at all. Returning the flat method-2 layout is rejected by the client as `Core::InvalidArgument`, console error **2306-0116** | `StructureVersion = 1` + `UseStructureHeader` | earlier research notes — a hard-won result of a live client rejecting the flat layout |
| Method 2 keeps the older flat layout with `pSourceKey` appended | — | K; PL `nex-protocols-common-go/ticket-granting/login_ex.go` |
| Which method a title uses follows its NEX version. NEX 4.6+ titles (e.g. SSBU) use method 6; earlier 4.x titles use method 2 | — | earlier research notes; per-title versions below |
| Switch users log in with an id token, not a password; the response's `sourceKey`, when present, replaces key derivation | — | K "Kerberos Authentication", P "nex/kerberos" |
| Wii U/3DS kerberos key derivation | MD5 of the password, `65000 + pid % 1024` times | K, P; implemented in PL as `DeriveKerberosKey` |
| Switch password-based derivation (special accounts only) | `MD5(MD5(password) + pack("<Q", pid))` | K, P |
| Switch internal ticket format: random key in the clear beside the encrypted body, final key `MD5(target key + random key)` | `KerberosTicketVersion = 1` | K; PL implements both formats |
| Session key length | 32 bytes | K, KS |
| The source key's length is **not** a protocol constant. What must hold is that the key the ticket is encrypted with is the key advertised in the response; RC4 takes any length and the console is told the key rather than deriving it | 16 bytes in KS (retail SMB35); 32 bytes in two independent working servers (Mario Kart 8 Deluxe and Mario Party Superstars), both deriving it with SHA-256 and returning all 32. Neither generalises, so it is `NEX_SOURCE_KEY_BYTES`. The claim that it *must* be 32 as "the Switch kerberos key size" is contradicted by the retail-working 16 and must not be propagated | KS, earlier research notes; conflict independently reached by the smb35 and mario-party-superstars agents |
| Under `prudp` where a title wants `prudps`, the PRUDP handshake completes but CONNECT carries an **empty payload** — which nex-go reads as a ticketless connect. This is why the title works through menus and fails only when it first needs the session | — | measured by the mario-party-superstars agent; explains the ticketless-CONNECT case recorded below |
| **The Switch byte-stream profile: 8-byte PIDs and structure headers on.** nex-go defaults to 4-byte PIDs and headers off, which is the Wii U/3DS wire. Per console, not per title | `PIDSize=8`, `UseStructureHeader=true` | earlier research notes, reported by the mk8d title agent; the mechanism is visible in PL `nex-go/types/structure.go` |
| `UseStructureHeader` gates the structure header in **both** directions: `WriteHeaderTo` writes nothing when it is off, and `ExtractHeaderFrom` consumes nothing | — | PL `nex-go/types/structure.go` |
| PRUDP virtual ports are **learned from the SYN** — the server answers as the port the client targeted; there is no fixed value | — | earlier research notes |
| **Switch NEX is PRUDP over WebSocket**, not over UDP, and the console reaches it over `wss` | — | KS serves a WebSocket transport; corroborated by Golf's live SNI hostname being forwarded by Traefik |
| WebSocket transport and PRUDP Lite are one choice, not two: nex-go reads a packet as Lite only when a websocket server is present | — | PL `nex-go/prudp_server.go`, reported by the Golf title agent |
| PRUDP Lite's SYN/CONNECT ack never sets a minor version, so it answers 0 — which matches the retail secure server, and needs no knob | — | PL `nex-go`, reported by the Golf title agent; a server whose library defaulted to 5 had to force minor 0 |
| **Some titles put authentication and secure on one listener, separated by stream ID** — authentication stream 1, secure stream 2, same transport | — | KS (SMB35) |
| **Other titles use two WSS listeners on two ports**, with a real dialable secure station URL. So the shape is per title, not universal, and both are known to work | ARMS auth 443 / secure 60006; Golf auth 8457 / secure 61000 | earlier research notes |
| Because of that, the secure station URL in the login response is a **placeholder** — the console does not dial it, it switches virtual port on the connection it already has | `address=0.0.0.1`, `port=1`, `sid=2` | KS |
| Full retail secure station URL shape for a Switch title | `prudps`, address `0.0.0.1`, port 1, `PID=<secure pid>`, `CID=1`, `type=2`, `sid=2`, `stream=10` | KS |
| A secure server PID need not be 2; a retail-observed constant exists | `257049437023956657` (SMB35) | KS |
| The login response's source key is 16 random bytes, hex-encoded — independently confirming the key inversion | — | KS |
| PRUDP-Lite CONNECT signature is a pure function of the access key and connection signature: `HMAC-MD5(MD5(accessKey), MD5(accessKey) + connectionSig)` | — | earlier research notes |
| Some consoles send a **ticketless (empty) CONNECT** to the secure server | — | earlier research notes |

## Identity

| Fact | Value | Source |
| --- | --- | --- |
| Login `extraData` is an `AuthenticationInfo` DataHolder whose `Token` field is a JWT | starts `eyJ` | earlier research notes; type confirmed in PL |
| **A real console sends a genuine BAAS id token with no custom claim.** Only an emulator build adds a server-specific claim to bind the account | — | earlier research notes |
| **`userName` on Switch is the console's NSA id, not the NEX PID.** It is a large decimal number (observed example on the order of 1.1e19, i.e. above 2^63) | — | earlier research notes |
| Titles later address players by that same login name rather than by the NEX PID, so the mapping must be remembered at login | Super Mario Maker 2's DataStore is the recorded case | earlier research notes |
| The `AuthenticationInfo` framing varies across NEX versions and titles, so strict structural parsing of the token field is fragile | — | earlier research notes |
| The 918-byte token ceiling is **documentation only** — it is defined in PL but never enforced, and full JWTs are carried in practice | not a constraint | PL `nex-protocols-go/ticket-granting/constants`, grep shows no use; whole JWTs are carried in practice |
| nx-baas internal identity API | `POST /internal/switch/identity`, `X-Internal-Key` | O `nx-baas/internal.go` |

## Per-title configuration (mechanism only — values live in each title's repository)

| Fact | Value | Source |
| --- | --- | --- |
| An access key is always exactly 8 hex digits | 2^32 search space | earlier research notes |
| Access keys come from Kinnay's public Game Server List, from the game binary, or by brute-forcing one captured `(connectionSig, signature)` pair against the formula above | — | earlier research notes |
| The `g<hex>-lp1.s.n.srv.nintendo.net` hostname encodes a **Game Server ID, not the access key** — the two are different values for the same title | — | earlier research notes (records an earlier wrong theory and its correction) |
| NEX version is carried as an integer: `40605` is NEX 4.6.5 | `major*10000 + minor*100 + patch` | earlier research notes |
| Observed NEX versions span 4.0.0 to 4.6.5 across Switch titles | — | earlier research notes |
| Super Mario Bros. 35 uses **method 6**, NEX version 40600, access key `0a69c592` | — | KS |
| The secure station URL scheme is per title: `prudps` for most, `prudp` for at least one major title. The wrong one lets menus work and then fails when the session is actually needed | — | earlier research notes |
| Pia's wire shape changed at Pia 5.19; titles before and after differ | — | earlier research notes |

## DataStore

| Fact | Value | Source |
| --- | --- | --- |
| Pretendo's `nex-protocols-common-go/v2` ships a complete DataStore (get/change meta, prepare and complete post/get, search, rate, delete, S3 presigning) | — | PL `nex-protocols-common-go/datastore` |
| ARMS needs DataStore: its **update** links a DataStore client even though the base game does not, and the update is what goes online | — | earlier research notes; reported by the arms title agent |
| DataStore protocol ID | `0x73` | earlier research notes |
| `SecureConnection::TestConnectivity` is method `0x5`, takes nothing and returns nothing; a real title polls it steadily (measured 23 times in six hours) and `nex-protocols-common-go` leaves it unset, so it answers `Core::NotImplemented` | — | PL `nex-protocols-go/secure-connection`; call frequency from earlier Mario Tennis Aces logs, reported by the mario-tennis title agent |
| DataStore's **persistence slot has no "not persistent" sentinel** in Pretendo's types — constructors default it to 0, which is also a legitimate slot. An implementation must pick one and say so; if it is wrong, ordinary objects posted to slot 0 evict each other | unresolved | PL `nex-protocols-go/datastore`; raised by the mario-party-superstars agent |
| `UpdateObjectPeriodByDataIDWithPassword` and `UpdateObjectDataTypeByDataIDWithPassword` declare **each other's** parameter names; the call sites pass the correct values, so implement by callback name | — | PL `nex-protocols-common-go/datastore/protocol.go` lines 26/28 vs `change_meta.go` lines 51/65, verified; found by the arms title agent |
| `GetObjectInfoByDataID` must error for a not-yet-upload-completed object, because `CompletePostObject` uses "returns data" to mean "already uploaded" and otherwise answers `PermissionDenied` forever. `GetObjectOwnerByDataID` and `GetObjectSizeByDataID` must still work on those objects | — | PL `nex-protocols-common-go/datastore/complete_post_object.go` lines 51-56, verified; found by the arms title agent |

Its permission check for "friends only" is documented upstream as not working on Switch,
because it asks for friends by 32-bit PID. OpenPak has the friend graph from nx-baas, so
`Host.FriendPIDs` fills that callback and the gap closes for us.

## Still unsettled

- Whether nx-baas can prove a genuine console BAAS id token. Today its internal API proves
  the emulator-style claim or takes an already-proven PID. `Host.PlayerByToken` sends the
  whole token as `id_token` alongside the claim so the wire is ready, but until nx-baas
  answers on that field a real console's login is refused. **This is the one cross-repo
  dependency and the next blocking item.**
- Pia minor-version and station-scheme behaviour per title: `NEX_STATION_SCHEME` is exposed,
  but no OpenPak title has connected yet to confirm a value.
- Whether any Switch title uses a secure stream other than 2, or an authentication stream
  other than 1. Both are configurable; the retail-observed pair is 1 and 2.
- Whether the placeholder station URL is universal across Switch titles or SMB35-specific. It
  is the default only in single-listener mode, so a title that needs a real address gets one
  by leaving the stream IDs equal, or by overwriting `Host.SecureStationURL`.
