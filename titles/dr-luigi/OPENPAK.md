# Dr. Luigi (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/dr-luigi` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=dr-luigi` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_POSTGRES_URI` |
| `SecureHost` | `PN_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-12):** row prepared in [`servers/nex/registration.sql`](../../registration.sql) section A, same derivation as every registered Wii U title (title ids JPN 0005000010150300, USA 0005000010152A00, EUR 0005000010152B00). Applies with the next deployment pass; container runs today.
