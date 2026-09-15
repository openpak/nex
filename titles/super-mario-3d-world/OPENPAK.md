# Super Mario 3D World (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/super-mario-3d-world` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=super-mario-3d-world` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_SM3DW_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_SM3DW_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_SM3DW_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_SM3DW_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_SM3DW_POSTGRES_URI` |
| `SecureHost` | `PN_SM3DW_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SM3DW_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-12):** row prepared in [`servers/nex/registration.sql`](../../registration.sql) section A — Wii U game-server id = low 32 bits of the title id (holds for all 41 entries of Kinnay's list; title ids JPN 0005000010106100, USA 0005000010145C00, EUR 0005000010145D00 from Kinnay's title database, one row per region). Applies with the next deployment pass; container runs today.
