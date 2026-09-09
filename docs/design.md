# Design

What Luigi's Mansion 3 needs from a server, what is here, and what is not.

## What this repository is

One binary that registers Luigi's Mansion 3's protocol set on the shared NEX host and gets out
of the way. `servers/nex` owns PRUDP, kerberos, the Switch ticket-granting methods,
SecureConnection, the nx-baas account lookup, the port block and `/healthz`. Pretendo's
`nex-protocols-common-go` owns the handlers for the standard protocols below. What is left — and
all this repository actually is — is:

1. Which protocols to register, wired to one matchmaking manager with friend-only lobbies going
   to nx-baas.
2. Utility settings, because the game reads them the moment it connects.
3. A minimal acknowledgement of the game's own station/presence protocol (`0x79`).

`server.go` is 1 and 2. `presence.go` is 3. `utility.go` is the empty settings maps.

## What Luigi's Mansion 3 needs

Luigi's Mansion 3 online is co-op: ScreamPark (competitive-ish party co-op) and ScareScraper
(the tower co-op), both peer-to-peer play arranged by the server. A console logs in, registers
its station URLs, then creates or searches for a session; NAT traversal gets the consoles talking
directly. On top of the standard matchmaking, the game runs its own presence protocol so a player
can see and follow friends into a room.

That maps onto:

| Protocol | ID | Why the game needs it |
| --- | --- | --- |
| MatchMaking | `0x15` | the gathering lifecycle |
| MatchMakingExt | `0x32` | leaving a gathering |
| MatchmakeExtension | `0x6D` | the lobby: create, join, auto-matchmake, browse |
| NATTraversal | `0x03` | probe requests and NAT reports so two consoles can reach each other |
| Utility | `0x6E` | NEX unique IDs, and the integer settings read on connect |
| Station / presence | `0x79` | the game's own "who is here / where are my friends" protocol |

DataStore (`0x73`) is **not** registered. `provenance.md` records why: nothing observes Luigi's
Mansion 3 making a DataStore call. This is a matchmaking-only co-op title, so there is nothing to
back — and inventing a DataStore for a title that does not touch it would be pure speculation.

Ranking (`0x70`) is **not** registered either; see `provenance.md`.

## Transport

Nothing to set: every `servers/nex` default is already right for this title, and `provenance.md`
shows the evidence for each (websocket transport, two-listener secure stream 1, 32-byte source
key, secure PID 2, prudps station scheme). `server.go` sets the per-title facts as explicit
constants anyway, so the values live next to their provenance rather than relying on a default
that could change.

## State

Matchmaking and NEX unique IDs are in Postgres, because `nex-protocols-common-go` keeps them
there. The libraries create their own `matchmaking`, `tracking` and `utility` schemas on first
run. `LM3_DATABASE_URL` is required and startup pings it: a NEX title server without matchmaking
is not a degraded server, it is a lobby the game cannot enter, so failing loudly beats starting.

## Utility settings

`GetIntegerSettings` is called right after `SecureConnection::Register`, and a reply the client
cannot parse drops the connection about a second later. `nex-protocols-common-go` reads the
request correctly; the map's content is the operator's. No capture of this title's settings
exists, so the honest answer is a correctly shaped **empty** map for every index — right on the
wire, possibly incomplete in content. No env-configurable override is provided (the sibling
Mario Party Superstars has one if a capture ever needs it here).

## The `0x79` station / presence protocol — a stub, honestly

This is the one piece of Luigi's Mansion 3 that is genuinely its own and has no common
implementation. `earlier/lm3` handles four methods on `0x79`:

- **5 UpdateApplicationBuffer** and **13 UpdateState** are publish-only; the retail server
  answers them empty.
- **14 InitializeStation** ("I am connecting, who is here?") and **15 GetStationLocation**
  ("where are my friends?") answer with a list of station records — a friend's room ID, the Pia
  mesh station URL, a display name — assembled from what other players have published and from
  live connection state.

`presence.go` registers a stub that **acknowledges all four and implements none of their
content**: 5 and 13 answer empty, 14 and 15 answer an empty record list (`u32` count = 0),
anything else on `0x79` gets `Core::NotImplemented`. The reasoning is the same trade the rest of
the fleet makes for an unknown: an *unacknowledged* `0x79` call would get `Core::NotImplemented`,
which is the answer most likely to hard-stall the session, so acknowledging keeps the connection
alive and gives a first live run a target to observe. "No friends have a room open" (count = 0) is
a correct answer for a solo host and a plausible one for a first run — it is not the whole
protocol.

