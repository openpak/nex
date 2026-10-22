# Design

What ARMS needs from a server, what is here, and what is not.

## What this repository is

One binary that registers ARMS's protocol set on the shared NEX host and gets out of the way.
`servers/nex` owns PRUDP, kerberos, the Switch ticket-granting methods, SecureConnection, the
nx-baas account lookup, the port block and `/healthz`. Pretendo's
`nex-protocols-common-go/v2` owns the handlers for every protocol below. What is left — and
all this repository actually is — is three things:

1. Which protocols to register, and in what order.
2. The friend lookup, so friend-list lobbies work.
3. A DataStore backing, because that is the one thing the common library deliberately leaves
   to the operator.

`cmd/armsd/main.go` is the whole of 1 and 2. `cmd/armsd/datastore.go` and `search.go` are 3.

## What ARMS needs

ARMS's online is peer-to-peer play arranged by the server. A console logs in, registers its
station URLs, then either creates a lobby or searches for one — Party Match, ranked, Party
Crash, or the friends list, which is the same search narrowed to sessions owned by people the
player knows. Once matched, NAT traversal gets two consoles talking directly and the server
stops mattering until someone leaves. The ARMS update adds a DataStore client on top for
event data.

That maps onto:

| Protocol | ID | Why ARMS needs it |
| --- | --- | --- |
| MatchMaking | `0x15` | the gathering lifecycle: unregister, find by ID, session URLs, host migration |
| MatchMakingExt | `0x32` | leaving a gathering |
| MatchmakeExtension | `0x6D` | the lobby itself: create, join, auto-matchmake, browse, application buffer |
| NATTraversal | `0x03` | probe requests and NAT property reports so two consoles can reach each other |
| Utility | `0x6E` | NEX unique IDs |
| DataStore | `0x73` | the update's event data |

Ranking (`0x70`) is **not** registered. `provenance.md` records why: no Ranking client is
linked in either ARMS build.

## Transport

Nothing to set: both `servers/nex` defaults are already right for ARMS. That is worth stating
because neither is right for *every* Switch title, and an earlier fleet-wide instruction to set
`NEX_SECURE_STREAM_ID=2` was retracted — it is a per-title fact, and ARMS's facts say otherwise.

**Transport is `websocket`**, now the default. Switch NEX titles speak PRUDP over WebSocket
rather than UDP, and
`provenance.md` records the evidence for ARMS specifically: it
listens WSS on both endpoints and opens no UDP socket at all. `servers/nex` defaults to `udp`,
so this is the one setting where the default is wrong for this title — and it is a silent
wrong, since nothing arrives on a socket the console never dials.

**The secure stream ID stays `1`**, the two-listener shape: authentication on `22140`, secure
on its own dialable `22141`. Two facts from ARMS itself say so — it
runs two separate WSS listeners (auth and secure), and the secure station URL it
hands the console carries `sid=1`. The single-listener form, `NEX_SECURE_STREAM_ID=2` with a
placeholder station URL, is used by Super Mario Bros. 35 and would break ARMS. If the stream ID
is ever wrong the host logs `trying to connect to unbound PRUDPEndPoint`, so this is checkable
rather than silent.

Everything else in the handshake is already right by default: `provenance.md` shows the secure
station URL `servers/nex` builds matches what the title expects, field for field.

## State

Matchmaking, NEX unique IDs and DataStore metadata are all in Postgres, because
`nex-protocols-common-go` keeps them there and writing a fourth in-memory matchmaker to avoid
one connection string would be more code, not less. The libraries create their own `matchmaking`,
`tracking` and `utility` schemas on first run; `datastore.go` creates `arms`.

`ARMS_DATABASE_URL` is required and startup pings it. A NEX title server without matchmaking
is not a degraded ARMS server, it is a lobby the game cannot enter, so failing loudly beats
starting.

## DataStore

A stub that answers every `0x73` call with an empty list and `0x73.8` with `DataStore::NotFound`
would not support Party Crash / FestaRanking data.

`nex-protocols-common-go` ships the real handlers. They need two things we supply: object
metadata storage, and an S3 client for object bodies. So the following are implemented here
rather than stubbed:

| Call | Method | What now happens |
| --- | --- | --- |
| `GetMeta` | 1 | reads the object by data ID, or by owner + persistence slot |
| `GetMetas` | 9 | the same, for a list of data IDs |
| `PreparePostObject` | 11 | creates the metadata row, returns a presigned S3 POST |
| `PrepareGetObject` | 12 | returns a presigned S3 GET for an existing object |
| `CompletePostObject` | 13 | checks the uploaded body's size against the declared one, then makes the object visible |
| `DeleteObject` | 14 | soft-deletes |
| `ChangeMeta` | 27 | updates name, period, data type, meta binary |
| `RateObject` | 32 | applies a vote to a rating slot and returns the new totals |
| `SearchObject` | 15 | a real query — see below |
| `PostMetaBinary` | 10 | creates a metadata-only object |
| `GetMetasMultipleParam`, `CompletePostObjects`, `RateObjects` | 33, 34, 35 | the batch forms, on the same callbacks |

Object bodies never pass through this process. `PreparePostObject` and `PrepareGetObject` hand
the console a presigned URL and the console talks to the object store directly, which is what
the retail DataStore did and what the library is built for.

**S3 is optional.** With `ARMS_S3_ENDPOINT` unset, every metadata call above still works and
the three that move a body answer `Core::NotImplemented` — which is exactly what the library
does for a callback nobody set, so this is not a special case, just an unset one. That keeps
the server runnable on a laptop with only Postgres. A half-configured store (endpoint set,
bucket or credentials missing) is a startup error rather than a surprise at the first upload.

### This backend is a lift candidate, not a permanent home

