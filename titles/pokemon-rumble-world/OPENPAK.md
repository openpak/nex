# Pokémon Rumble World (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/pokemon-rumble-world` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=pokemon-rumble-world` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_PRW_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_PRW_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_PRW_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_PRW_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_PRW_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_PRW_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_PRW_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_PRW_POSTGRES_URI` |
| `SecureHost` | `PN_PRW_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_PRW_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_PRW_HPP_SERVER_PORT`, `PN_PRW_S3_ACCESS_KEY`, `PN_PRW_S3_ACCESS_SECRET`, `PN_PRW_S3_BUCKET`, `PN_PRW_S3_ENDPOINT`, `PN_PRW_S3_REGION`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
