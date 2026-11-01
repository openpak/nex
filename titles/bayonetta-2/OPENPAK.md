# Bayonetta 2 (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/bayonetta-2` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=bayonetta-2` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_BAYONETTA2_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_BAYONETTA2_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_BAYONETTA2_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_BAYONETTA2_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_BAYONETTA2_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_BAYONETTA2_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_BAYONETTA2_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_BAYONETTA2_POSTGRES_URI` |
| `SecureHost` | `PN_BAYONETTA2_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_BAYONETTA2_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_BAYONETTA2_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Deployed (2026-11-01, nex v1.4.0):** container `openpak-bayonetta-2`, block 22910 (auth 22910/udp, secure 22911/udp, host firewall open), database `bayonetta2`. nn-account `servers` row(s) applied on prod for game server `1011B900` (from upstream's PN validation). Not console-verified.
