# Metroid Prime: Federation Force / Blast Ball (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/metroid-prime-federation-force` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=metroid-prime-federation-force` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_MPFF_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_MPFF_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_MPFF_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_MPFF_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_MPFF_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_MPFF_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_MPFF_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_MPFF_POSTGRES_URI` |
| `SecureHost` | `PN_MPFF_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_MPFF_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.
