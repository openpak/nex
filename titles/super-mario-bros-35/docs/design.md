# Design

What Super Mario Bros. 35 needs from a server, what this one does, what it does not, and
why. Every protocol fact cites its source in [`provenance.md`](provenance.md).

## A title with nothing on the other side

Super Mario Bros. 35 was a time-limited release. Nintendo delisted it on 30 March 2021 and
switched the service off on 31 March 2021. That is not the situation the rest of the fleet
is in, and it changes three things worth stating plainly:

1. **There is no live service to compete with.** Nobody can play this game online today, on
   any console, by any means, except against a server somebody runs. A server is not an
   alternative to the official one; it is the only one.
2. **There is no identity wall.** Titles that still have a vendor behind them force a
   question about what a token proves and who is entitled to validate it. Here there is no
   vendor left to ask. Identity in this server exists for OpenPak's own reasons — one
   account per player, a friend graph, a name in a lobby — not because anything outside
   demands it.
3. **The game will never be patched again.** Every wire format here is final. A structure
   that decodes today decodes forever, and a fact recovered once does not need rechecking
    against a future version. That is worth more than it sounds: it means "we matched the wire format exactly" is a durable claim rather than a snapshot.

The practical consequence is that the bar is *playability*, not parity with a live service.
Nothing here is a downgrade from something a player could otherwise have.

## The shape of the game

Super Mario Bros. 35 is a 35-player battle royale, and the interesting thing about its
matchmaking is how little of it is matchmaking.

NEX forms the lobby and does nothing else. The client proposes a `MatchmakeSession` with
`maxParticipants` 35 and a set of search criteria; the server finds a session matching the
criteria or creates the one proposed, joins the caller, and tells the participants already
there. The 35 is the client's number. **The server never counts to it, and no NEX call
starts a match.**

What starts a match is a second server the game connects to afterwards. Unlike ARMS,
Mario Kart 8 Deluxe, Splatoon 2 or Smash Ultimate, this game does not play peer to peer over
Pia. Gameplay runs through **Eagle**, a relay Nintendo used for exactly four titles: Tetris
99, Super Mario Bros. 35, PAC-MAN 99 and F-Zero 99. When a player joins a lobby, the NEX
server pushes them one notification carrying the relay's address and a signed token to
present to it. If that notification never arrives, the lobby fills and then nothing happens:
the game is waiting for an address, and shows no error.

So the server's whole job is:

```
login  ->  lobby  ->  "the relay is at <url>, here is your token"  ->  (relay)
                  ->  round bookkeeping, lobby chat, name-and-Mii exchange
```

## What is implemented

Everything on the NEX side.

| Protocol | ID | Where it comes from |
| --- | --- | --- |
| Ticket Granting (Switch login, method 6) | 0xA | `openpak/nex` |
| Secure Connection | 0xB | Pretendo common, via `openpak/nex` |
| Match Making | 0x15 | Pretendo common |
| Matchmake Extension | 0x6D | Pretendo common |
| Match Making Ext | 0x32 | Pretendo common |
| Message Delivery | 0x1B | Pretendo common |
| Utility | 0x6E | Pretendo common, plus this title's `GetIntegerSettings` table |
| Ranking2 | 0x7A | written here |
| Matchmake Referee | 0x78 | written here |

Ranking2 and Matchmake Referee are the two the game calls that
`nex-protocols-common-go` has no implementation of. `nex-protocols-go` still supplies their
wire types and method dispatch, so what this repository writes is only behaviour:

- **Ranking2** stores each player's common data (their name and Mii, which every client
  uploads on connect and reads back for the other 34 faces in the lobby). Rankings are
  empty, category settings are one fixed answer, and score-rank estimates are zeroes. That
   is not a stub — Super Mario Bros. 35 has no server-side leaderboard, so there is nothing to return.
- **Matchmake Referee** opens and closes rounds, and refuses a round whose named players are
  not actually in the gathering they are claimed to be in. Without that check, any client
  could push a round-start notification at any other player on the server.

Both stores are in memory. Common data is re-uploaded on every connect and a round is a
single match in progress; neither survives a restart usefully, so neither gets a table.

