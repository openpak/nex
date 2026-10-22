# Design

What Animal Crossing: New Horizons needs from a server, what is here, and what is not.

## What this repository is

One binary that registers ACNH's protocol set on the shared NEX host and gets out of the way.
`openpak/nex` owns PRUDP, kerberos, the Switch ticket-granting methods, SecureConnection, the
nx-baas account lookup, the port block and `/healthz`. Pretendo's `nex-protocols-common-go/v2`
owns the handlers for the standard protocols below. What is left — and all this repository
actually is — is three things:

1. Which protocols to register, and in what order.
2. The friend lookup, so the "visit a friend" menu populates.
3. A DataStore backing, because that is the one thing the common library deliberately leaves
   to the operator.

`cmd/acnhd/main.go` is the whole of 1 and 2. `cmd/acnhd/datastore.go` and `search.go` are 3,
adapted from the OpenPak sibling `servers/arms`.

## What ACNH needs

ACNH online is peer-to-peer island visits arranged by the server. A console logs in, registers
its station URLs, then either opens its own island as a gathering (optionally behind a Dodo
Code) or finds one to visit — a friend's island (located by participant PID) or a public island
(browsed by Dodo Code). Once matched, NAT traversal gets two consoles talking directly and the
server stops mattering until someone leaves.

That maps onto:

| Protocol | ID | Why ACNH needs it |
| --- | --- | --- |
| MatchMaking | `0x15` | the gathering lifecycle: unregister, find by ID, session URLs, host migration |
| MatchMakingExt | `0x32` | leaving a gathering |
| MatchmakeExtension | `0x6D` | the island session: create, join, browse by Dodo Code, **and the two ACNH-only calls in Gaps below** |
| NATTraversal | `0x03` | probe requests and NAT property reports so two consoles can reach each other |
| Utility | `0x6E` | NEX unique IDs, **plus two ACNH-only methods in Gaps** |
| DataStore | `0x73` | island/dodo/best-friend data — see below |

## Transport

Every `openpak/nex` default is right for ACNH — `provenance.md` carries the evidence for each:
`websocket` transport, `prudps` scheme, two-listener stream `1`, 32-byte source key,
`SecurePID=2`. The only non-empty settings are the ones every title has: `NEX_PORT_BASE`,
`NEX_ACCESS_KEY`, `NEX_VERSION`, and the nx-baas / database URLs. This is worth stating because
none of those defaults is right for *every* Switch title, and the wrong `NEX_SECURE_STREAM_ID`
in particular is only checkable by the log line `trying to connect to unbound PRUDPEndPoint`.

## State

Matchmaking, NEX unique IDs and DataStore metadata are all in Postgres, because
`nex-protocols-common-go` keeps them there. The libraries create their own `matchmaking`,
`tracking` and `utility` schemas on first run; `datastore.go` creates `acnh`. `ACNH_DATABASE_URL`
is required and startup pings it: a NEX title server without matchmaking is a lobby the game
cannot enter, so failing loudly beats starting.

## DataStore — real, but ACNH's usage is unmapped

ACNH's DataStore usage was never mapped from captures, so the cheap option would be a
"discovery" stub on `0x73` — log every call, answer empty-success. This server implements the
real handlers instead, over
`nex-protocols-common-go/v2/datastore`:

| Call | Method | What now happens |
| --- | --- | --- |
| `GetMeta` | 1 | reads the object by data ID, or by owner + persistence slot |
| `GetMetas` | 9 | the same, for a list of data IDs |
| `PreparePostObject` | 11 | creates the metadata row, returns a presigned S3 POST |
| `PrepareGetObject` | 12 | returns a presigned S3 GET for an existing object |
| `CompletePostObject` | 13 | checks the uploaded body's size, then makes the object visible |
| `DeleteObject` | 14 | soft-deletes |
| `ChangeMeta` | 27 | updates name, period, data type, meta binary |
| `RateObject` | 32 | applies a vote to a rating slot and returns the new totals |
| `SearchObject` | 15 | a real query — owner/friend/public filters, data type, tags, refer-id, date bounds, whitelisted sort, paged with `count(*) OVER ()` |
| `PostMetaBinary` | 10 | creates a metadata-only object |
| `GetMetasMultipleParam`, `CompletePostObjects`, `RateObjects` | 33, 34, 35 | the batch forms, on the same callbacks |

Object bodies never pass through this process: `PreparePostObject` / `PrepareGetObject` hand the
console a presigned URL and it talks to the object store directly. **S3 is optional** — with
`ACNH_S3_ENDPOINT` unset, every metadata call still works and the three body-moving calls answer
`Core::NotImplemented` (which is what the library does for an unset callback anyway), so the
server runs on a laptop with only Postgres. A half-configured store fails at startup.

**The caveat that matters:** because ACNH's `0x73` usage was never mapped, we do not know which
methods it calls, or with what data types and tags. The implementation is the general one. A
method ACNH makes that the common library does not model will now return a NEX error rather than
the discovery stub's empty-success — whether that regresses anything real is the first DataStore
question a capture answers.

### This backend is a lift candidate

