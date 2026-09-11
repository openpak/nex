# Minecraft: Wii U Edition on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/minecraft-wiiu` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=minecraft-wiiu` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_MINECRAFT_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_MINECRAFT_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_MINECRAFT_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_MINECRAFT_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_MINECRAFT_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_MINECRAFT_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_MINECRAFT_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_MINECRAFT_POSTGRES_URI` |
| `SecureHost` | `PN_MINECRAFT_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_MINECRAFT_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_MINECRAFT_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_MINECRAFT_ALLOW_PUBLIC_MATCHMAKING`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