Matchmaking, messaging and the NEX unique-id table are Postgres-backed, because that is what
the Pretendo common protocols are: they take a `*sql.DB` and create their own schemas. This
server opens the database and pings it at startup rather than letting a bad URL surface as a
server that starts cleanly and then fails every call.

## What is not implemented

### The Eagle relay

Not here, and not anywhere in OpenPak yet. This repository owns the NEX half of the handoff
— the address, the per-player token, and the revision 1 notification that carries them — and
that half is complete and tested. The relay itself is a separate program.

It belongs in a repository of its own rather than this one, because it is shared by four
titles. Building it inside Super Mario Bros. 35 would mean building it again for Tetris 99,
PAC-MAN 99 and F-Zero 99, or extracting it later under pressure. `SMB35_EAGLE_URL` points at
it whenever it exists; until then the server logs, at startup, exactly what will happen
without it.

The token contract is fixed and documented, so the relay can be written against
[`provenance.md`](provenance.md) without touching this code.

### DataStore

**Super Mario Bros. 35 does not use DataStore at all.** The public reference implementation (kinnay/SMB35) registers no
DataStore service.

This is worth being explicit about, because OpenPak has a real DataStore, which is a genuine
advantage on other titles in the fleet. It buys nothing here: there
is no stub to replace. The right DataStore implementation for this title is none.

### Things not implemented

Ranking2 `PutScore`, `DeleteCommonData`, `GetRankingByPrincipalID` and the ranking charts;
Matchmake Referee's stats methods. The game does not call them. They answer
`Core::NotImplemented` through Pretendo's own dispatch, which is the correct answer and costs
no code.

### Per-player round results

`EndRound` carries each player's result, and this server reads past them. They are in the
Eagle variant's layout (see below), nothing in the game reads them back — no leaderboard, no
   rating, no stats call the client makes — so they are discarded. The round ID
decodes before the results, so closing a round is reliable regardless.

## The Eagle-variant structures

Super Mario Bros. 35 uses NintendoClients' *eagle* variant of the matchmaking protocol
module, and two of its structures carry fields Pretendo's generated Go types do not have:
`MatchmakeRefereeStartRoundParam` and `MatchmakeRefereePersonalRoundResult` each end with a
`reportSummaryMode` (u8) and an `eventID` (u32). `AutoMatchmakeParam` similarly ends with a
block-list parameter Pretendo does not model.

All of these are trailing fields, and in each case the structure is the last thing in its
message, so the leading fields — the ones this server actually acts on — decode correctly and
the extra bytes are simply not read.

The one place that would break is `GetStartRoundParam`, whose response has to be the *whole*
parameter structure: a client that sent five fields and gets three back cannot parse the
reply. So `StartRound` keeps the raw request body and `GetStartRoundParam` returns it
verbatim. The request body of `StartRound` *is* the parameter structure, under the same
library version, so this is not a way around the encoding — it is the encoding.

The ceiling: this server cannot look at `reportSummaryMode` or `eventID`. The upgrade path is
to declare the Eagle-variant types here and decode them properly, the day something needs to
act on a value. Both places are marked with a `ponytail:` comment.

## The transport, and why this title is the odd one

Switch NEX titles speak **PRUDP over WebSocket**, not PRUDP over UDP. More than that, Super
Mario Bros. 35 expects auth and secure on **one** listener, separated by virtual port: auth
on stream 1, secure on stream 2. The login response's secure `StationURL` is a placeholder —
address `0.0.0.1`, port `1` — with `sid=2`. The console never opens a second socket.

None of that was expressible when this repository was started; `openpak/nex` listened on UDP
with two endpoints on two ports, both on stream 1, and a secure PID of 2. It is expressible
now — `servers/nex` took the four changes as configuration:

| Setting | Value here | Why |
| --- | --- | --- |
| `NEX_TRANSPORT` | *(unset — websocket is the default)* | every Switch title |
| `NEX_SECURE_STREAM_ID` | `2` | puts both endpoints on the listener at `+0`; `+1` is reserved and unused |
| `NEX_SECURE_PID` | `257049437023956657` | the retail constant; feeds the ticket target, the station URL PID and the notification source |
| `NEX_STATION_SCHEME` | `prudps` | per title, and a wrong value fails late rather than loudly |
| `NEX_VERSION` | `40600` | 4.6.0 |
| `NEX_SOURCE_KEY_BYTES` | `16` | per title, and the host's default is 32 — see below |

