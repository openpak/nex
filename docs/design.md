# Design

What Mario Strikers needs from a server, what is here, and what is not.

## What this repository is

One binary that registers Strikers' confirmed protocol set on the shared NEX host and gets out
of the way. `servers/nex` owns PRUDP, kerberos, the Switch ticket-granting methods,
SecureConnection, the nx-baas account lookup, the port block and `/healthz`. Pretendo's
`nex-protocols-common-go/v2` owns the handlers for every protocol registered below. What is left
— and all this repository actually is — is three things:

1. Which protocols to register, and in what order.
2. The friend lookup, so friend-list matchmaking works.
3. A DataStore backing, because that is the one thing the common library deliberately leaves to
   the operator.

`cmd/strikersd/main.go` is the whole of 1 and 2. `cmd/strikersd/datastore.go` and `search.go`
are 3.

## What Strikers needs

Strikers' online is server-arranged play: quick battle, matchmaking, and "clubs" — persistent
groups the game builds on top of NEX gatherings. A console logs in, registers its station URLs,
then creates or searches for a session; NAT traversal gets two consoles talking directly and the
server steps back until someone leaves. On top of that Strikers has a club layer with its own
MatchmakeExtension methods (create/join/list club, member roster).

That maps onto:

| Protocol | ID | Why Strikers needs it | Here |
| --- | --- | --- | --- |
| MatchMaking | `0x15` | the gathering lifecycle | registered (common) |
| MatchMakingExt | `0x32` | leaving a gathering | registered (common) |
| MatchmakeExtension | `0x6D` | the session itself; and the club methods on top | common set registered; **club methods deferred** |
| NATTraversal | `0x03` | probe requests / NAT reports so two consoles reach each other | registered (common) |
| Utility | `0x6E` | NEX unique IDs | registered (common) |
| DataStore | `0x73` | init calls only — Strikers stores nothing here | registered (common, empty store) |

`provenance.md` carries the full protocol table, the deferred set (Ranking `0x70`,
MatchmakeReferee `0x78`, Subscriber `0x79`, Ranking2 `0x7A`, and the `0x6D` club methods), and
why each is deferred rather than stubbed.

## Transport and handshake

Nothing to set: every `servers/nex` default is right for Strikers, and `provenance.md` shows the
secure station URL matches the working earlier one field for field. Transport is `websocket`
(the default now). The secure stream ID stays `1`, the two-listener shape — authentication on
`22300`, secure on its own dialable `22301` — which `earlier/strikers`'s `sid=1` confirms. If
the stream ID is ever wrong the host logs `trying to connect to unbound PRUDPEndPoint`, so it is
checkable rather than silent.

The access key `MAGNI5jG` is an 8-character ASCII string, not hex. `servers/nex` passes it
straight to nex-go, which MD5s it, so no host change is needed — but a `servers/nex` that ever
hex-decoded the key would break this title silently. Recorded in `provenance.md`.

## State

Matchmaking, NEX unique IDs and DataStore metadata are all in Postgres, because
`nex-protocols-common-go` keeps them there and writing an in-memory matchmaker to avoid one
connection string would be more code, not less. The libraries create their own `matchmaking`,
`tracking` and `utility` schemas on first run; `datastore.go` creates `strikers`.

`STRIKERS_DATABASE_URL` is required and startup pings it. A NEX title server without matchmaking
is not a degraded Strikers server, it is a lobby the game cannot enter, so failing loudly beats
starting.

## DataStore — registered, but Strikers stores nothing in it

This is the one deliberate judgement call in the port, so it is written down in full.

`earlier-nex` has no DataStore implementation, so the earlier network Strikers server answers
every `0x73` call with an empty list and `0x73.8` with `DataStore::NotFound`. It is a probe stub.
Strikers' real online data — clubs and rosters — travels over **MatchmakeExtension** (`0x6D`
game-specific methods) and an in-memory club store, **not** DataStore. So Strikers genuinely does
not use DataStore for storage.

We register the common DataStore anyway, over an **empty** Postgres store (the exact ARMS code,
schema renamed to `strikers`):

- The game issues `0x73` init calls. Leaving `0x73` unregistered answers `Core::NotImplemented`,
  and the earlier server registered a stub specifically to avoid that — evidence NotImplemented
  on init is bad in this engine.
- The common DataStore over an empty store returns exactly the empty list / `NotFound` the stub
  returned, but through real, reviewed handlers.
