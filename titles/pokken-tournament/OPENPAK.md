# Pokkén Tournament (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/pokken-tournament` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=pokken-tournament` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_POKKENTOURNAMENT_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_POKKENTOURNAMENT_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_POKKENTOURNAMENT_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_POKKENTOURNAMENT_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_POKKENTOURNAMENT_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_POKKENTOURNAMENT_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_POKKENTOURNAMENT_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_POKKENTOURNAMENT_POSTGRES_URI` |
| `SecureHost` | `PN_POKKENTOURNAMENT_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_POKKENTOURNAMENT_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_POKKENTOURNAMENT_S3_ACCESS_KEY`, `PN_POKKENTOURNAMENT_S3_ACCESS_SECRET`, `PN_POKKENTOURNAMENT_S3_BUCKET`, `PN_POKKENTOURNAMENT_S3_ENDPOINT`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.
