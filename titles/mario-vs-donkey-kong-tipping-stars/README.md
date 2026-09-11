# mario-vs-donkey-kong-tipping-stars

Mario vs. Donkey Kong: Tipping Stars on the shared NEX host, one server for the Wii U and 3DS
versions (the game is cross-buy and shares its online data). Written in Go for OpenPak on
nex-go v2 and the common DataStore protocol; it replaces Pretendo's nex-go v1 server
(`mario-vs-dk-tipping-stars-secure`, AGPL-3.0), whose behaviour it mirrors.

| Fact | Value | Source |
| --- | --- | --- |
| Title ids | Wii U `0005000010149300` / `178E00` / `179200`; 3DS `000400000012C800` / `CA00` | Pretendo nex-viewer title database |
| NEX | 3.7.1, PRUDP v1 over UDP | Pretendo's server (`SetDefaultNEXVersion`, `SetPRUDPVersion(1)`) |
| Access key | `d8927c3f` | Pretendo's server; configuration here (`NEX_ACCESS_KEY`), never a literal in source |
| Game-server ids in nn-account | `10149300` (Wii U), `0012C800` (3DS) | low 32 bits of the lead title id per console, the Wii U convention |

## What the game does with the server

Only DataStore, and only meta-binary objects (no file bodies, so no S3 is needed; `MVDK_S3_*`
stays unset): the player profile in persistence slot 0 (data type 122), the tip bucket in slot
1 (data type 123, `DeleteLastObject` on re-post), `GetMeta` by persistence target, `GetMetas`
by ids, `ChangeMeta`, `DeleteObject`, `RateObject` on three slots of the tip bucket (extra-tip
total, extra-tip count, play count). The common DataStore protocol on the Postgres store the
Switch titles use implements every one of those generically; the ratings table carries the
three slots from the game's own `RatingInitParams`. Level sharing itself is Miiverse
(nn-juxtaposition), not this server.

Not console-verified.
