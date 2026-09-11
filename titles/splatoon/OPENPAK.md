# Splatoon (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/splatoon` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=splatoon` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_SPLATOON_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_SPLATOON_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_SPLATOON_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_SPLATOON_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_SPLATOON_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_SPLATOON_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_SPLATOON_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_SPLATOON_POSTGRES_URI` |
| `SecureHost` | `PN_SPLATOON_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SPLATOON_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_SPLATOON_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_SPLATOON_LOCAL_AUTH`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-11):** row in nn-account's `servers` table (title ids from Pretendo's nex-viewer title database; Wii U game-server ids from Kinnay's Wii U list, where the id is the low 32 bits of the title id). Deployed as one container from the shared image. Not console-verified.
