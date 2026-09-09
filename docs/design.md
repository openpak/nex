# Design

What Mario Golf: Super Rush needs from a server, what this one does, and what it does not.

## What the title needs

Golf is peer-to-peer. Up to four players share a round, Golf Adventure's Battle Golf gathers
a larger lobby, and once the players are introduced to each other the shots themselves are
Pia traffic between consoles. The server's whole job is the part before that:

    ticket granting -> secure connection -> matchmaking -> NAT traversal -> P2P

Everything in that line is a protocol several dozen titles share, and Pretendo's AGPL-3.0
`nex-protocols-common-go/v2` implements all of it. So this repository is wiring, not
protocol work: `cmd/golf/main.go` is the entire server.

## What is implemented

`servers/nex` already registers ticket granting on the authentication endpoint and secure
connection on the secure one. On top of that, Golf registers:

| Protocol | Backing | Notes |
| --- | --- | --- |
| Match Making (`0x15`) | Postgres | gatherings, participants, host migration, session URLs |
| Matchmake Extension (`0x6D`) | Postgres | sessions, auto-matchmake, communities, notification data |
| Match Making Ext (`0x32`) | Postgres | `EndParticipation` |
| NAT Traversal (`0x03`) | none needed | probe relaying between two consoles |
| Utility (`0x6E`) | Postgres | NEX unique IDs, and their association with a PID |

The library creates its own schemas and tables on startup (`CREATE SCHEMA IF NOT EXISTS`
inside each `SetManager`), so there is no migration step and no schema in this repository.
Match Making is registered first because it is the one that creates the `matchmaking` schema
that Matchmake Extension then adds tables to.

Two things this gains over the earlier server for the same title:

- **Lobbies survive a restart.** Its matchmaking is in-process and dies with the process,
  taking every open round with it. Ours is in Postgres.
- **Friend-only matchmaking needs no friend table.** `MatchmakingManager.GetUserFriendPIDs`
  is `Host.FriendPIDs` — the shared host's nx-baas identity call, the same door the NPLN
  titles use — so a player's friends are the same people to every OpenPak service. Upstream
  notes that check as not working on Switch; it works here.

## What is not implemented, and why

**DataStore (`0x73`) — not registered.** OpenPak can serve DataStore properly, which
Reference cannot: `earlier-nex` has no implementation, so every DataStore call in that fleet
is a per-title stub. That capability is real and it matters — for a title that calls
DataStore. Golf is not known to be one. The earlier server for Golf never registers
DataStore at all and says outright that whether Golf needs it is unknown until a real
client's calls show it. So there is no stub here to replace, and nothing to implement
against: no observed data type, no observed field, no observed permission. Building an
S3-backed object store on that basis would be guessing at a wire format nobody has seen.

The moment a capture shows Golf calling `0x73`, this is roughly a day: register
`datastore.NewProtocol()`, hand `common_datastore.NewCommonProtocol` a MinIO client and the
object-metadata callbacks, and `get_meta`, `get_metas`, `prepare_get_object`,
`prepare_post_object`, `complete_post_object`, `search_object`, `rate_object`,
`change_meta` and `delete_object` all come from the library. That backend should live in
`servers/nex` and be shared, not be written once per title — the titles that are known to
need it (`arms`, `mario-party-superstars`, `super-mario-maker-2`) will each want the same
one.

**Ranking (`0x70`) — not registered.** Two independent reasons, either one sufficient.
Nobody has observed Golf calling it: the earlier server registers a generic handler that is
in fact Mario Kart 8 Deluxe-shaped — an in-memory profile and tournament store — and answers
`Core::NotImplemented` to everything else, so registering it there was a starting point, not
a measurement. And `nex-protocols-go` v2.3.5, the version `servers/nex` pins, cannot wire
the protocol at all: `ranking.Protocol.SetHandlerGetStats` takes `constants.StatsFlag` while
`ranking.Interface` declares `types.UInt32`, so `common_ranking.NewCommonProtocol` does not
compile. v2.3.7 fixes that; the bump belongs to `servers/nex`.

