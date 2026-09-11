# Yo-kai Watch 2 (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/yo-kai-watch-2` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=yo-kai-watch-2` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_YKW2_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_YKW2_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_YKW2_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_YKW2_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_YKW2_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_YKW2_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_YKW2_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_YKW2_POSTGRES_URI` |
| `SecureHost` | `PN_YKW2_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_YKW2_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_YKW2_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
