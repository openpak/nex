# Terraria (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/terraria` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=terraria` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_TERRARIA_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_TERRARIA_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_TERRARIA_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_TERRARIA_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_TERRARIA_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_TERRARIA_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_TERRARIA_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_TERRARIA_POSTGRES_URI` |
| `SecureHost` | `PN_TERRARIA_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_TERRARIA_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_TERRARIA_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Status (2026-11-01):** imported and builds in the shared image. No `servers` row and no container yet; not console-verified.