With those, the retail station URL falls out of the host with no override:
`prudps:/address=0.0.0.1;port=1;PID=257049437023956657;CID=1;sid=2;stream=10;type=2`.

The source key length is the other per-title value. `servers/nex` defaults to 32, which has
same-title evidence behind it on Mario Kart 8 Deluxe; this title has same-title evidence for
16, from a server that worked against a retail client. A wrong length is a login
that fails with nothing logged on either side, which is why it is set explicitly here rather
than left to a default. `servers/nex` v0.2.0 made it configurable after this came up.

The single-listener shape is **specific to this title**, not the fleet default — other Switch
titles in OpenPak use two listeners. It is set here because this is what a server
that worked against a retail client did.

## Open questions — what the next round needs

Everything below needs a real client and a capture. None of it is guessed at in code.

1. **Does the handoff notification actually reach the game?** Earlier research notes record an Eagle handoff
   being sent to a real PAC-MAN 99 client and never honoured, and could
   not tell whether the console processes pushed notifications at all. If the notification
   channel is dead on this title too, every lobby hangs and nothing in the NEX server can
   tell. **Evidence needed:** a client that visibly reacts to *any* server-pushed
   notification. Type 109000 (gathering destroyed) is the good probe: the game must leave
   the lobby, so no reaction means no listening.
2. **Is the revision 1 notification accepted?** The map of parameters only exists in
   revision 1, and earlier research notes record a title that refused the map-carrying variant.
   **Evidence needed:** the game opening an Eagle socket after a handoff.
3. **Does the game reach us over WebSocket on our block?** The host can now serve the shape
   above, but nothing has been in front of it. **Evidence needed:** a CONNECT on the auth
   stream and its virtual port, which also settles whether stream 2 is right for secure. If
   nex-go logs "trying to connect to unbound PRUDPEndPoint", the stream IDs are wrong.
4. **Do the integer settings at index 0 mean what the public reference implementation (kinnay/SMB35) recorded?** They are carried
   across unchanged and nobody knows what any index controls. **Evidence needed:** a match
   that runs, and then one changed value at a time.
5. **Is index 10 really empty?** The public reference implementation (kinnay/SMB35) left it empty with an open question in its own
   source. **Evidence needed:** whether the game misbehaves when it gets nothing back.
6. **What does the relay have to do?** Out of scope here, but the blocker on playability.
   **Evidence needed:** the Eagle framing, beyond the wiki's summary of the transport and the
   handshake token.

There is also one blocker that belongs to nobody here: nx-baas cannot yet prove a real
console's BAAS id token, only an emulator's bound claim, so a genuine console login is
refused today. That is being handled elsewhere, and it fails as a clean refusal rather than a
hang.

Until 1–3 are answered on hardware, what this repository can claim is: it builds, it serves
`/healthz`, it registers every protocol the game calls with correct wire types and correct
values, and its handoff token is byte-checkable against the documented format. That is what
the tests check.

## Sources

- [kinnay/SMB35](https://github.com/kinnay/SMB35) — an archived, publicly hosted server that
  worked against a retail client. AGPL-3.0.
- [NintendoClients wiki — Game Server List](https://github.com/kinnay/NintendoClients/wiki/Game-Server-List)
- [NintendoClients wiki — Eagle Protocol](https://github.com/kinnay/NintendoClients/wiki/Eagle-Protocol)
- NintendoClients `nintendo/nex/matchmaking_eagle.py` and `ranking2_eagle.py`
- Pretendo's `nex-go/v2`, `nex-protocols-go/v2`, `nex-protocols-common-go/v2`, read as
  AGPL-3.0 source
- OpenPak's own [`servers/nex`](../../../README.md), `nx-baas` and [`ports.md`](https://docs.openpak.org/contributors/ports/)
