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
our port plan, our deploy shape. <https://github.com/openpak/docs/blob/main/research/research-third-party-networks.md> records the decision.

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

### 3. Transport and the shape of the two endpoints

A Switch NEX title does not speak PRUDP over UDP. It speaks **PRUDP over WebSocket**, and it
reaches both the authentication and the secure service **on one connection**, separated by
PRUDP virtual port: authentication on stream 1, secure on stream 2. The console never opens a
second socket, so the secure station URL in the login response is a placeholder — retail shape
is address `0.0.0.1`, port 1, with `sid=2` doing the actual steering.

nex-go already supports all of this: `ListenWebSocket` beside `Listen`, and a server that
routes each packet to the endpoint whose stream ID it is addressed to. So the single-listener
mode is *less* code than two servers, not more — it is one server with two endpoints bound.

`NEX_TRANSPORT` picks the transport, and picking websocket also picks PRUDP Lite — nex-go
reads a packet as Lite only when a websocket server is present, so they are one switch, not
two. A console connects over `wss`, and OpenPak runs both TLS shapes: Traefik terminating and
forwarding plain `ws`, or Traefik passing the SNI through to a listener that terminates
itself, which is what the 21xxx range in ports.md exists for. `NEX_TLS_CERT` and `NEX_TLS_KEY`
select the second; setting exactly one of them is refused, because a half-configured pair
listening plain fails as silence rather than as an error.

`NEX_SECURE_STREAM_ID` picks the shape: leave it equal to
`NEX_STREAM_ID` for two listeners on `+0` and `+1`, or set it different for one listener on
`+0`. That one is per title with no safe default — SMB35 uses the single listener, while ARMS,
Golf and Mario Party Superstars run two WSS listeners on two ports with a real secure URL, and
all four are known-working servers. They cannot both be one listener and the same stream, because nex-go refuses to bind two
endpoints with the same stream ID — so the one knob says everything, and the station URL
default follows it.

`NEX_SECURE_PID` exists because the secure server's PID is not always the conventional 2; it
is the ticket target, the station URL's `PID=`, and the source of pushed notifications. A
title that knows its retail constant sets it. A title that needs the whole URL to match
something observed overwrites `Host.SecureStationURL` before `Run`.

### 4. Ports, health, deploy

One block of ten per title from `ports.md`: `+0` authentication, `+1` secure, `+2` HTTP. In
single-listener mode `+1` is reserved and nothing binds it; the block still belongs to the
title.
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

	nexhost "github.com/openpak/nex"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
)

func main() {
	host, err := nexhost.New(nexhost.FromEnv())
	if err != nil {
		log.Fatal(err)
	}

	// Register FIRST, then build the common protocol. NewCommonProtocol reads the endpoint
	// off the protocol, and RegisterServiceProtocol is what sets it -- match-making type
	// asserts it and panics on nil.
	protocol := matchmake_extension.NewProtocol()
	host.Secure.RegisterServiceProtocol(protocol)
	common_matchmake_extension.NewCommonProtocol(protocol)

	log.Fatal(host.Run())
}
```

**Order matters, and getting it wrong is a panic, not an error.**
`NewCommonProtocol` reads the endpoint off the protocol it is handed, and nothing sets that
endpoint until `RegisterServiceProtocol` does. match-making type asserts it, so building the
common protocol first dies with `interface conversion: nex.EndpointInterface is nil`. Register,
then build.

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
graph to answer with. OpenPak does, through nx-baas, so `Host.FriendPIDs` fills it — and the
same function fills match-making's `GetUserFriendPIDs` for friend-only lobbies, which has the
same gap for the same reason:

```go
protocol := datastore.NewProtocol()
host.Secure.RegisterServiceProtocol(protocol) // must come first, see above
common := common_datastore.NewCommonProtocol(protocol)
common.S3Bucket = os.Getenv("NEX_S3_BUCKET")
common.S3Presigner = myPresigner              // ports.md 23010 locally
common.GetUserFriendPIDs = host.FriendPIDs    // friends-only permissions now work
common.GetObjectInfoByDataID = myStore.MetaByDataID // the title's own storage
```

The remaining callbacks are the title's object storage and are title-specific by nature — a
level's metadata is not shaped like a replay's. This repository deliberately does not invent a
common schema for them.

`Host.FriendPIDs` drops a friend whose PID does not fit 32 bits rather than truncating it.
Truncation would not lose a friend, it would name a *different* account and hand that stranger
a friend's access. OpenPak PIDs are nowhere near that today; the log line it emits is the
signal that they have outgrown the upstream callbacks.

### The shared object store (agreed, not yet built)

Four titles have now each written their own Postgres+S3 DataStore, which is the divergence
this repository exists to prevent. The backend moves here. What makes that reasonable is that
the schema is not ours to invent: `DataStoreMetaInfo` is defined by NEX — data id, owner PID,
size, name, data type, meta binary, permissions, ratings, tags, periods — so a generic store
over it is a faithful implementation, not a guess imposed on forty titles.

It lands as a **subpackage**, `github.com/openpak/nex/datastore`, not in the root package. Not
every title stores objects — a purely match-making title should not carry a Postgres and S3
client in its dependency graph because the root package imports one. (ARMS is *not* an example
of that: its base game links no DataStore client but its update does, and the update is what
goes online, so ARMS needs the store.)

The intended API is one call. A title gets a working store and keeps only what is genuinely
its own:

```go
store, err := nexds.Open(nexds.FromEnv())  // Postgres + S3 from env, migrations applied
if err != nil {
	log.Fatal(err)
}
defer store.Close()

