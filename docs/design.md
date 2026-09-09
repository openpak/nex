# Design

What this repository is, what it deliberately is not, and where the honest gaps are.

## Not a NEX implementation

NEX is Nintendo's fork of Quazal Rendez-Vous: PRUDP over UDP, RMC on top of it, kerberos
tickets, and a few dozen service protocols. Pretendo has implemented all of that under
AGPL-3.0, which is OpenPak's own licence:

- [`nex-go/v2`](https://github.com/PretendoNetwork/nex-go) — PRUDP v0/v1/lite, connections,
  fragmentation, kerberos tickets, RMC.
- [`nex-protocols-go/v2`](https://github.com/PretendoNetwork/nex-protocols-go) — the wire
  types and method dispatch for ticket-granting, secure-connection, match-making,
  matchmake-extension, datastore, ranking, ranking2, nat-traversal, utility, messaging,
  notifications and the rest.
- [`nex-protocols-common-go/v2`](https://github.com/PretendoNetwork/nex-protocols-common-go)
  — working handler implementations for the parts every title shares.

Writing a fourth one would be a year of work to arrive at the same place. `servers/nex`
consumes those three and adds only what OpenPak has that Pretendo does not: our accounts,
our port plan, our deploy shape. `research-third-party-networks.md` records the decision.

## What nex-go gives us

A `PRUDPServer` owns a UDP socket and an access key; a `PRUDPEndPoint` bound to it owns the
connections on one stream. The library does the SYN/CONNECT handshake, and on CONNECT it
decrypts the kerberos ticket the client presents, checks it against the server account and
sets the connection's PID. Everything a title protocol later reads about "who is calling"
comes from that. Two callbacks are the only thing the library asks of us:

```go
endpoint.AccountDetailsByPID      func(pid types.PID) (*nex.Account, *nex.Error)
endpoint.AccountDetailsByUsername func(username string) (*nex.Account, *nex.Error)
```

`nex.Account` carries a PID, a username, a 16-character password, and `RequiresTokenAuth`,
which nex-go's own comment describes as *"Always false for special accounts or user accounts
pre-Switch"* — the hook for the Switch story below.

## The gap OpenPak has to fill

### 1. Switch login is not implemented upstream

The ticket-granting protocol has one set of method IDs and two sets of names
([Kinnay, Authentication Protocol](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol)):

| ID | Wii U / 3DS name | Switch name |
| --- | --- | --- |
| 1 | `Login` | `ValidateAndRequestTicket` |
| 2 | `LoginEx` | `ValidateAndRequestTicketWithCustomData` |
| 3 | `RequestTicket` | `RequestTicket` |
| 6 | `LoginWithContext` | `ValidateAndRequestTicketWithParam` |

`nex-protocols-go` routes method 1, 2 and 6 to the Switch handlers whenever the server's
library version is `>= 4.0.0`. `nex-protocols-common-go` sets handlers for `Login`, `LoginEx`
and `RequestTicket` only. So on a NEX 4 title **none of the common login handlers are
reachable** and the server answers `Core::NotImplemented` to every login. That is the first
thing this repository had to write: `login.go` implements methods 2 and 6.

Why token auth changes the shape. On 3DS and Wii U a NEX account has a password, and both
sides derive the kerberos key from it — *"the password is MD5-hashed `65000 + pid % 1024`
times"* ([Kinnay, Kerberos Authentication](https://github.com/kinnay/NintendoClients/wiki/Kerberos-Authentication),
which is `nex.DeriveKerberosKey`). A Switch has no such password. Instead *"normal users must
provide an id token to log in. If an id token is not provided, or if it is invalid, the server
refuses to hand out a ticket"*, and the login response carries a `sourceKey` field described
as *"Kerberos key. If present, key derivation is skipped and this key is used to decrypt the
ticket instead."*

That inverts the trust: the server picks the source key and tells the client. We generate 16
random bytes per login. No account password ever crosses the wire, and a stolen ticket is
useless past the two minutes nex-go allows.

The ticket itself is unchanged and built from nex-go primitives (`login.go: issueTicket`):
an outer ticket encrypted to the caller under the source key, holding a 32-byte session key,
the secure server's PID, and an internal ticket encrypted to the secure server. Switch uses
the newer internal format — a random key in the clear beside the encrypted body, final key
`MD5(target key + random key)` — which is `PRUDPServer.KerberosTicketVersion = 1`.

Method 6's response is the `ValidateAndRequestTicketResult` structure with no leading
`%retval%`; errors come back as RMC errors. Method 2 keeps the older flat response
(`%retval%`, `pidPrincipal`, `pbufResponse`, `pConnectionData`, `strReturnMsg`) with
`pSourceKey` appended. Both are implemented; which one a title uses follows its NEX version,
and the Switch fleet spans 4.0.0 to 4.6.5.

One detail is not optional: **method 6's response must be wrapped in a structure header with
version 1.** A client handed the flat method-2 layout rejects the login as
`Core::InvalidArgument` and the console shows 2306-0116. The first version of this file got
that wrong.

### 2. Identity

`nx-baas` is the OpenPak Switch adapter and the only thing that can prove a console's BAAS id
token — it holds the signing keys and this server deliberately does not. Two token shapes
arrive: an emulator adds a binding claim only nx-baas can sign, while a real console sends a
plain BAAS id token with no such claim. Both go to nx-baas. Its internal game API on port
20070, guarded by `X-Internal-Key`, answers

```
POST /internal/switch/identity   {"openpak": "nx2…"}  or  {"pid": 1800000042}
-> {"pid", "baas_user_id", "nickname", "friends": [{"pid", "baas_user_id", "nickname"}]}
```

`servers/stardew-valley` already proves NPLN logins through exactly this door, so a player is
the same person to a NEX title and an NPLN title, and a NEX title gets the friend graph for
free — no per-title friend table.

`account.go` wires that to nex-go's two callbacks. A user account comes back with
`RequiresTokenAuth: true` and a throwaway password, which is safe precisely because the token
path never derives a key from it.

The wiki documents `userName` as *"Username (your pid)"*, and that reading is a trap. On a
Switch the name is the console's **NSA id**, a decimal number that is never equal to the NEX
PID. The first version of this file compared the two and refused the login when they
disagreed, which would have refused every real console. The token is the only authority on
identity; the name is recorded at login instead, because titles go on to ask for players by
the name they logged in with and nothing else can translate it back
(`Host.PIDForLoginName`).

### 3. Ports, health, deploy

One block of ten per title from `ports.md`: `+0` authentication, `+1` secure, `+2` HTTP.
`NEX_PORT_BASE` is validated at startup against the two NEX ranges, so a typo fails loudly
instead of listening somewhere harmless-looking.

nex-go offers `EnableBasicUDPHealthCheck`, a UDP echo. ports.md rule: nothing infers liveness
from a UDP socket. So `Run` serves plain HTTP `/healthz` on `+2` and the UDP echo is unused.

Dockerfile and `.github/workflows/release.yml` match `servers/stardew-valley`: a tag push
builds `ghcr.io/openpak/nex` for amd64 and arm64, and the deploy host's
`podman-auto-update` does the rest.

## How a title server uses this

```go
package main

import (
	"log"

	nexhost "openpak/nex"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
)

func main() {
	host, err := nexhost.New(nexhost.FromEnv())
	if err != nil {
		log.Fatal(err)
	}

	protocol := matchmake_extension.NewProtocol()
	common_matchmake_extension.NewCommonProtocol(protocol)
	host.Secure.RegisterServiceProtocol(protocol)

	log.Fatal(host.Run())
}
```

`host.Auth` and `host.Secure` are the bare nex-go endpoints. There is no wrapper around
`RegisterServiceProtocol`, no plugin registry and no protocol abstraction: a title that needs
something nex-go can express talks to nex-go. `host.PlayerByPID` is there for the one thing
nex-go cannot answer — who this PID is on OpenPak, and who their friends are.

`cmd/nexd` is that same main minus the protocol registration, so a bare host can be deployed
to answer "does a console reach us and complete the handshake" before any title work starts.

## What DataStore needs, and why it is worth saying out loud

Several Switch titles put their whole online mode in DataStore — user-generated levels,
replays, ghost data, shared photos. `nex-protocols-common-go` ships a complete implementation
of it: get and change meta, prepare and complete post and get, search, rate, delete, and S3
presigned upload and download URLs. Nothing in this repository wraps it; a title constructs it
and registers it on `host.Secure` like any other protocol.

The one callback that does not work out of the box upstream is friends-only object
permissions, because it asks for a friend list by 32-bit PID and upstream has no Switch friend
graph to answer with. OpenPak does, through nx-baas, so `Host.FriendPIDs` fills it:

```go
protocol := datastore.NewProtocol()
common := common_datastore.NewCommonProtocol(protocol)
common.S3Bucket = os.Getenv("NEX_S3_BUCKET")
common.S3Presigner = myPresigner              // ports.md 23010 locally
common.GetUserFriendPIDs = host.FriendPIDs    // friends-only permissions now work
common.GetObjectInfoByDataID = myStore.MetaByDataID // the title's own storage
host.Secure.RegisterServiceProtocol(protocol)
```

The remaining callbacks are the title's object storage and are title-specific by nature — a
level's metadata is not shaped like a replay's. This repository deliberately does not invent a
common schema for them.

`Host.PIDForLoginName` matters here too. A DataStore request for "this console's own objects"
arrives naming the player by the name they logged in with, not by NEX PID, so a title that
looks the request up by PID finds nothing and tells the player their uploads are gone. The
login records the translation; DataStore reads it back.

## What is settled, and what is not

Facts taken from the existing earlier servers, from Kinnay's wiki and from the Pretendo
libraries are recorded with their sources in [`provenance.md`](provenance.md). The four
questions that blocked the first round are answered there:

- **The token.** `AuthenticationInfo.Token` is a JWT. An emulator adds a binding claim only
  nx-baas can sign; a real console sends a plain BAAS id token with no such claim. Both are
  forwarded to nx-baas. The 918-byte ceiling turned out to be documentation only — it is never
  enforced, and whole JWTs are carried in practice — so no short token needs minting.
- **The username.** It is the console's NSA id, not the NEX PID, and the two are never equal.
  The first version refused a login when they disagreed, which would have refused every real
  console. That check is gone; the name is recorded and translated instead.
- **Method 2 or 6.** Both, depending on the title's NEX version, which spans 4.0.0 to 4.6.5
  across the Switch fleet. Both are implemented. Method 6's response must carry a structure
  header with version 1 — the flat method-2 layout is rejected as `Core::InvalidArgument`,
  console error 2306-0116. That was the second bug, and it is fixed and pinned by a comment.
- **Virtual ports.** Learned from the client's SYN rather than fixed. nex-go binds an endpoint
  per stream ID and drops packets for an unbound one, so `NEX_STREAM_ID` exists and defaults
  to 1.

Two things per title cannot be settled here and belong in the title's own repository: its
**access key** (always 8 hex digits; from Kinnay's public Game Server List, from the binary,
or by brute-forcing one captured handshake pair against
`HMAC-MD5(MD5(key), MD5(key)+connectionSig)`) and its **NEX version**. Note that the
`g<hex>-lp1.s.n.srv.nintendo.net` hostname encodes a Game Server ID, which is a different
value from the access key for the same title.

`NEX_STATION_SCHEME` is per title and untested by us: `prudps` for most, `prudp` for at least
one major title. Getting it wrong is not a clean failure — menus keep working and the title
gives up later, when it actually needs the session.

### The one blocking dependency

`nx-baas` cannot yet prove a real console's BAAS id token. Its internal identity API takes the
emulator's binding claim or an already-proven PID. `Host.PlayerByToken` already sends the whole
token as `id_token` alongside the claim, so the wire is ready, but until nx-baas answers on
that field a genuine console's login is refused — correctly, but refused. That is a change in
the nx-baas repository, not this one, and it is the next thing standing between this host and
a console completing a login.

## Sources

- [Pretendo wiki — NEX kerberos](https://nintendo-wiki.pretendo.network/docs/nex/kerberos)
- [Kinnay, NintendoClients wiki — Kerberos Authentication](https://github.com/kinnay/NintendoClients/wiki/Kerberos-Authentication)
- [Kinnay, NintendoClients wiki — Authentication Protocol (Ticket Granting)](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol)
- [Kinnay, NintendoClients wiki — NEX Common Types](https://github.com/kinnay/NintendoClients/wiki/NEX-Common-Types)
- The three Pretendo libraries above, read as AGPL-3.0 source.
- OpenPak's own `nx-baas`, `servers/stardew-valley`, `nn-friends`, `ports.md`.