**Why the real implementation is deferred, not just skipped.** Two things block it, and both are
next-round work:

1. **A clean-room capture.** `earlier/lm3` reverse-engineered the populated record layout from a
   single retail capture — its comments quote the exact observed bytes. Reproducing that layout
   from *their* transcription would be structural copying, which the clean-room policy forbids.
   OpenPak needs to take its own capture of a Luigi's Mansion 3 session reaching the "with
   friends" screen and record the `0x79` wire shapes in this repo's own provenance before
   implementing them.
2. **Host surface.** Building the records means enumerating live connections and reading each
   one's registered Pia station URLs (and mapping a friend PID to its connection). `earlier/lm3`
   does this against its own core's `FindConnectionByPID` / `Stations()` / connection-ID
   accessors. `github.com/openpak/nex` (v0.2.1) exposes `Host.Secure` (a `*nex.PRUDPEndPoint`)
   but no convenience for "the station URLs this PID registered". nex-go's `PRUDPEndPoint` has the
   underlying connection registry, so this is reachable, but a first-class helper on the shared
   host — "give me the Pia stations for this PID" — is the clean place for it and would serve any
   title that grows a presence protocol. **Proposed `servers/nex` change: a `Host` method to look
   up a connected PID's registered station URLs.**

Until both land, friend-room *visibility* (seeing a friend's open room before joining) does not
work; standard matchmaking (auto-matchmake, create/join a session) does not depend on `0x79` and
is expected to.

## Identity and friends

`servers/nex` resolves a console to an OpenPak account through nx-baas and hands back the friend
graph with it. `host.FriendPIDs` is wired to the matchmaking manager's `GetUserFriendPIDs`, which
is the upstream callback documented as not working on Switch (it asks by 32-bit PID and Pretendo
has no Switch friend graph). OpenPak does, in nx-baas. That is the whole of friend-only lobbies;
this title keeps no friend table of its own.

## What is not here, and why

- **DataStore (`0x73`).** No DataStore call is observed. A matchmaking-only title.
- **Ranking (`0x70`).** No client is observed linking it. Registered defensively by
  `earlier/lm3`; three lines plus a database here when a capture says otherwise.
- **The populated `0x79` records.** Deferred; see above. This is the real remaining work.
- **A NAT-check / STUN responder and a PRUDP station relay.** `earlier/lm3` carries a large one
  for punching two firewalled NATs together with no port forwarding. On OpenPak, NAT traversal is
  `common_nat_traversal`, and a relay for the no-port-forwarding case is a deployment concern, not
  something baked into one title's game server.
- **A monitoring dashboard, presence reporter, ZNC handler, and the signed-token identity gate.**
  `earlier/lm3` has all of them. OpenPak answers "is it up" with `/healthz`, proves identity in
  `servers/nex`, and "who is online" belongs to the account side.

## Open questions — what the next round needs

In the order a first live run would answer them:

1. **Does the game reach us at all?** A wrong access key fails silently. **Evidence needed:** any
   packet arriving at the auth port.
2. **Method 2 or method 6, and which `apiVersionGeneral`?** `NEX_VERSION` is `4.0.0`. **Evidence
   needed:** the RMC method ID of the first login.
3. **`SecureConnection::Register` (`0x0B.1`) or `RegisterEx` (`0x0B.4`)?** `earlier/lm3` handles
   both, and this title's Pia is 5.17 (just below the 5.19 that reshaped this area), so plain
   `Register` is plausible. `nex-protocols-common-go` refuses plain `Register` unless
   `EnableInsecureRegister()` is called and `servers/nex` does not expose that toggle. **If the
   first `0x0B` call is method 1, `servers/nex` must expose `EnableInsecureRegister()`.**
4. **What does `0x79` actually carry?** The method IDs are known (5/13/14/15); the populated
   record layout is not, in the clean room. **Evidence needed:** an OpenPak capture of a session
   reaching the "with friends" screen — the request and response bytes of methods 13, 14 and 15.
5. **Does MatchMaking `0x15` methods 1 / 3 need special handling?** `earlier/lm3` intercepts them
   with empty success; this server lets the common handler answer. **Evidence needed:** whether
   matchmaking works on the OpenPak stack without the interception.

Until 1 and 2 are answered on hardware, "it starts against a real Postgres, serves `/healthz`,
registers Luigi's Mansion 3's protocols, and acknowledges its `0x79` presence calls" is all this
repository claims.