protocol := datastore.NewProtocol()
host.Secure.RegisterServiceProtocol(protocol)          // register first, always
common := common_datastore.NewCommonProtocol(protocol)
store.Wire(common, host)                               // every generic callback, incl. FriendPIDs
```

`Wire` fills the whole generic surface: object metadata CRUD, search, ratings, permission
checks against `Host.FriendPIDs`, S3 presigned get and post, and the upload-completion
bookkeeping. It takes the `Host` because permissions and login-name translation both need it.

What stays in the title repository, because it is genuinely title-shaped:

- **The meaning of `metaBinary` and `dataType`.** The store carries those bytes and never
  parses them. A course, a ghost and a photo differ only there.
- **Custom protocol methods.** A title that extends DataStore registers its own handlers on
  the same `protocol` after `Wire`; anything it sets afterwards wins.

Rows are scoped by a title key so one Postgres can host several titles without objects
leaking between them — a title's search must never return another's. A schema per title is the
other way to get that, and it is what the first implementation did; a title key is preferred
here so one database can serve several deployments.

Two things in the upstream callback surface will silently corrupt or deadlock a store that
implements them naively. Both were found the hard way by the first implementation and are
confirmed in `nex-protocols-common-go` v2.6.1:

- **`UpdateObjectPeriodByDataIDWithPassword` and `UpdateObjectDataTypeByDataIDWithPassword`
  declare each other's parameter names.** The first takes a parameter spelled `dataType` and is
  called with `param.Period`; the second takes one spelled `period` and is called with
  `param.DataType`. The call sites are right and the signatures are misleading, so implement
  by callback name and ignore the parameter name, or the two columns get written swapped.
- **Log the method ID of every unimplemented DataStore call rather than guessing which to
  write.** The protocol carries on the order of a hundred methods and a title uses a handful;
  the existing implementation for the most DataStore-heavy title found its set by deploying
  with none of them and reading the method IDs the console asked for. The shared store should
  do that logging once, for everyone.
- **The persistence slot has no "not persistent" sentinel.** Pretendo's constructors default
  it to 0, which is also a legitimate slot, so the store has to choose a representation. One
  implementation used `0xFFFF` and flagged it as an assumption. If it is wrong and a title
  posts ordinary objects to slot 0, every object evicts the previous one — silent data loss,
  not a glitch.

  It cannot be settled by reading an existing server: the one title whose online mode is
  mostly DataStore has an implementation that never wrote the generic persistence path at all
  and records it as deferred. So this waits on a real `PreparePostObject` from a console, and
  the shared store should carry the assumption behind one named constant with this note
  attached, so that changing it later is a one-line change rather than an audit.
- **The search sort column cannot be a bound parameter**, since it lands in `ORDER BY`. It
  must map onto a fixed set of known columns, with a stable fallback for the sort values that
  have no column at all (rating slots), or the ordering is both unstable and an injection
  surface. Whitelist it and test every value.
- **`GetObjectInfoByDataID` must return an error for an object that has not been
  upload-completed.** `CompletePostObject` uses "did this return data" as its test for
  "already uploaded" and answers `PermissionDenied` when it does — so a store that happily
  returns incomplete objects makes completion fail forever. `GetObjectOwnerByDataID` and
  `GetObjectSizeByDataID` must still work on those same incomplete objects, because completion
  needs both.

Super Mario Maker 2 is the shape-defining case: it is the one title with a real implementation
to read for facts (course upload, download and search), and it is also the one that proves the
login-name point below, because its DataStore addresses players by the name they logged in
with rather than by PID.

`Host.PIDForLoginName` matters here too. A DataStore request for "this console's own objects"
arrives naming the player by the name they logged in with, not by NEX PID, so a title that
looks the request up by PID finds nothing and tells the player their uploads are gone. The
login records the translation; DataStore reads it back.

## What is settled, and what is not

Facts taken from earlier research notes, from Kinnay's wiki and from the Pretendo
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
