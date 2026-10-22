# Provenance

Where every non-obvious fact in this repository came from. Required by
[`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md): facts about a console and a
protocol are facts, and this file is what makes "we rewrote it from the facts" checkable.

Sources, referred to below by these names:

- **Kinnay** — [kinnay/NintendoClients wiki](https://github.com/kinnay/NintendoClients/wiki),
  public reverse-engineering documentation.
- **Notes** — earlier research notes on this title, facts only.
- **Pretendo** — the AGPL-3.0 `nex-go`, `nex-protocols-go` and `nex-protocols-common-go`
  sources, which OpenPak links against and may therefore also read.
- **ports.md** — OpenPak's own port plan.

## Identity of the title

| Fact | Value | Source |
| --- | --- | --- |
| NEX access key | `29f267b9` | Kinnay's Game Server List, recorded in the notes. Configuration here, never a literal in source (`NEX_ACCESS_KEY`) |
| Game server ID | `23932A00` | Same entry. It is the `g23932a00-lp1…` hostname the console resolves — **not** the access key, and one cannot be derived from the other |
| NEX version | `4.3.5` (`40305`) — **unverified** | the notes say in as many words that no public source documents this title's version, and that 4.3.5 was chosen because ARMS is a contemporaneous release. Carried across as a starting guess, not a fact |

## Transport and endpoint shape

| Fact | Value | Source |
| --- | --- | --- |
| Transport | PRUDP over WebSocket, TLS | the notes: both endpoints listen with a certificate and key, on ports 443 and 60010 |
| Listener shape | **Two** listeners, authentication and secure on separate ports | Same: two ports, both listening. This is the ARMS/Mario Golf shape in `servers/nex`'s README table, not SMB35's single listener |
| Secure endpoint stream ID | 1 | the notes: the secure station URL carries `sid=1` |
| Station URL | scheme `prudps`, `CID=1`, `PID=2`, `sid=1`, `stream=10`, `type=2` | the notes. `stream=10` is the RVSecure stream type, which is what `servers/nex` sets from `constants.StreamTypeRVSecure` |
| Secure server PID | 2 | Same. The conventional NEX value, and `servers/nex`'s default |
| Session key length | 32 bytes | the notes |
| PRUDP minor version 0 on the secure endpoint | not needed here | Earlier work set it explicitly. nex-go answers with the minor version the client sent (`prudp_endpoint.go`, *"No change needed, we can just support what the client wants"*), so there is nothing to configure |

## Protocols the title speaks

The set registered on the secure endpoint in the notes is the observed set. IDs are
Kinnay's and match Pretendo's constants.

| Protocol | ID | Implemented here by |
| --- | --- | --- |
| Secure Connection | `0x0B` | `servers/nex` (it registers this itself) |
| Match Making | `0x15` | `nex-protocols-common-go` |
| Matchmake Extension | `0x6D` | `nex-protocols-common-go`, plus two additions below |
| Match Making Ext | `0x32` | `nex-protocols-common-go` |
| NAT Traversal | `0x03` | `nex-protocols-common-go` |
| Ranking | `0x70` | `nex-protocols-common-go` over our Postgres store |
| Utility | `0x6E` | `nex-protocols-common-go` |
| DataStore | `0x73` | `nex-protocols-common-go` over our Postgres store |

## Behaviours the console has been observed to have

| Fact | Source |
| --- | --- |
| A freshly created session stays **closed** until a separate, later `OpenParticipation` call. Two players searching in that window each miss the other and each create a session, and both then wait forever — a "double host" deadlock. Hit and fixed on 2026-08-16 | the notes, which record the flag added to close the race. Pretendo's search query has the matching half: `find_matchmake_session.go` selects only rows with `ms.open_participation=true` |
| Playing with a friend goes through `FindMatchmakeSessionByParticipant`, method `0x33` (51) of protocol `0x6D`. Left unanswered it returns an empty list and a friend's open room is never found | the notes enable exactly this method for this title; the method ID is Pretendo's `MethodFindMatchmakeSessionByParticipant` |
| DataStore `GetMeta` (method 8) is called, and earlier work answered it `NotFound` `0x80690004` | the notes. That code is Pretendo's `ResultCodes.DataStore.NotFound` (`0x00690004`) with the error bit set |
| DataStore `PostMetaBinary` (method 21) is called with tournament entry data and **must** return a `u64` data id. Answering it with an empty list left the client with no object id and it died silently — found live on 2026-08-16 | the notes |
| `SecureConnection::TestConnectivity` (method 5) is called by this title — 23 times in six hours on 2026-08-23. It takes no parameters and wants an empty response | the notes, which name Mario Tennis Aces as the title the measurement came from |
| Pia sends an undocumented PRUDP packet type 8 as a keepalive on the secure stream | the notes, which note this title may never send one. No evidence it does |
| The public station type a Switch expects is Pia-version specific: `0x0B` for Pia 5.19 (SSBU), `0x03` for Splatoon 2 | the notes. Unknown for this title; earlier work used the plain default and never had to change it |

## Why Rankings needed an implementation

An earlier Ranking implementation answered methods `0x04`, `0x10`, `0x11`, `0x12`, `0x13`
and `0x16` — the *custom* Ranking methods Mario Kart 8 Deluxe uses, written from an MK8
measurement — and its default branch answered `Core::NotImplemented` to everything else. The
standard Ranking protocol (`UploadScore` `0x1`, `GetCommonData` `0x6`, `GetRanking` `0x9`,
`GetCachedTopXRanking` `0xE`, `GetCachedTopXRankings` `0xF`) was therefore unanswered.

That is a missing implementation, not a missing fact. Pretendo's
`nex-protocols-common-go/v2/ranking` implements all five of those methods; the method IDs and
the `RankingScoreData` / `RankingOrderParam` / `RankingRankData` structures are Pretendo's,
read as AGPL source. See [`design.md`](design.md) for what that means in practice.

## OpenPak's own facts

| Fact | Value | Source |
| --- | --- | --- |
| NEX port block | 22170: auth 22170, secure 22171, `/healthz` 22172 | ports.md, claimed by this repository |
| Account identity, friend graph | `Host.PlayerByPID` / `Host.FriendPIDs` over nx-baas `/internal/switch/identity` | `servers/nex` |
| Matchmaking, tracking and utility tables | created by `nex-protocols-common-go` when its managers are set | Pretendo source; this repository creates only the `datastore` and `ranking` schemas |
