# IRONFALL Invasion (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/ironfall-invasion` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=ironfall-invasion` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_IRONFALL_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_IRONFALL_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_IRONFALL_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_IRONFALL_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_IRONFALL_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_IRONFALL_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_IRONFALL_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_IRONFALL_POSTGRES_URI` |
| `SecureHost` | `PN_IRONFALL_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_IRONFALL_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-11):** row in nn-account's `servers` table (title ids from Pretendo's nex-viewer title database; Wii U game-server ids from Kinnay's Wii U list, where the id is the low 32 bits of the title id). Deployed as one container from the shared image. Not console-verified.