`openpak/nex` has agreed to move the DataStore object store into a shared subpackage,
`github.com/openpak/nex/datastore`, behind a one-call `store.Wire(common, host)`. `datastore.go`
and `search.go` should be **deleted** when it lands: nothing in them is ACNH-shaped except the
schema name `acnh` and the S3 key bases, and `DataStoreMetaInfo` is defined by NEX, so the
columns are a transcription of the wire type. What must change on the way across is scoping —
this keeps rows in a per-title Postgres schema; the shared store wants a title-key column so one
database holds several titles without a search in one leaking objects from another. Until it
lands this stays, because ACNH needs a working DataStore and a discovery stub stores nothing.

## Identity and friends

`openpak/nex` resolves a console to an OpenPak account through nx-baas and hands back the
player's friend graph with it. `host.FriendPIDs` is assigned to both the matchmaking manager's
`GetUserFriendPIDs` (which is what fills the "visit a friend" menu — friend-notification-data
lookups ask by PID) and DataStore's. ACNH keeps no friend table; a player is the same person
here as in any other OpenPak title. The lookup fails closed, so a friends-only view stays shut
on an error rather than opening to everyone, and it drops a friend whose PID does not fit 32
bits rather than truncating it into a different account.

## Gaps — what ACNH needs that the common libraries do not provide

Each is verified absent from `nex-protocols-common-go@v2.6.1` and documented with method IDs,
wire shape and source in `provenance.md`. They answer `Core::NotImplemented` today rather than a
wrong guess, and each is either a future custom handler here or a convergence into `openpak/nex`.
None can be built correctly without a capture, because the details that make them work were
depend on live measurements of a working session that we do not have.

1. **`FindMatchmakeSessionByParticipant` (`0x6D.0x33`)** — locating a friend's island. Without
   it, "visit a friend" returns nothing. The response must be `{PID, session}` pairs carrying
   the full session **and** its 32-byte session key, or the visit stalls at 2618-0502.
2. **`UpdateMatchmakeSession` (`0x6D.0x2C`)** — writing a Dodo Code onto a live session. ACNH
   sends the code only here. The common library *does* store and search the code word when it is
   set at `CreateMatchmakeSession` time, so this gap bites only if ACNH sets the code post-create
   — which the facts say it does. A capture settles it.
3. **Utility `0x6E.9` / `0x6E.10`, Ranking `0x70.25`** — small non-standard overlays on the common handlers.
   Ranking is left entirely unregistered (its only known ACNH call is the unimplemented method
   25). Utility 1–8 work via the common library.
4. **Public P2P station `type=0x0B`** — an `openpak/nex` change. Pretendo's common
   SecureConnection hardcodes the peer's public station to `type=0x03` (`register_ex.go:92`),
   the Wii-U-era shape; ACNH's Pia 5.x needs the Pia-5.19 `0x0B` + `Pa` shape or the hole-punch
   does not build (2618-0502). This is the sibling `servers/arms`'s "nothing to set" inverted:
   `0x03` is right for arms's older Pia and wrong for ACNH's. **`openpak/nex` needs a per-title
   way to advertise the Pia-5.19 public-station flag.**

## What is not here, and why

- **Custom matchmaking handlers for the gaps above.** They depend on captures we do not have and
  on behaviours that can only be measured on a live session (`PublicStationFirst`, `JoinRespExistingCount`), and
  hand-writing them blind would guess at wire shapes.
  Registered common protocols + documented gaps is the honest scope until hardware speaks.
- **A monitoring dashboard, presence reporter, connection reaper.** OpenPak answers "is it up" with `/healthz` and "who is online" belongs to the account
  side. nex-go reaps its own dead connections.
- **Online gates** (verified-email, one-place-at-a-time, NSA allow-listing). Those
  are `nx-baas`/account policy, applied at identity resolution in `openpak/nex`, not here.

## Open questions — what the next round needs

In the order a first live run would answer them:

1. **Does ACNH reach us at all?** A wrong access key fails silently. **Evidence:** any packet at
   `22180`. And confirm the key is `v43a10em`, not the GSID `2EE2E300`.
2. **Method 2 or method 6?** `NEX_VERSION=40000` suggests method 2. **Evidence:** the RMC method
   ID of the first login.
3. **`SecureConnection::Register` (`0x0B.1`) or `RegisterEx` (`0x0B.4`)?** `openpak/nex` refuses
   plain `Register` unless `EnableInsecureRegister()` is called, which it does not expose. ACNH's
   Pia is newer than arms's, so `RegisterEx` is likely — but confirm. **Evidence:** the method ID
   of the first `0x0B` call.
4. **Does the peer's public station need `type=0x0B`?** The facts say yes and `openpak/nex`
   builds `0x03`. **Evidence:** the `type` field in the `InitiateProbe` / registered public
   station of a working retail session vs. ours; a visit stalling at 2618-0502 while the
   server-side punch succeeds is the signature.
5. **Which MatchmakeExtension methods does ACNH call, in what order, with what session fields?**
   Especially `0x33`, `0x2C`, and whether the code word is set at create or via `0x2C`.
   **Evidence:** the RMC trace of one host-open-island and one visit-a-friend flow.
6. **Which DataStore methods does ACNH call, and with what data types and tags?** **Evidence:**
   the `0x73` method IDs from a session that touches island/dodo/best-friend data.

Until 1 and 2 are answered on hardware, "it starts against a real Postgres, serves `/healthz` on
22182, listens two-listener on 22180/22181, creates its schemas, registers ACNH's standard
protocols, and its DataStore round-trips objects for real" is all this repository claims — and
that much has been run, not assumed (see the README).
