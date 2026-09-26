# Provenance

Every non-obvious fact this server is built on, and where it came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): facts about a console and a
protocol are not authorship, but which fact came from where has to stay checkable.

Nothing here was copied. Each row is a value or a behaviour that was read, written down, and
then implemented in OpenPak's own shape. The DataStore backing is our own AGPL code, carried
across from `servers/arms` and `servers/mario-tennis-aces` (OpenPak repositories, an allowed
input), not from earlier.

## Sources

| Tag | What |
| --- | --- |
| `earlier/smo` | `~/REPOS/super-mario-odyssey`, the earlier network Odyssey server (source-available). Read for facts only. |
| `kinnay` | [Kinnay's NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki), incl. the [Game Server List](https://github.com/kinnay/NintendoClients/wiki/Game-Server-List) and the DataStore protocol page |
| `pretendo-src` | Pretendo's AGPL-3.0 `nex-go/v2` v2.3.1, `nex-protocols-go/v2` v2.3.7, `nex-protocols-common-go/v2` v2.6.1, read as source we are licensed to use |
| `openpak` | Our own `servers/nex`, `servers/arms`, `servers/mario-tennis-aces`, `nx-baas`, `ports.md` |

## Title identity

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `afef0ecf` | `kinnay` Game Server List. **Reference-verified, not binary-verified** — unlike ARMS's key, this was not independently pulled from Odyssey's own `.rodata`. `earlier/smo` `main.go` cites the same value from the same list and records the same caveat. |
| Game Server ID | `255BA201` | `kinnay` Game Server List. **Not** the access key — see the trap below. |
| NEX library version | `4.0.0` (`40000`) | **Unconfirmed.** The Game Server List records no per-title NEX version. `earlier/smo` `main.go` defaults to `40000` as the best-precedented guess for Odyssey's Oct-2017 (Splatoon 2 / ACNH / MK8D) era and marks it "the first thing to try changing if LoginEx never completes." We carry the same value and the same caveat. |
| Base title ID | `0100000000010000` | `earlier/smo` `main.go` |

`NEX_ACCESS_KEY` and `NEX_VERSION` are configuration (`example.env`), never literals in source.

**The Game Server ID is not the access key.** Odyssey's game server ID is `255BA201` and its
access key is `afef0ecf`; neither is derivable from the other. The ID is what appears in the
`g<hex>-lp1.s.n.srv.nintendo.net` hostname a console resolves, which makes it the thing most
likely to be mistaken for a key. An access key is always 8 hex digits and comes from the public
Game Server List or the game binary. A wrong access key is the worst failure mode in the whole
server: the PRUDP-Lite packet signature simply does not match, the console retries forever, and
neither side logs an error. If Odyssey reaches us and nothing at all happens, check this first.

## What Odyssey needs, and the shape of the server

Odyssey is the odd one out of the NEX fleet. It has **no matchmaking, no lobby, no NAT
traversal, no peer-to-peer play at all** (`earlier/smo` `README.md`, `main.go` — it registers
none of those protocols). Its entire online surface is **Balloon World**: a player hides a
balloon (a capture-pose screenshot plus costume snapshot at an in-game spot) and other players
search for and pop it, with "clap" ratings. That is DataStore (`0x73`) end to end.

So this binary registers a smaller protocol set than any sibling, and the one protocol the
whole title lives on has to be a real implementation.

| Protocol | ID | Status here | Source for "Odyssey uses it" |
| --- | --- | --- | --- |
| TicketGranting | `0x0A` | `servers/nex` | login is universal |
| SecureConnection | `0x0B` | `servers/nex` | login is universal |
| Utility | `0x6E` | registered | `earlier/smo` `main.go` registers it; NEX unique IDs back the DataStore post flow |
| DataStore | `0x73` | registered, **real** | `earlier/smo` `datastore.go`; it *is* the game |
| Ranking | `0x70` | **not** registered — see below | `earlier/smo` registers it speculatively |
| MatchMaking / MatchmakeExtension / MatchMakingExt / NATTraversal | `0x15/0x6D/0x32/0x03` | **not** registered | `earlier/smo` registers none of them; Odyssey has no lobby |

**Ranking (`0x70`) is not registered.** `earlier/smo` registers it but its own comment says
that costs "three lines and is harmless even if Odyssey's client never links a
RankingProtocolClient" — i.e. it is speculative, exactly as ARMS's was, and `servers/arms`
records that no Ranking client is actually linked. We leave it off rather than register a
protocol on a guess. If a capture shows a `0x70` call, `common_ranking.NewCommonProtocol` is
three lines. This is the single most likely addition, because Balloon World does surface
weekly balloon-finder standings — but whether those ride Ranking (`0x70`) or the Balloon
extension's own `SearchBalloon` result buckets (below) is exactly what a capture must settle.

## DataStore — real base protocol, and the Balloon extension gap

### The base protocol is real (what we made real)

`earlier-nex` has a DataStore *stub* across the rest of the fleet, but for Odyssey
specifically `earlier/smo` `datastore.go` did implement the base methods — as an **in-memory
map flushed to a JSON file every five seconds**, with a `SearchObject` that decoded only
`dataType` and a local HTTPS file host standing in for S3 (`earlier/smo` `README.md`,
`objectstore.go`).

OpenPak backs the base DataStore with `pretendo-src`'s `nex-protocols-common-go/v2/datastore`
handlers over **durable Postgres metadata + presigned S3 for bodies + the real friend graph**.
The following base methods are served for real (metadata always; the three body-moving ones
when `SMO_S3_ENDPOINT` is set):

| Call | Method | What now happens |
| --- | --- | --- |
| `GetMeta` / `GetMetas` | 8 / 9 | read an object by data ID, or by owner + persistence slot |
| `PreparePostObject` | 24 | create the metadata row, return a presigned S3 POST |
| `PrepareGetObject` | 25 | return a presigned S3 GET for an existing object |
| `CompletePostObject` | 26 | check the uploaded body's size, then make the object visible |
| `DeleteObject` | 4 | soft-delete |
| `SearchObject` | 12 | a real query honouring the whole `DataStoreSearchParam`, not just `dataType` — see `search.go` |
| `RateObject` / (`RateObjects`) | 15 / 40 | apply a clap to a rating slot, return the new totals |
| `PostMetaBinary` | 21 | a metadata-only object |
| `ChangeMeta` | (27) | name, period, data type, meta binary |

Method IDs above are from `earlier/smo` `datastore.go`; they are the standard DataStore
numbering `pretendo-src` also uses, so the common protocol dispatches them without a map.

Object bodies never pass through this process: `PreparePostObject`/`PrepareGetObject` hand the
console a presigned URL and it talks to the object store directly — which is what retail did
and what the library is built for. **S3 is optional**: with `SMO_S3_ENDPOINT` unset every
metadata call still works and the three body-movers answer `Core::NotImplemented`, which is
what the library does for an unset callback anyway. Balloon World needs S3 in production
(a balloon *is* a screenshot), but the server runs on a laptop with only Postgres.

### The Balloon extension is NOT served (the convergence gap)

`earlier/smo` `datastore.go` also implements a **title-specific extension on `0x73`,
"Data-Store-Protocol-(SMO)", methods 47-53**:

| Method | Name | Source |
| --- | --- | --- |
| 47 / 48 | `AddToBufferQueue` / `AddToBufferQueues` | `earlier/smo` `datastore.go` |
| 49 / 50 | `GetBufferQueue` / `GetBufferQueues` | the capture-pose screenshot buffers |
| 51 | `ClearBufferQueues` | |
| 52 | `SearchBalloon` | the search the Balloon World UI actually uses (vs. generic `SearchObject`) |
| 53 | `FetchMyInfos` | |

**None of these exist in `pretendo-src`.** Pretendo ships datastore extensions for Miitopia,
Super Mario Maker, Super Smash Bros. 4, Pokémon Bank / Gen 6 — but **no super-mario-odyssey**
(checked the `datastore/` subpackage list). So the common protocol answers all of 47-53 with
`Core::NotImplemented`, and this server does **not** serve them.

That matters more here than the "unimplemented method" line usually does, because of one
recorded observation:

> **`FetchMyInfos` (53) is the FIRST DataStore call Odyssey makes**, ahead of
> `PreparePostObject`. `earlier/smo` `datastore.go` marks this **confirmed live**: a real
> `FetchMyInfos` call's 45-byte body only parses cleanly (17 `balloonDataTypes` entries) once
> the Switch NEX-4 structure header is accounted for, and skipping that header was "the actual
> cause of the client's `2306-0111` after this call."

So a base-DataStore-only server will start, log in, and then meet a `FetchMyInfos` it answers
`NotImplemented` to — Balloon World does not get off the ground without the extension. This is
recorded as **unexplained/unfinished on our side, not a contradiction**: the honest state is
that OpenPak serves the base DataStore for real and the Balloon extension is a capture-gated
title module that does not exist yet in any common library.

**Everything else about the extension's wire shape is `earlier/smo`'s own guess** — its
`README.md` says none of `SearchBalloon`'s result bucketing or `AddToBufferQueue`'s slot field
width is cross-checked against a real capture. Reproducing those guesses blind would be
building on an unconfirmed layout *and* mirroring their handler shape, so it is deliberately
not done. See `design.md` for what building it properly requires.

## Transport and handshake — every `servers/nex` default is right for Odyssey

`earlier/smo` serves both endpoints with `ListenSecure(port, cert, key)` — TLS WebSocket —
and opens no UDP socket, and the server is recorded as its working transport. So:

| Knob | Value for Odyssey | Source / why |
| --- | --- | --- |
| `NEX_TRANSPORT` | `websocket` (default) | `earlier/smo` `main.go` listens WSS on both endpoints, no UDP. Switch PRUDP-over-WebSocket. |
| `NEX_SECURE_STREAM_ID` | `1` (default) — two-listener | `earlier/smo` runs auth (`:8453`) and secure (`:60013`) as **separate servers on separate ports**, and its secure station URL carries `sid=1`, `stream=10`. Two-listener shape, like ARMS. |
| `NEX_STATION_SCHEME` | `prudps` (default) | `earlier/smo` `stationScheme()` defaults to `prudps` (the Switch-era scheme). |
| `NEX_SOURCE_KEY_BYTES` | `32` (default) | `earlier/smo` `resolveUser` returns a `sha256.Sum256(...)[:]` source key — 32 bytes. |
| `NEX_SECURE_PID` | `2` (default) | `earlier/smo` `securePID = 2`; session key length 32, also the default. |
| `NEX_VERSION` | `4.0.0` | unconfirmed guess — see Title identity. This is the one value most likely to be wrong. |

The secure station URL `servers/nex` builds (CID 1, PID 2, sid 1, stream type RVSecure = 10,
type 2, scheme prudps) matches the one `earlier/smo` `main.go` constructs field for field.

`earlier/smo` also carries `SMO_SECURE_MINOR=0` (PRUDP minor version). That is moot on nex-go:
`pretendo-src`'s `prudp_endpoint.go` echoes the client's own SYN minor version, so there is
nothing to set — the same finding `servers/arms` records for `ARMS_SECURE_MINOR`.

## Identity

The earlier login path (`earlier/smo` `main.go`) resolves a signed `nx2.` HMAC token, or a
bare test PID, or an NSA id looked up against `openpak-account`, and falls back to an FNV hash
of the username for anonymous players. Its own bare-PID branch is a known authentication hole.

None of that is reproduced. OpenPak resolves identity in `servers/nex` through nx-baas's
internal identity API, which proves a BAAS id token, and this server never sees a username. The
friend graph comes back on the same call and is what `host.FriendPIDs` is wired to — and for
Odyssey the friend graph is load-bearing, because Balloon World shows friends' balloons.

One thing to check if Odyssey DataStore searches ever come back wrongly empty: a Switch logs in
under its **NSA id**, not its PID, and some titles then name a player by that login name rather
than by PID. None of the DataStore callbacks this server sets receives a name — every one is
given a PID — so there is nothing to translate today. `Host.PIDForLoginName` is the hook.

## Ports

`22340` (auth `22340`, secure `22341`, `/healthz` `22342`), the block reserved for this title
in `openpak` `ports.md`. the earlier server's `:8453` auth / `:60013` secure / `:8459` object store are
theirs and do not carry over — but the fact that both NEX endpoints are TLS WebSocket does, and
is why `NEX_TRANSPORT=websocket`.

## Error codes

| Code | Meaning | Source |
| --- | --- | --- |
| `0x80690004` | `DataStore::NotFound`, facility 105 | `earlier/smo` `datastore.go`; the same value is `nex.ResultCodes.DataStore.NotFound` in `pretendo-src`, which is what this server returns |
| `2306-0111` | what Odyssey shows after `FetchMyInfos` if the NEX-4 structure header is mishandled | `earlier/smo` `datastore.go`, marked confirmed live |

## Facts we did not need

Recorded so nobody looks for them twice: `earlier/smo` also carries a monitoring dashboard, a
presence reporter, and a local HTTPS blob host (`objectstore.go`) standing in for S3. The first
two are operational shape, not protocol facts; the third OpenPak replaces with a real S3 bucket
reached through the library's presigner. A per-title kerberos password shared between the two
endpoints does not apply either: `servers/nex` generates both special accounts' passwords per
process, because nothing outside that process derives a key from them.

## What has actually been run

Built (`go build ./...`), vetted (`go vet ./...`) and tested (`go test ./...`) green, then run
against a throwaway Postgres with the block's env: the binary starts in the two-listener shape,
`/healthz` on `22342` answers `ok super-mario-odyssey websocket auth=22340 secure=22341`, and
startup creates the `odyssey` DataStore schema (`datastore_objects`, `datastore_ratings`) and
the library's `utility` schema. No console has touched it — the NEX version, the access key,
and the entire Balloon extension are the open questions a live capture must answer.
