# Design

What Mario Party Superstars needs from a server, what this repository implements, what it
does not, and why.

## The shape of the thing

Almost nothing here is Mario Party Superstars code. Three layers do the work:

| Layer | What it does | Whose code |
| --- | --- | --- |
| `nex-go`, `nex-protocols-go`, `nex-protocols-common-go` | PRUDP, kerberos, RMC, and working handlers for every standard NEX protocol | Pretendo, AGPL-3.0 |
| [`servers/nex`](../../nex) | Switch login (`ValidateAndRequestTicketWith{Param,CustomData}`), OpenPak identity through nx-baas, the port block, `/healthz` | OpenPak |
| this repository | The two things neither of those covers for this title, and the per-title wire-shape values | OpenPak |

If a line here could have been a call into one of the layers above, it should be. The whole
package is under 400 lines across two title-specific files.

## What this title needs

The set of protocols comes from what the earlier network server for this title registers
(recorded, with sources, in [`provenance.md`](provenance.md)): Secure Connection, Match
Making, Matchmake Extension, Match Making Ext, NAT Traversal, Utility, DataStore, Ranking.

Everything except Utility's settings and two Matchmake Extension methods is already
implemented by `nex-protocols-common-go` and is registered as-is. DataStore is registered by
nothing, for the reason in section 3.

### 1. Friend join — the two missing matchmaking methods

`matchmaking.go`. `nex-protocols-common-go` implements most of Matchmake Extension but not
`FindMatchmakeSessionByGatheringIDDetail` (`0x29`) or `FindMatchmakeSessionByParticipant`
(`0x33`), and those two are how this game follows a friend into a lobby:

- The joiner asks **`FindMatchmakeSessionByParticipant`** which gathering the host is in.
  Answered empty — which is what an unimplemented method amounts to — the game gives up on
  the friend and runs a public auto-matchmake instead, ending up in a session of its own.
  This is the "my friend could not see my lobby" failure the earlier network server traced.
- The Join Room screen then polls **`FindMatchmakeSessionByGatheringIDDetail`** roughly once
  a second against that gathering ID, and expects `RendezVous::SessionVoid` once the room is
  gone.

Both are adapters, not a second matchmaking implementation: the queries they need
(`GetPlayingSession`, `GetMatchmakeSessionByID`) already exist in the common library's own
database package, and everything else — creating, joining, leaving, cleaning up after a
dropped connection — is the common protocol untouched.

`FindMatchmakeSessionByGatheringIDDetail` is answered to anyone who knows the gathering ID,
so it clears the session key and user password before replying. Joining is what hands those
over.

### 2. Utility integer settings

`utility.go`. The game calls `GetIntegerSettings` immediately after
`SecureConnection::Register`, and on the earlier network stack the connection died about a
second later, every time, because the reply was malformed — that server's generic handler
read the single index parameter as a list length. Pretendo's implementation parses it
correctly, so the parsing half of that bug does not exist here.

What is left is content, and nobody knows it. There is no capture of this title's settings
and no public reference server. The choice made here is to answer a correctly shaped **empty**
map rather than borrow another title's numbers, and to make the real values loadable from
`MPS_INTEGER_SETTINGS` — a wrong guess should cost a restart, not a release. Everything else
Utility does (NEX unique IDs) is the common protocol, backed by the same Postgres.

### 3. DataStore — registered nowhere, deliberately

The DataStore protocol (`0x73`) is not registered. That is a decision, not an omission, and
it reverses one made earlier in this repository.

The first version of this server carried a full Postgres-and-S3 DataStore implementation,
because the earlier network server stubs DataStore and OpenPak can do better. Two things
changed that:

1. **Nothing has ever observed this title call DataStore.** Their stub was added by analogy
   with Super Smash Bros. Ultimate, before any MPS DataStore call was seen — their connection
   never survived long enough to make one. An object store built to the protocol rather than
   to an observed wire format is speculation with a schema.
2. **The object store is a fleet component.** Three titles need one, and the decision is that
   it lives in `servers/nex` so they do not each carry a divergent copy. A title-specific
   store here would have been stranded.

