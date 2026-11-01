# Sonic & All-Stars Racing Transformed (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/sonic-all-stars-racing-transformed-wiiu` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=sonic-all-stars-racing-transformed-wiiu` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_SASRTWIIU_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_SASRTWIIU_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_SASRTWIIU_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_SASRTWIIU_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_SASRTWIIU_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_SASRTWIIU_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_SASRTWIIU_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_SASRTWIIU_POSTGRES_URI` |
| `SecureHost` | `PN_SASRTWIIU_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SASRTWIIU_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_SASRTWIIU_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Deployed (2026-11-01, nex v1.4.0):** container `openpak-sonic-all-stars-racing-transformed-wiiu`, block 22920 (auth 22920/udp, secure 22921/udp, host firewall open), database `sasrtwiiu`. nn-account `servers` row(s) applied on prod for game server `10111F00` (Kinnay's Wii U NEX list and upstream's PN validation). Not console-verified.