`servers/nex` has since agreed to move the DataStore object store into a shared subpackage,
`github.com/openpak/nex/datastore`, with a one-call `store.Wire(common, host)` filling the
generic surface. That is the right call and this code should be deleted when it lands.

Nothing here is ARMS-shaped, which is what makes it liftable: the only title-specific values
are the schema name `arms` and the S3 key bases. `DataStoreMetaInfo` is defined by NEX, so
these columns are a transcription of the wire type rather than a schema anyone invented. What
would have to change on the way across is scoping — this keeps rows in a per-title Postgres
schema, and the shared store wants a title-key column so one database can hold several titles
without a search in one leaking objects from another.

Until it lands this stays, because ARMS needs a working DataStore and a stub is what we are
replacing.

### Search

`searchQuery` in `search.go` translates a `DataStoreSearchParam` into one SELECT with a
`count(*) OVER ()` for the unpaged total, so a search is one round trip. It handles owner
filters, the friend-owned and public-access search types, data type (both the single-value
and the NEX 4 list form), tags, refer-data-ID, and the four created/updated bounds; it orders
by a column from a fixed map and pages with the client's `ResultRange`.

Two deliberate limits, both marked in the code:

- Rating sort columns (`SearchSortColumnRating*`, `SearchSortColumnRatingAverage*`) fall back
  to `data_id`. They need a join per slot and no ARMS search has been seen asking for one.
- Search types this code does not translate into SQL simply do not narrow the query. That is
  safe rather than lax: `nex-protocols-common-go` re-checks every returned object against the
  caller's permissions before it goes on the wire, so an untranslated search type costs a
  wasted row, never a leaked one.

Everything a console sends is a bound parameter. The only text a console influences is the
ORDER BY column, and that comes out of the map or falls back. `search_test.go` checks exactly
that: placeholder count matches argument count, no sent value appears as text in the query,
an unknown sort column falls back, and the page size is capped.

## Identity and friends

`servers/nex` resolves a console to an OpenPak account through nx-baas and hands back the
player's friend graph with it. `host.FriendPIDs` is assigned to both the matchmaking manager's
`GetUserFriendPIDs` and DataStore's, which are the two upstream callbacks documented as not
working on Switch for the same reason: they ask by 32-bit PID and Pretendo has no Switch friend
graph. OpenPak does. That is the whole of "Friends List lobby joining" — ARMS keeps no friend
table, and a player is the same person to this title as to an NPLN one.

Use the host's function, not a local one. It fails closed on a lookup error, so a friends-only
lobby stays shut rather than opening to everyone, and it *drops* a friend whose PID does not
fit 32 bits instead of truncating it — truncation would not lose a friend, it would name a
different account and hand that stranger a friend's access.

`needsFriends` keeps the nx-baas call off every search that does not need the graph.

One thing to check if ARMS DataStore searches ever come back wrongly empty: a Switch logs in
under its **NSA id**, not its PID, and some titles then name a player by that login name rather
than by PID (Super Mario Maker 2's DataStore is the recorded case). None of the DataStore
callbacks this server sets receives a name — every one of them is given a PID — so there is
nothing to translate today. `Host.PIDForLoginName` is the hook if a capture shows otherwise.

## What is not here, and why

- **Ranking (`0x70`).** No client links it. Three lines when a capture says otherwise.
- **Messaging, notifications, message delivery.** Nothing in ARMS is known to use them.
- **A Pia type-8 keepalive handler.** `provenance.md` explains: it mattered for Super Smash
  Bros. Ultimate, and whether ARMS's older Pia sends one at all is unknown. Adding a handler
  for a packet the title may never send is a thing to maintain in exchange for nothing.
- **A monitoring dashboard and a presence reporter.** OpenPak
  answers "is it up" with `/healthz` and "who is online" belongs to the account side.
- **A Dockerfile and a release workflow.** Not written until the server has been run against
  something. `servers/nex`'s are the shape to copy.

## Open questions — what the next round needs

`servers/nex`'s design doc lists six that apply to every NEX title. These are the ones that
are ARMS's own, in the order a first live run would answer them:

1. **Does ARMS reach us at all?** A wrong access key fails silently: the PRUDP-Lite signature
   does not match, the console retries forever, neither side logs. **Evidence needed:** any
   packet arriving at `22140/udp`.
2. **Method 2 or method 6, and which `apiVersionGeneral`?** `NEX_VERSION` is `4.3.5` from the
   update's `.rodata`. **Evidence needed:** the RMC method ID in the first login.
3. **`SecureConnection::Register` (`0x0B.1`) or `RegisterEx` (`0x0B.4`)?** This decides
   whether we need a change from `servers/nex`. `nex-protocols-common-go` refuses plain
   `Register` unless `EnableInsecureRegister()` is called, and `servers/nex` builds the
   SecureConnection common protocol privately, so a title cannot reach it. If ARMS — a Pia
   5.7.0 title, older than the 5.19 that changed this area — calls `Register`, it gets
   `Authentication::ValidationFailed` and never registers a station. **Evidence needed:** the
   method ID of the first `0x0B` call. **If it is 1, `servers/nex` must expose that toggle.**
4. **Does ARMS's Pia send the type-8 keepalive?** **Evidence needed:** any PRUDP packet of
   type 8 on the secure stream.
5. **Which DataStore methods does ARMS actually call, and with what data types and tags?**
   The implementation is general; what Party Crash and FestaRanking need of it is not known.
   **Evidence needed:** the `0x73` method IDs from a session that reaches the Party Crash
   screen, and the `DataStoreSearchParam` of any search.

Until 1 and 2 are answered on hardware, "it starts against a real Postgres, serves `/healthz`,
registers ARMS's protocols, and its DataStore round-trips objects for real" is all this
repository claims — and that much has been run, not assumed.
