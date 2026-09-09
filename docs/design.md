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
`pSourceKey` appended. Both are implemented; which one a title uses depends on whether it
was built against NEX 4.4.0 or later, which we cannot know before we see the title.

### 2. Identity

`nx-baas` is the OpenPak Switch adapter and the only thing that can prove a console's BAAS id
token: the token carries an `openpak` claim signed with nx-baas's key. Its internal game API on
port 20070, guarded by `X-Internal-Key`, answers

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

The username a Switch sends in `userName` is documented as *"Username (your pid)"*. We treat
the **token** as the authority and refuse a login whose username parses as a different PID,
rather than trusting the client's claim. See the open questions.

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

## Open questions — what the next round needs

Everything below is unverified because it needs a real console, a real title and a capture.
None of it is guessed at in the code; where a fact is missing the code fails closed.

1. **Does a Switch NEX title actually send the BAAS id token we mint?** The whole identity
   chain assumes the token in `AuthenticationInfo.Token` is an nx-baas id token whose `openpak`
   claim we can prove. `openpakClaim` accepts either the full JWT or the bare claim, but if the
   console sends something else entirely — a per-title token from a BAAS endpoint we do not
   serve yet — the chain breaks at the first login. **Evidence needed:** a capture of the
   ticket-granting login from a console against a title, with the token field's shape.
2. **Token length.** `nex-protocols-go` notes an observed ceiling of 918 bytes on the token
   field. A full BAAS id token JWT may exceed it. **Evidence needed:** the byte length of the
   token in that same capture. If it does not fit, nx-baas must mint a short NEX-specific
   token and the `openpak` claim becomes the thing the console carries.
3. **The `userName` field.** We refuse a login whose username disagrees with the token's PID.
   That is right if the console's PID comes from our BAAS, and wrong if the console sends
   something derived from a Nintendo-side identifier. **Evidence needed:** the username string
   from the capture, next to the PID nx-baas holds for that account.
4. **Method 2 or method 6?** Depends on the title's NEX version, and each title differs.
   **Evidence needed:** the RMC method ID in the first login packet, plus
   `apiVersionGeneral` (e.g. 40601) so `NEX_VERSION` can be set correctly per title.
5. **The access key.** Per title, and not derivable — it comes from the game binary or from
   published reverse-engineering. **Evidence needed:** a documented access key per title we
   intend to host, recorded in that title's repository, not here.
6. **Whether the secure endpoint needs a stream ID other than 1.** Both endpoints use stream
   1 today, matching the Wii U/3DS shape. **Evidence needed:** the CONNECT packet's virtual
   port from the capture.

Until 1–4 are answered on hardware, "it starts, it serves `/healthz`, it completes a login
whose token nx-baas proves" is exactly as much as this repository can claim, and the tests
check only that.

## Sources

- [Pretendo wiki — NEX kerberos](https://nintendo-wiki.pretendo.network/docs/nex/kerberos)
- [Kinnay, NintendoClients wiki — Kerberos Authentication](https://github.com/kinnay/NintendoClients/wiki/Kerberos-Authentication)
- [Kinnay, NintendoClients wiki — Authentication Protocol (Ticket Granting)](https://github.com/kinnay/NintendoClients/wiki/Authentication-Protocol)
- [Kinnay, NintendoClients wiki — NEX Common Types](https://github.com/kinnay/NintendoClients/wiki/NEX-Common-Types)
- The three Pretendo libraries above, read as AGPL-3.0 source.
- OpenPak's own `nx-baas`, `servers/stardew-valley`, `nn-friends`, `ports.md`.
