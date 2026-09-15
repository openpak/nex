# Design

What Monster Hunter Generations Ultimate needs from a server, what this one does, what it
does not, and what would have to be measured to close the difference.

## The whole server is wiring

Two files under `cmd/mhgud`. `main.go` starts [`servers/nex`](../../../README.md) and registers the
protocols; `datastore.go` gives the DataStore protocol somewhere to keep its rows. Nothing
else exists, and nothing else should: PRUDP, kerberos, the login handshake and OpenPak
identity are `servers/nex`'s, and matchmaking, NAT traversal, unique IDs and every DataStore
method are Pretendo's `nex-protocols-common-go`. A title server that writes any of that
again is writing it worse.

## What MHGU is

A Switch port of a 3DS game, and its online code is the 3DS's. Two consequences drive
everything here (facts and sources in [provenance.md](provenance.md)):

**It talks raw PRUDP over UDP.** Native Switch NEX titles use PRUDP-Lite over a WebSocket;
MHGU's secure-connect code opens a UDP socket. That is `NEX_TRANSPORT=udp`, and it is the
`servers/nex` default, so there is nothing to set.

**Its ticket is the old one.** `KerberosTicketVersion = 0`: no random-key wrapper in front of
the encrypted body, 124 bytes instead of 148. `servers/nex` hardcodes version 1, which is
right for every native Switch title, so `main.go` sets 0 on both endpoints after `New`.

That second one is not a preference. On the other stack this title never got a packet to the
secure port: the client raises `2306-0116` (Core::BufferOverflow, "buffer too large to
send") during its secure CONNECT, before transmitting, and the kerberos ticket it echoes back
is the thing in the packet whose size we control. Ticket version 0 is the only lever anyone
has found. **Whether it is enough is unknown** — nobody has run this title against version 0
and watched.

## What is implemented

Registered on the secure endpoint, all of them Pretendo's common implementations:

| Protocol | What it gets |
| --- | --- |
| Match Making `0x15`, Matchmake Extension `0x6D`, Match Making Ext `0x32` | Real matchmaking against Postgres: sessions, gatherings, participation, communities, notifications. Friend-only matchmaking answers from the player's real friend graph, because `GetUserFriendPIDs` calls nx-baas through `servers/nex`. |
| NAT Traversal `0x03` | Probe initiation, NAT property and traversal reporting. |
| Utility `0x6E` | NEX unique IDs, stored in Postgres. Integer and string settings answer an empty map. |
| DataStore `0x73` | The real thing — see below. |
| Ticket Granting `0x0A`, Secure Connection `0x0B` | From `servers/nex`, registered before this server sees the host. |

### DataStore, which is the point

The other stack has no DataStore. Its core's only `0x73` code is Super Mario Maker 2's, and
its MHGU server answers every DataStore call with an empty list — `GetMeta` (`0x73.8`) with
DataStore::NotFound — precisely so the game keeps walking and shows what it asks for next.

Here the common protocol's callbacks are implemented against Postgres (`datastore.objects`,
`datastore.ratings`) and an S3-compatible store, so these answer for real:

- `GetMeta` `0x8`, `GetMetas` `0x9`, `GetMetasMultipleParam` `0x24` — metadata by data ID, by
  persistence slot, with access passwords and permission checks.
- `PreparePostObject` `0x18`, `CompletePostObject` `0x1A`, `CompletePostObjects` `0x25` —
  reserve a data ID, hand the console a presigned upload, confirm the size that arrived.
- `PrepareGetObject` `0x19` — presigned download for an object the caller may read.
- `SearchObject` `0xC` — owner, data type, tags, created/updated windows, ordering, range.
- `RateObject` `0xF`, `RateObjects` `0x28` — rating slots with ranges and initial values.
- `ChangeMeta` `0x26`, `PostMetaBinary` `0x15`, `DeleteObject` `0x4`.

Permission (public / friends-only / named recipients / owner-only) is enforced by the common
protocol, and the friends case is answered by OpenPak's own friend graph.

Object bodies live in the bucket, not in Postgres. Without `S3_ENDPOINT` the server still
starts and the metadata half still works; the transfer half answers `Core::NotImplemented`.
That is deliberate: nothing else in this server needs a bucket, and refusing to start would
cost the matchmaking too.

## What is not implemented, and why

- **Ranking `0x70`, Ranking2, Messaging, Message Delivery, Matchmake Referee.** No evidence
  MHGU calls any of them. The binary's SDK strings name DataStore and Utility; the rest would
  be code written for a call that may never come.
- **Anything MHGU-specific.** No hunt-specific matchmaking rules, no per-title search
  behaviour, no guild-card semantics. Nobody has measured a single MHGU RMC call. Writing
  title behaviour now would be guessing with a compiler.
- **Object expiry.** `period` is stored and reported; nothing sweeps expired rows.
- **A per-user rating log.** One account can rate one slot repeatedly.
- **Its own metrics, dashboard or admin surface.** `/healthz` on `NEX_PORT_BASE+2`, and
  Postgres is queryable.

Each of those is one `// ponytail:` comment in the code where it bites.

## Assumptions a capture would settle

1. **Does ticket version 0 clear `2306-0116`?** The whole title is blocked behind this on the
   other stack. **Evidence:** a run against this server; either the secure endpoint sees a
   CONNECT, or the console shows the same error and the ticket is not the only oversize part.
2. **Which login method, and does the console send a token we can prove?** `servers/nex`
   implements methods 2 and 6 and requires an nx-baas-provable id token. A 3DS-lineage title
   may well send method 1, or send something else in `AuthenticationInfo`. **Evidence:** the
   RMC method ID and the token field of the first login packet.
3. **The station URL type flags.** We set BehindNAT|Public (3) rather than the OpenPak
   default of Public (2), on the other stack's reading of what a 3DS-era Pia wants. Neither
   value has been observed working. **Evidence:** whether the console dials the secure port
   at all, and what it does with the station it is handed.
4. **Which DataStore methods MHGU actually calls, and with what data types.** The store is
   generic because the title's use of it is unmeasured. **Evidence:** the first rows a
   console writes; every field it sets is in the table.
5. **`SearchObject` filtering conventions.** `searchQuery` treats data type `0xFFFF`, a zero
   `ReferDataID`, a zero `DateTime` and an empty list as "do not filter", and ignores
   `SearchTarget`, `OwnerType` and `MinimalRatingFrequency`. **Evidence:** a real search
   parameter from the client.
6. **Stream IDs.** Both endpoints bind stream 1, the Wii U/3DS shape, which matches this
   title's lineage. **Evidence:** the virtual port in the CONNECT packet.

Until 1 and 2 are answered on hardware, what this repository can claim is: it starts, it
serves `/healthz`, it listens on its block, it creates its schema, and its DataStore round
trips against a real Postgres. The tests check exactly that much.
