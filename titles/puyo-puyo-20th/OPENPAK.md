# Puyo Puyo!! 20th Anniversary (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/puyo-puyo-20th` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=puyo-puyo-20th` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_PP20_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_PP20_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_PP20_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_PP20_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_PP20_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_PP20_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_PP20_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_PP20_POSTGRES_URI` |
| `SecureHost` | `PN_PP20_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_PP20_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_PP20_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_PP20_LOCAL_AUTH`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
