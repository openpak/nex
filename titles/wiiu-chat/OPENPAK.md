# Wii U Chat on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/wiiu-chat` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=wiiu-chat` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_WUC_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_WUC_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_WUC_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_WUC_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_WUC_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_WUC_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_WUC_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_WUC_POSTGRES_URI` |
| `SecureHost` | `PN_WUC_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_WUC_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-12):** rows prepared in [`servers/nex/registration.sql`](../../registration.sql) section A from the system-applet title ids (JPN 000500101005A000, USA 000500101005A100, EUR 000500101005A200, WiiBrew). The low-32 game-server-id rule has only been observed on games, so these are marked unobserved — a console that asks and misses answers 1021 exactly as with no row. Applies with the next deployment pass.
