# Provenance

Every non-obvious fact in this repository and where it came from, per
[`docs/clean-room-policy.md`](../../../docs/clean-room-policy.md). Facts were read, written
down here, and then OpenPak's own version was written from these notes. No earlier code,
comment, doc text, structure or naming was copied.

Sources are abbreviated:

- **K** — Kinnay's NintendoClients wiki (public).
- **P** — Pretendo wiki (public).
- **PL** — Pretendo's AGPL libraries, read as source we may depend on.
- **N** — a earlier network repository, read for facts only.
- **O** — our own repositories.
- **KS** — [kinnay/SMB35](https://github.com/kinnay/SMB35), a public AGPL-3.0 reference server
  known to work with a retail Switch client (`source/main.py`).

## Protocol

| Fact | Value | Source |
| --- | --- | --- |
| TicketGranting protocol ID | `0x0A` | K, PL, corroborated by N `earlier-nex/ticketgranting.go` |
| Method IDs 1/2/3/6 carry two name sets; the Switch names apply from NEX 4.0.0 | `ValidateAndRequestTicket`, `…WithCustomData`, `RequestTicket`, `…WithParam` | K "Authentication Protocol"; dispatch confirmed in PL `nex-protocols-go` |
| Method 6 request is a `ValidateAndRequestTicketParam` structure: platformType, userName, extraData, ignoreApiVersionCheck, apiVersionGeneral, apiVersionCustom | — | K; type present in PL |
| Method 6 response is a `ValidateAndRequestTicketResult` structure with **no leading `%retval%`** | sourcePid, bufResponse, serviceNodeUrl, currentUtcTime, returnMsg, sourceKey | K |
| **Method 6's response must be wrapped in a structure header with version 1**, which requires the Switch byte-stream profile above to be on — setting the version alone writes no header at all. Returning the flat method-2 layout is rejected by the client as `Core::InvalidArgument`, console error **2306-0116** | `StructureVersion = 1` + `UseStructureHeader` | N `earlier-nex/ticketgranting.go` — stated as a hard-won result of a live client rejecting the flat layout |
| Method 2 keeps the older flat layout with `pSourceKey` appended | — | K; PL `nex-protocols-common-go/ticket-granting/login_ex.go` |
| Which method a title uses follows its NEX version. NEX 4.6+ titles (e.g. SSBU) use method 6; earlier 4.x titles use method 2 | — | N `earlier-nex/ticketgranting.go`; per-title versions below |
| Switch users log in with an id token, not a password; the response's `sourceKey`, when present, replaces key derivation | — | K "Kerberos Authentication", P "nex/kerberos" |
| Wii U/3DS kerberos key derivation | MD5 of the password, `65000 + pid % 1024` times | K, P; implemented in PL as `DeriveKerberosKey` |
| Switch password-based derivation (special accounts only) | `MD5(MD5(password) + pack("<Q", pid))` | K, P |
| Switch internal ticket format: random key in the clear beside the encrypted body, final key `MD5(target key + random key)` | `KerberosTicketVersion = 1` | K; PL implements both formats |
| Session key length | 32 bytes | K, KS |
| The source key's length is **not** a protocol constant. What must hold is that the key the ticket is encrypted with is the key advertised in the response; RC4 takes any length and the console is told the key rather than deriving it | 16 bytes in KS (retail SMB35), 32 bytes in N `mario-kart-8-deluxe/main.go` (live MK8D). Neither generalises, so it is `NEX_SOURCE_KEY_BYTES`. N's 32 comes from sha256 being 32 bytes wide, not from a stated console requirement | KS, N; conflict independently reached by the smb35 title agent |
| **The Switch byte-stream profile: 8-byte PIDs and structure headers on.** nex-go defaults to 4-byte PIDs and headers off, which is the Wii U/3DS wire. Per console, not per title | `PIDSize=8`, `UseStructureHeader=true` | N `mario-kart-8-deluxe`, reported by the mk8d title agent; the mechanism is visible in PL `nex-go/types/structure.go` |
| `UseStructureHeader` gates the structure header in **both** directions: `WriteHeaderTo` writes nothing when it is off, and `ExtractHeaderFrom` consumes nothing | — | PL `nex-go/types/structure.go` |
| PRUDP virtual ports are **learned from the SYN** — the server answers as the port the client targeted; there is no fixed value | — | N `earlier-nex/endpoint.go` |
| **Switch NEX is PRUDP over WebSocket**, not over UDP, and the console reaches it over `wss` | — | KS serves a WebSocket transport; corroborated by N's SMB35 server using a WSS listener, and by Golf's live SNI hostname being forwarded by Traefik |
| WebSocket transport and PRUDP Lite are one choice, not two: nex-go reads a packet as Lite only when a websocket server is present | — | PL `nex-go/prudp_server.go`, reported by the Golf title agent |
| PRUDP Lite's SYN/CONNECT ack never sets a minor version, so it answers 0 — which matches the retail secure server, and needs no knob | — | PL `nex-go`, reported by the Golf title agent; N had to force minor 0 because its own default was 5 |
| **Some titles put authentication and secure on one listener, separated by stream ID** — authentication stream 1, secure stream 2, same transport | — | KS (SMB35) |
| **Other titles use two WSS listeners on two ports**, with a real dialable secure station URL. So the shape is per title, not universal, and both are known to work | ARMS auth 443 / secure 60006; Golf auth 8457 / secure 61000 | N `arms/main.go`, `mario-golf-super-rush/main.go` — both live, Golf marked playable |
| Because of that, the secure station URL in the login response is a **placeholder** — the console does not dial it, it switches virtual port on the connection it already has | `address=0.0.0.1`, `port=1`, `sid=2` | KS |
| Full retail secure station URL shape for a Switch title | `prudps`, address `0.0.0.1`, port 1, `PID=<secure pid>`, `CID=1`, `type=2`, `sid=2`, `stream=10` | KS |
| A secure server PID need not be 2; a retail-observed constant exists | `257049437023956657` (SMB35) | KS |
| The login response's source key is 16 random bytes, hex-encoded — independently confirming the key inversion | — | KS |
| PRUDP-Lite CONNECT signature is a pure function of the access key and connection signature: `HMAC-MD5(MD5(accessKey), MD5(accessKey) + connectionSig)` | — | N `mario-party-superstars/README.md`, `mario-golf-super-rush/README.md` |
| Some consoles send a **ticketless (empty) CONNECT** to the secure server | — | N `earlier-nex/endpoint.go` |

## Identity

| Fact | Value | Source |
| --- | --- | --- |
| Login `extraData` is an `AuthenticationInfo` DataHolder whose `Token` field is a JWT | starts `eyJ` | N `earlier-nex/authbinding.go`; type confirmed in PL |
| **A real console sends a genuine BAAS id token with no custom claim.** Only an emulator build adds a server-specific claim to bind the account | — | N `earlier-nex/authbinding.go` |
| **`userName` on Switch is the console's NSA id, not the NEX PID.** It is a large decimal number (observed example on the order of 1.1e19, i.e. above 2^63) | — | N `earlier-nex/ticketgranting.go`, `datastore_smm2.go` |
| Titles later address players by that same login name rather than by the NEX PID, so the mapping must be remembered at login | Super Mario Maker 2's DataStore is the recorded case | N `earlier-nex/datastore_smm2.go` |
| The `AuthenticationInfo` framing varies across NEX versions and titles, so strict structural parsing of the token field is fragile | — | N `earlier-nex/authbinding.go` |
| The 918-byte token ceiling is **documentation only** — it is defined in PL but never enforced, and full JWTs are carried in practice | not a constraint | PL `nex-protocols-go/ticket-granting/constants`, grep shows no use; N carries whole JWTs |
| nx-baas internal identity API | `POST /internal/switch/identity`, `X-Internal-Key` | O `nx-baas/internal.go` |

## Per-title configuration (mechanism only — values live in each title's repository)

| Fact | Value | Source |
| --- | --- | --- |
| An access key is always exactly 8 hex digits | 2^32 search space | N `mario-party-superstars/README.md` |
| Access keys come from Kinnay's public Game Server List, from the game binary, or by brute-forcing one captured `(connectionSig, signature)` pair against the formula above | — | N title READMEs and `main.go` files |
| The `g<hex>-lp1.s.n.srv.nintendo.net` hostname encodes a **Game Server ID, not the access key** — the two are different values for the same title | — | N `mario-party-superstars/README.md` (records an earlier wrong theory and its correction) |
| NEX version is carried as an integer: `40605` is NEX 4.6.5 | `major*10000 + minor*100 + patch` | N title configuration across the fleet |
| Observed NEX versions span 4.0.0 to 4.6.5 across Switch titles | — | N title `main.go`/dashboard values |
| Super Mario Bros. 35 uses **method 6**, NEX version 40600, access key `0a69c592` | — | KS |
| The secure station URL scheme is per title: `prudps` for most, `prudp` for at least one major title. The wrong one lets menus work and then fails when the session is actually needed | — | N `arms/main.go`, `mario-party-superstars/main.go` |
| Pia's wire shape changed at Pia 5.19; titles before and after differ | — | N `mario-party-superstars/README.md` |

## Capability gap this repository closes

| Fact | Source |
| --- | --- |
| The Reference NEX core has **no general DataStore** — only one title-specific implementation. That stub is what leaves several DataStore-dependent titles incomplete there | N `earlier-nex` file listing; per-title READMEs describing DataStore as unstubbed |
| Pretendo's `nex-protocols-common-go/v2` ships a complete DataStore (get/change meta, prepare and complete post/get, search, rate, delete, S3 presigning) | PL `nex-protocols-common-go/datastore` |

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
