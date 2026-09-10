# Design

What Minecraft needs from a server, what is here, and what is not.

## What this repository is

One binary that registers Minecraft's protocol set on the shared NEX host and gets out of the
way. `servers/nex` owns PRUDP, kerberos, the Switch ticket-granting methods, SecureConnection,
the nx-baas account lookup, the port block and `/healthz`. Pretendo's
`nex-protocols-common-go/v2` owns the handlers for every protocol below. What is left — and all
this repository is — is two things:

1. Which protocols to register, and in what order.
2. The friend lookup, so friend-list lobbies work.

`cmd/minecraftd/main.go` is the whole of both. There is no third file: Minecraft registers no
DataStore, so unlike ARMS there is no object-store backing to write.

## What Minecraft needs

Minecraft's Switch online is session join. A console logs in, then either hosts a world or
searches for one to join — public, or a friend's, which is the same search narrowed to
sessions owned by people the player knows. Once matched, NAT traversal gets the two consoles
talking directly and the server stops mattering until someone leaves.

That maps onto:

| Protocol | ID | Why Minecraft needs it |
| --- | --- | --- |
| MatchMaking | `0x15` | the gathering lifecycle: unregister, find by ID, session URLs, host migration |
| MatchMakingExt | `0x32` | leaving a gathering |
| MatchmakeExtension | `0x6D` | the lobby itself: create, join, auto-matchmake, browse |
| NATTraversal | `0x03` | probe requests and NAT property reports so two consoles can reach each other |
| Utility | `0x6E` | NEX unique IDs |

This is ARMS's protocol set minus DataStore. The registration wiring is identical because the
shared host and common library are identical; what differs is the per-title configuration
(`example.env`) and the two protocols ARMS has that Minecraft does not.

## What is not here, and why

- **DataStore (`0x73`).** Minecraft's working earlier network server never registers it
  (`provenance.md`). ARMS's update does, which is why ARMS's port has a real DataStore backing
  and this one has none. Nothing to build.
- **Ranking (`0x70`).** Minecraft's server *does* register a Ranking handler, but that handler
  is measured against Mario Kart 8 Deluxe, not Minecraft, and there is no Minecraft-specific
  ranking fact anywhere. `provenance.md` lays out the full reasoning; the short version is:
  registering the common Ranking protocol without a backing returns `Core::NotImplemented`,
  which is byte-for-byte what an unregistered protocol returns, so registering it bare buys
  nothing; and inventing an MK8D-shaped backing is fork, not facts. Left off, exactly as ARMS
  is. A capture showing a Minecraft `0x70` call is what would change this — see open questions.
- **The identity, gates, dashboard, reaper, and presence reporter** in Minecraft's server.
  Those are the earlier server's account integration and operational shape; OpenPak's identity is
  nx-baas via `servers/nex`, liveness is `/healthz`, and online policy is the account side.

## Transport and the station scheme

**Transport is `websocket`** (the `servers/nex` default): Switch NEX titles speak PRUDP over
WebSocket, and Minecraft's working server listens WSS on both endpoints and opens no UDP
socket. TLS termination is the front proxy's unless `NEX_TLS_CERT`/`NEX_TLS_KEY` are set.

**The secure stream ID stays `1`** (the default), the two-listener shape: auth on `22310`,
secure on its own dialable `22311`. Minecraft's server runs two listeners and hands out a
station URL with `sid=1`. Do not set `NEX_SECURE_STREAM_ID=2` — that single-listener shape is
SMB35's, and the symptom of getting it wrong is the host logging `trying to connect to unbound
PRUDPEndPoint`.

**The station scheme is `prudp`, and this is the one non-obvious setting.** Most fleet titles
(ARMS included) advertise the secure station URL as `prudps`; Minecraft's working server
advertises `prudp`. `provenance.md` records this as a cross-title contradiction — ARMS's own
evidence says `prudp` breaks Pia, Minecraft's own working server uses it — and the value here
matches what is attested to work *for Minecraft*. Because the wrong scheme fails silently and
late (menus work, the title gives up when Pia needs the session), this is the first thing to
confirm on hardware. `NEX_STATION_SCHEME` changes it in one line.

## State

Matchmaking and NEX unique IDs live in Postgres, because `nex-protocols-common-go` keeps them
there and writing an in-memory matchmaker to avoid one connection string would be more code,
not less. The libraries create their own `matchmaking`, `tracking` and `utility` schemas on
first run. `MINECRAFT_DATABASE_URL` is required and startup pings it: a NEX title server
without matchmaking is not a degraded Minecraft server, it is a lobby the game cannot enter, so
failing loudly beats starting.

## Identity and friends

`servers/nex` resolves a console to an OpenPak account through nx-baas and hands back the
friend graph. `host.FriendPIDs` is assigned to the matchmaking manager's `GetUserFriendPIDs`,
which is the whole of friends-list lobby joining. It fails closed on a lookup error (a
friends-only lobby stays shut rather than opening to everyone) and drops a friend whose PID
does not fit 32 bits rather than truncating it into a different account. Use the host function,
not a local one.

## What a first hardware run must answer

`servers/nex`'s design doc lists the questions common to every NEX title. These are
Minecraft's own, in the order a first live run would answer them:

1. **Does Minecraft reach us at all?** A wrong access key fails silently — the PRUDP-Lite
   signature does not match and the console retries forever with nothing logged either side.
   The key here was cryptographically verified against a measured signature (`provenance.md`),
   so this is expected to pass, but it is still the first thing to confirm. **Evidence needed:**
   any packet arriving at the auth listener.
2. **Is the station scheme right?** `prudp` is Minecraft's observed value and contradicts
   ARMS's cross-title finding. **Evidence needed:** whether the console, after login, actually
   dials the secure endpoint and completes a Pia session, or stalls when the session is needed.
   If it stalls, flip `NEX_STATION_SCHEME=prudps`.
3. **NEX version — 4.0.0 or 4.3.1?** The code ran 40000, a comment says 4.3.1. Behaviourally
   equal for us, but worth settling. **Evidence needed:** the login's `apiVersionGeneral`.
4. **Does Minecraft use Ranking?** Not registered, on the reasoning in `provenance.md`.
   **Evidence needed:** any `0x70` call, and the payload shape if one appears — that is what a
   real Ranking implementation would have to be built from.

Until 1 and 2 are answered on hardware, "it starts against a real Postgres, serves `/healthz`,
listens on its block, and registers Minecraft's protocols" is all this repository claims — and
that much has been run, not assumed (see the README).