- It is our own already-reviewed code, and the agreed lift-candidate for the shared
  `github.com/openpak/nex/datastore` subpackage (not built yet). Registering it directly keeps
  Strikers and ARMS byte-identical here, which is what makes the eventual lift a rename.

The alternative — a bare "register none" — was rejected because of the observed NotImplemented
risk. The other alternative — reproducing the earlier server's per-method zero-struct/empty-list probe — was
rejected because that classification is unconfirmed and answering a struct method with an empty
list overflows the client (`Core::BufferOverflow`). The empty common store is the choice that is
both correct and lowest-effort-to-lift.

If a capture confirms Strikers never touches a DataStore object, this can shrink to a stub. Until
then it stays.

### The DataStore backend (when the game does use it)

The metadata handlers are the ARMS implementation verbatim (renamed schema and S3 key bases):
`GetMeta`, `GetMetas`, `PreparePostObject`, `PrepareGetObject`, `CompletePostObject`,
`DeleteObject`, `ChangeMeta`, `RateObject`, `SearchObject`, `PostMetaBinary`, and the batch
forms — all backed by Postgres. Object bodies never pass through this process: `PreparePostObject`
and `PrepareGetObject` hand the console a presigned S3 URL and it talks to the object store
directly.

**S3 is optional.** With `STRIKERS_S3_ENDPOINT` unset, every metadata call still works and the
three that move a body answer `Core::NotImplemented` — which is what the library does for an unset
callback anyway. A half-configured store (endpoint set, bucket or credentials missing) is a
startup error. `search.go`'s `searchQuery` is fully parameterised; the only text a console
influences is the ORDER BY column, which comes from a fixed map or falls back to `data_id`.
`search_test.go` checks exactly that.

## Identity and friends

`servers/nex` resolves a console to an OpenPak account through nx-baas and hands back the player's
friend graph. `host.FriendPIDs` is assigned to both the matchmaking manager's `GetUserFriendPIDs`
and DataStore's — the two upstream callbacks documented as not working on Switch upstream (they
ask by 32-bit PID and Pretendo has no Switch friend graph; OpenPak does). Use the host's function,
not a local one: it fails closed on a lookup error and drops a friend whose PID does not fit 32
bits rather than truncating it (truncation would name a different account and hand that stranger a
friend's access). `needsFriends` keeps the nx-baas call off every search that does not need the
graph.

## What is not here, and why

- **Ranking (`0x70`).** Strikers registers it, but `common_ranking` needs an operator-supplied
  leaderboard backend and Strikers' category/score shapes are unobserved. A real backend when a
  capture shows them.
- **The club methods, MatchmakeReferee (`0x78`), Subscriber (`0x79`), Ranking2 (`0x7A`).** No
  common implementation; the Reference stubs are unconfirmed reverse-engineering. Deferred to a
  capture round — see `provenance.md`.
- **A Pia type-8 keepalive handler.** Moot on nex-go; `RegisterCustomPacketHandler` is the hook
  if a capture shows a type-8 packet arriving.
- **A monitoring dashboard, a presence reporter, a proxy-protocol mode.** The earlier server has
  all three. OpenPak answers "is it up" with `/healthz`, "who is online" belongs to the account
  side, and TLS/PROXY termination is the front proxy's.

## Open questions — what the next round needs

In the order a first live run would answer them:

1. **Does Strikers reach us at all?** A wrong access key fails silently. **Evidence needed:** any
   packet arriving at `22300`.
2. **Method 2 or method 6, and which `apiVersionGeneral`?** `NEX_VERSION` is `4.0.0`. **Evidence
   needed:** the RMC method ID in the first login.
3. **`SecureConnection::Register` (`0x0B.1`) or `RegisterEx` (`0x0B.4`)?** If Strikers calls plain
   `Register`, `servers/nex` must expose `EnableInsecureRegister()`. **Evidence needed:** the
   method ID of the first `0x0B` call.
4. **Which club / init methods does Strikers actually call, in what order, with what bytes?** This
   is what turns the deferred `0x6D` club layer and the `0x78`/`0x79`/`0x7A` protocols from method
   IDs into implementations. **Evidence needed:** the RMC log of a session that reaches the club
   screens.

Until 1 and 2 are answered on hardware, "it starts against a real Postgres, serves `/healthz`,
registers Strikers' confirmed protocols, and its DataStore round-trips objects for real" is all
this repository claims — and the config path for that much has been run (`host.New` accepts the
Strikers config; build, vet and the search tests are green), not assumed.
