# Monster Hunter 4 Ultimate (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/monster-hunter-4-ultimate` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=monster-hunter-4-ultimate` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_MH4U_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_MH4U_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_MH4U_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_MH4U_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_MH4U_POSTGRES_URI` |
| `SecureHost` | `PN_MH4U_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_MH4U_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-11):** 3DS title ids not found in any public list; no nn-account row yet. The container runs; a console cannot be pointed at it until that fact is observed.
