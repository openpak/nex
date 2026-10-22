# Design

What Super Smash Bros. Ultimate needs from a server, what this repository implements, what it
does not, and the open questions a first live run would settle.

## The shape of it

Three files of Go, one binary. `openpak/nex` brings the PRUDP/WebSocket transport, the kerberos
handshake, the Switch login and OpenPak account identity; `nex-protocols-common-go` brings
working implementations of every service protocol this title speaks. What is written here is
only what neither of them can know:

| File | What it is |
| --- | --- |
| `main.go` | which protocols the title speaks, and the order they are wired in |
| `datastore.go` | where DataStore objects live (Postgres + presigned S3) |
| `search.go` | the SearchObject WHERE clause |

There is no wrapper around any of it. A protocol the common library implements is registered
and left alone. `datastore.go`/`search.go` are a shared backend, not SSBU-specific code — see
the LIFT CANDIDATE note in `provenance.md`.

## What the title needs

The protocol set is the one the console was observed to use (see `provenance.md`): match-making,
matchmake-extension, match-making-ext, NAT traversal, Utility, DataStore, plus the
secure-connection and ticket-granting protocols `servers/nex` registers itself. Online play is
matchmaking-driven — quickplay, background matchmaking, and Battle Arenas — and peer to peer
once matched: nothing relays match traffic, and the server's job ends when two consoles have
each other's station URLs.

## The two protocols made real

This is the whole point of the port. A stub that answers every DataStore (`0x73`) and Utility
(`0x6E`) method with an empty list (`GetMeta` forced to `NotFound`) is enough for the online
menu to draw and not enough for online play to be complete. Both are implemented here.

**DataStore.** `nex-protocols-common-go/v2/datastore` implements the protocol and asks only
where objects live. `datastore.go` answers with Postgres for metadata, ratings and persistence
slots, and presigned S3 for object bodies so no body passes through this process. `GetMeta`,
`GetMetas`, `SearchObject`, `PreparePostObject`, `PrepareGetObject`, `CompletePostObject`,
`PostMetaBinary`, `ChangeMeta`, `RateObject` and `DeleteObject` all do what they say — the table
in `provenance.md` lists each. Persistence slots are real: a new
object posted into a slot retires the slot's previous occupant.

Object **bodies** need an S3-compatible bucket, which is optional: without `SSBU_S3_ENDPOINT`
the metadata calls all work and the three that move a body answer `Core::NotImplemented`, which
is what the library does for an unset callback anyway. What SSBU actually posts — replay blobs,
Arena data, or nothing — is not yet known, so this is deliberately not a startup requirement.

**Utility.** `nex-protocols-common-go/v2/utility` with `NewUtilityManager` over the same
Postgres implements the NEX-unique-ID getters SSBU asks for during bring-up. A stub returns an empty list; these return real, persistent IDs.

Access and update passwords: the DataStore backend stores them but nothing SSBU is known to
post sets one, so the checks read "no password, or the right one" and are marked in the source.

## What is not implemented

- **The Pia type-8 keepalive.** This is the highest-risk gap and it is deliberate. Earlier
  research measured SSBU's Pia 5.19 sending an undocumented PRUDP packet type 8
  every ~3s on the secure stream, and **soft-locking at the online menu** when it went un-ACKed;
  ACKing it unblocked the menu (earlier research notes). nex-go has no type-8 handler in its
  `packetHandlers` map, but `PRUDPEndPoint.RegisterCustomPacketHandler` is exported and
  `Host.Secure` reaches it, so a one-line ACK handler is possible without an upstream change.
  It is **not** built because we cannot yet confirm nex-go's current build does not already ACK
  type 8, nor that SSBU still soft-locks on our stack — building a handler blind risks
  double-ACKing or masking the real behaviour. One capture settles it. If a first run stalls at
  the online menu with type-8 packets arriving un-ACKed, this is the fix, and whether it belongs
  in `servers/nex` (per-console Pia behaviour) or here is the decision that capture also forces.
- **Ranking (`0x70`).** Not registered. A Ranking handler that refuses every standard method
  (`Core::NotImplemented`) was observed to still bring online up, so Ranking is not the blocker; SSBU's online is matchmaking, not a NEX leaderboard. If a capture shows a
  real `0x70` call, `common_ranking` is three lines over the store `servers/mario-tennis-aces`
  already worked out.
- **Matchmaking behaviour patches.** `servers/mario-tennis-aces` needed two: opening
  participation on session create (a "double host" deadlock), and answering
  `FindMatchmakeSessionByParticipant` (join a friend's room). Plain
  matchmaking handlers with neither patch were observed to work for SSBU, so neither is added here. If SSBU exhibits
  the double-host stall in quickplay or cannot join a friend's Arena, the fixes are the same
  shape as Mario Tennis Aces's, a few lines each.
- **Block lists, per-player rating history, Ranking time/group filters.** Not applicable or not
  reached: OpenPak has no block list, a player can rate the same object twice, and there is no
  Ranking here to filter.

## Two things that will decide whether it works on hardware

1. **The type-8 keepalive**, above. The most likely reason a first run stalls, and the one
   behaviour earlier research measured specifically for SSBU.
2. **`NEX_VERSION`.** `4.0.0` is a guess inherited from earlier research notes where it was a copied MK8
   template value, so it may not even be SSBU's own value. The console reports `apiVersionGeneral`;
   the first capture settles it, and `NEX_VERSION` takes the integer form directly (`40000`).

Everything else is checkable now: the server starts, serves `/healthz`, listens on its block,
creates its schema, registers the protocol set including a real DataStore and Utility, and its
DataStore queries have been run against a real Postgres.

## Sources

Every protocol fact cites its origin in [`provenance.md`](provenance.md).
