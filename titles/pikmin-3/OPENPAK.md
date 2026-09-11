# Pikmin 3 (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/pikmin-3` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=pikmin-3` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_PIKMIN3_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_PIKMIN3_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_PIKMIN3_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_PIKMIN3_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_PIKMIN3_POSTGRES_URI` |
| `SecureHost` | `PN_PIKMIN3_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_PIKMIN3_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.
