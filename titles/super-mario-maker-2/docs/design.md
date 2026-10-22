# Design

What Super Mario Maker 2 needs from a server, what this repository implements, what it does
not, and where the Course World layer goes next.

## The shape of it

Four files of Go, one binary. `openpak/nex` brings the PRUDP/WebSocket transport, the kerberos
handshake, the Switch login and OpenPak account identity; `nex-protocols-common-go` brings
working implementations of the base service protocols. What is written here is only what
neither of them can know:

| File | What it is |
| --- | --- |
| `main.go` | which protocols the title speaks, and the order they are wired in |
| `datastore.go` | where DataStore objects (courses) live — Postgres metadata + presigned S3 bodies |
| `ranking.go` | where leaderboards live |
| `schema.go` | the two Postgres schemas those two own |

There is no wrapper around any of it. A protocol the common library implements is registered
and left alone. Unlike Mario Tennis Aces, this title has **no** hand-written matchmaking
behaviours: no measured quirk calls for one.

## Why SMM2 is the DataStore shape-defining case

SMM2's entire online mode *is* DataStore. Course World — publish a level, browse, search, rate,
comment, bookmark, super-worlds, per-maker profiles — is all protocol `0x73`. It is the most
DataStore-heavy title in the fleet and the natural reference for the shared object store.
Everything the shared store must eventually do shows up here first:

- **Object bodies are real.** A course's level BLOB is genuinely uploaded and downloaded
  (`PreparePostObject`/`PrepareGetObject`/`CompletePostObject`), so presigned S3 is not
  optional decoration the way it is for a metadata-only title.
- **Persistence slots are real and bounded.** A maker's own courses occupy per-owner slots
  capped at 32 ("32/32" on the save screen). Posting into an occupied slot replaces the
  previous occupant. This is the base-object persistence the store already implements, and
  SMM2 is its heaviest user.
- **Friends-only visibility** rides `host.FriendPIDs`, the OpenPak friend graph nx-baas fills.
- **A large title-specific method surface** sits on top of the base object methods, on the
  same protocol id. This is what the shared store has to accommodate that a plain base
  DataStore does not.

The full method map, the `UserInfo` profile object (revision 3, its stat tables and field
order), the four relation-upload types, and the NSA→PID lookup rule are all recorded in
[`provenance.md`](provenance.md) and were sent as the design input for the shared object store
(`github.com/openpak/nex/datastore`).

## DataStore: what is wired here

The base DataStore object layer, over Postgres (metadata, tags, ratings, persistence slots)
and presigned S3 (course bodies), through `nex-protocols-common-go`'s common protocol. These
base methods do what they say for SMM2:

| Method | Is, here |
| --- | --- |
| `GetMeta` (8), `GetMetas` (9) | a course's real metadata, ratings included |
| `PreparePostObject` (24) | a real course row + a presigned upload URL for the level BLOB |
| `CompletePostObject` (26) | the course marked ready (or dropped on failure) |
| `PrepareGetObject` (25) | a presigned download URL for a stored course |
| `SearchObject` (12) | courses matching owner, data type, tags, refer id and time range |
| `RateObject` (15) | a rating slot's running total |
| `DeleteObject` (4) | the course stops being found |

Persistence slots are real: a course posted into a slot replaces the slot's previous occupant,
and deletes it when the client asks. This is what a maker's own-course slots are built on.

## DataStore: what is NOT wired here, and why

SMM2's **title-specific** `0x73` methods — `GetUsers` (48), `RegisterUser` (47),
`SyncUserProfile` (49), `CanPostCourse` (60), the Course World browse/comment/world-map
methods, and the custom relation-upload preps (66, 132) — are **not** implemented in this
binary. Two reasons, both deliberate:

1. **They share protocol `0x73` with the base object methods.** A single protocol handler owns
   `0x73`; serving both the base methods (via the common implementation) and the SMM2 methods
   (our serialization) from one handler is exactly the job of the shared
   `github.com/openpak/nex/datastore` subpackage — which is not built yet. The team's decision
   for now is to register the common base protocol directly, which this does.
2. **`UserInfo` is a heavy, revision-pinned serialized type** (26 fields, nested structs,
   non-contiguous stat maps, a revision-3 tail). Writing its clean-room serialization belongs
   in the shared store, designed once around this case, not open-coded in one title's binary
   ahead of that store existing.

Consequence, stated honestly: with only the base layer wired, a real SMM2 client boots but
**Course World does not populate** — `GetUsers`/`SyncUserProfile` go unanswered, so the game
falls back to a generic Mii and an empty world. That is the same honesty bar as the sibling
titles: the base DataStore is *implemented and tested*; Course World is *specified and handed
to the shared store*, not yet *working on hardware*.

## Two things that will decide whether it works on hardware

1. **Secure PRUDP minor version 0.** A working server forced this on the secure endpoint, on a strong
   measurement (answering the client's offered version 6 with anything but 0 makes it skip its
   kerberos ticket, and SMM2 then dies at matchmaking while the menu still looks fine). That is
   a fact about that server's own NEX stack; whether Pretendo's `nex-go` needs the same forcing is
   **unverified** and is the top hardware risk. If it does, it is a one-line `servers/nex`
   change scoped to the secure endpoint — the auth endpoint must stay on its default or it
   stops receiving logins. See [`provenance.md`](provenance.md).
2. **`NEX_VERSION`.** `4.0.0` is inherited from a working server's runtime value. The console reports
   `apiVersionGeneral`; the first capture settles it, and `NEX_VERSION` takes the integer form
   directly (`40000`).

Everything else is checkable now: the server builds, vets, tests green (the DataStore and
Ranking queries are parsed and run against a real Postgres, including the persistence-slot
replacement), serves `/healthz`, and listens on its block.

## servers/nex changes this title may need

- **Secure PRUDP minor version 0** — see above; unverified, top risk.
- **An NSA→PID hook reachable from the DataStore store** — `GetUsers` (48) asks for profiles by
  NSA id, not PID. The shared store needs to reach `servers/nex`/nx-baas's identity map to
  translate before a profile lookup. Not needed for the base object layer wired today, but
  required the moment `GetUsers` is implemented.
- **`SecureConnection::TestConnectivity`** and the Pia type-8 keepalive are the same
  fleet-wide items noted on the siblings; no SMM2-specific evidence changes them.

## Sources

Every protocol fact cites its origin in [`provenance.md`](provenance.md).
