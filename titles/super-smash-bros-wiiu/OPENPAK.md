# Super Smash Bros. for Wii U on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/super-smash-bros-wiiu` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=super-smash-bros-wiiu` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_SSBWIIU_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_SSBWIIU_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_SSBWIIU_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_SSBWIIU_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_SSBWIIU_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_SSBWIIU_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_SSBWIIU_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_SSBWIIU_POSTGRES_URI` |
| `SecureHost` | `PN_SSBWIIU_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SSBWIIU_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_SSBWIIU_DATASTORE_S3BUCKET`, `PN_SSBWIIU_DATASTORE_S3KEY`, `PN_SSBWIIU_DATASTORE_S3SECRET`, `PN_SSBWIIU_DATASTORE_S3URL`, `PN_SSBWIIU_HPP_SERVER_PORT`, `PN_SSBWIIU_LOCAL_AUTH`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.
