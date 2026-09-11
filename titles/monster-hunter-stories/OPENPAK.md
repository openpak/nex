# Monster Hunter Stories (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/monster-hunter-stories` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=monster-hunter-stories` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_MHS_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_MHS_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_MHS_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_MHS_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_MHS_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_MHS_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_MHS_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_MHS_POSTGRES_URI` |
| `SecureHost` | `PN_MHS_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_MHS_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_MHS_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