The implementation is not lost — it is in this repository's history at commit `5490013`
(`datastore.go`, `datastore_test.go`: object metadata in one Postgres table, presigned S3
uploads and downloads, and every callback the common protocol asks for, with four tests
against a real database). It was offered to the shared backend as a starting point.

One thing worth carrying forward: the reason the earlier network server needs a stub does not
exist on this stack. `earlier-nex` **silently drops** calls to an unregistered protocol,
which is what soft-locked SSBU. `nex-go` answers `Core::NotImplemented`, which is exactly what
a stub says. Registering an empty DataStore here would change nothing.

## What is not implemented

- **Ranking (`0x70`).** the earlier network server registers a generic acknowledgement here
  and its own README says it is unknown whether this title needs real leaderboard data. A
  generic acknowledgement is not better than `Core::NotImplemented` — both tell the game
  "no data" — and a real implementation needs a category and score-format schema that no
  capture has provided. Left out until something shows the game calling it.
- **Messaging, notifications, subscriber.** the earlier network server registers none of
  these either, though its logs show the client *linking* a Subscriber protocol and possibly
  a NAT-traversal-internal one. That is an observation about the game binary, not about what
  it calls. Their fallback logger exists to find out; ours is `nex-go`'s own
  `Core::NotImplemented`, which is the same information in a different place.
- **DataStore.** See above.
- **A Pia keepalive handler.** `earlier-nex`'s PRUDP switch only knows packet types 0–4, so
  it drops Pia's periodic type-8 keepalive and the console gives up on the connection. That
  is a gap in their transport, not a property of the game, and `nex-go` is a different
  transport. Whether it has the same gap is an open question, not something to pre-emptively
  patch around.
- **A presence reporter.** Their server reports active PIDs to their account service every
  30 seconds so the console's friend list shows players as online. OpenPak's equivalent
  belongs in nx-baas, which already owns presence for NPLN titles, rather than in each of
  forty title servers. Worth raising as a fleet-wide question.
- **A dashboard.** `/healthz` plus the Gatus status page is OpenPak's answer to "is it up".

## Deploy

`Dockerfile` and `.github/workflows/release.yml` are the fleet shape: tag `v1.2.3`, the
workflow publishes `ghcr.io/openpak/mario-party-superstars:1.2.3` and `:latest`, and the
deploy host's `podman-auto-update` does the rest.

The earlier blocker is resolved. `servers/nex` now has a real module path
(`github.com/openpak/nex`, tagged), so this repository depends on it as an ordinary module
and a single-repo checkout builds. Local multi-repo work uses a `go.work` at
`~/REPOS/Openpak/servers`, which never enters `go.mod`.

One requirement is left, and it is the same for every NEX title: `github.com/openpak/nex` is
a **private** repository, so the public module proxy and checksum database cannot serve it.
Anything that fetches it needs `GOPRIVATE=github.com/openpak/*` and credentials —

- locally, whatever git credential helper already lets you clone the org;
- in the container build, a `netrc` build secret (never a layer);
- in CI, an org secret `OPENPAK_MODULE_TOKEN` with read access to `openpak/nex`, because
  `GITHUB_TOKEN` only covers this repository.

That secret does not exist yet. Until it is created the release workflow will fail at
`go mod download`, and the failure will say so.

## What the next round needs

In rough order of how much they unblock:

1. **A console that gets past `GetIntegerSettings`.** Everything below is downstream of a
   connection that survives. If it does not, the two candidates are the settings content and
   the Pia keepalive, in that order.
2. **The login capture `servers/nex` is asking for** — token shape, token length, `userName`,
   and whether this title uses method 2 or method 6. NEX 4.6.5 points at method 6, but that
   is inference from a version number.
3. **A two-console friend join.** Host creates a lobby, joiner opens Join Room. What the
   joiner calls, in what order, and with what gathering ID, tells us whether the two methods
   implemented here are the right two and whether the friend graph reaches them.
4. **Any DataStore call at all.** Nobody has ever seen this title make one, which is why
   nothing is registered for it. The first such call decides whether this title needs the
   shared object store at all, and what object types to record.
5. **Whether Ranking is ever called.**

## Sources

Every protocol fact, with where it came from, is in [`provenance.md`](provenance.md).
