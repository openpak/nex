# Kid Icarus: Uprising (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/kid-icarus-uprising` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=kid-icarus-uprising` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_KIU_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_KIU_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_KIU_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_KIU_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_KIU_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_KIU_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_KIU_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_KIU_POSTGRES_URI` |
| `SecureHost` | `PN_KIU_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_KIU_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