**Everything downstream of a login.** Nothing here has been run against a console, an
emulator or the real title. The server starts, answers `/healthz`, and completes a login
whose id token nx-baas proves — that is the whole of what can be claimed.

## Transport: a secure WebSocket, not UDP

A Switch NEX title does not speak PRUDP over UDP. It opens a `wss://` connection and speaks
PRUDPLite over it, and the console reaches ours by the DNS name
`g211a3f00-lp1.s.n.srv.nintendo.net` on 443. So Golf sets `NEX_TRANSPORT=websocket`, which
is the only default it changes from the shared host: `servers/nex` defaults to `udp` for the
Wii U and 3DS era titles.

The listeners themselves are plain `ws`. TLS is terminated by the front proxy, which is
where every other console-facing listener in OpenPak terminates it, and is `servers/nex`'s
decision rather than this repository's. It does mean the deployment has one more obligation
than a UDP title: the console reaches the *secure* server at whatever address and port the
station URL names, so 22131 needs a TLS front too, not only the 22130 the SNI route points
at. Nothing on hardware has exercised that yet.

One thing this transport gives us for free. The earlier stack had to force the secure
endpoint's PRUDP minor version to 0, because its library defaulted to 5 and a console that
is answered 5 sends CONNECT with an empty payload and never hands over its kerberos ticket.
`nex-go` sets a minor version on the ack only in its PRUDPv1 branch, so a Lite ack already
carries 0. No knob, no configuration, nothing to get wrong.

## Ports

Block 22130 from [`ports.md`](../../../ports.md): authentication 22130, secure 22131,
`/healthz` 22132. Unlike the UDP titles, both game ports are TCP and carry TLS, so the
firewall rules are `ufw allow 22130/tcp` and `ufw allow 22131/tcp`, and Traefik's SNI route
for the DNS name above points at 22130. The console reaches the secure server at the address
in the station URL — `NEX_SECURE_HOST` and 22131 — so that port is reachable from a console
directly and needs a certificate the console will accept.

## What the next round needs

Everything below wants one capture from a real console or an emulator running the retail
title. Nothing here is guessed at in the code; where a fact is missing, startup fails or the
protocol is left unregistered.

1. **`NEX_VERSION`.** 4.6.5 is an era guess inherited from the earlier server, which is
   explicit that it never measured it. Evidence needed: `apiVersionGeneral` from the first
   login packet.
2. **Which login method.** Method 2 or method 6, which follows from the version.
   `servers/nex` implements both, so this only decides which code path runs. Evidence
   needed: the RMC method ID of the first ticket-granting call.
3. **Does the token in `AuthenticationInfo` reach nx-baas?** The whole identity chain
   assumes it is an nx-baas id token whose `openpak` claim we can prove. Evidence needed: the
   token field's shape and byte length (`nex-protocols-go` notes an observed 918-byte
   ceiling).
4. **`prudps` or `prudp`.** Golf's is the Reference default, never measured for this title.
   The failure is not clean: with the wrong one the menus work and the title dies later.
   Evidence needed: whether CONNECT to the secure endpoint carries a payload.
5. **Does Golf call Ranking or DataStore at all?** The RMC log answers this on the first
   real session. Evidence needed: a protocol/method histogram from one round of online play.
6. **That the console accepts the front proxy's certificate** for
   `g211a3f00-lp1.s.n.srv.nintendo.net`, on the secure port as well as the authentication
   one. An operational fact about our TLS setup, not about the title.

## Sources

- [`docs/provenance.md`](provenance.md) — every fact and where it came from.
- [`servers/nex/docs/design.md`](../../nex/docs/design.md) — the login, kerberos and
  identity story this server sits on.
- [Kinnay, NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki) —
  authentication protocol, kerberos, common types.
- Pretendo's `nex-go/v2`, `nex-protocols-go/v2`, `nex-protocols-common-go/v2`, read as
  AGPL-3.0 source.
