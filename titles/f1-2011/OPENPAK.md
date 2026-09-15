# F1 2011 (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/f1-2011` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=f1-2011` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_F1_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_F1_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_F1_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_F1_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_F1_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_F1_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_F1_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_F1_POSTGRES_URI` |
| `SecureHost` | `PN_F1_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_F1_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_F1_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-12):** 3DS title ids found in the public 3dsdb lists (EUR 000400000006A600, JPN 0004000000067B00); the game-server id the console sends stays unobserved, and NASC rejects a row whose id does not match (error 152). Prepared row held in [`servers/nex/registration.sql`](../../registration.sql) section B until one console observation supplies the id.
