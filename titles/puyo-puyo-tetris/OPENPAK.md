# Puyo Puyo Tetris (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/puyo-puyo-tetris` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=puyo-puyo-tetris` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_PUYOPUYOTETRIS_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_PUYOPUYOTETRIS_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_PUYOPUYOTETRIS_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_PUYOPUYOTETRIS_AUTHENTICATION_SERVER_PORT` |
| `FriendsGRPCKey` | `PN_PUYOPUYOTETRIS_FRIENDS_GRPC_API_KEY` |
| `hostOf(FriendsGRPCAddr)` | `PN_PUYOPUYOTETRIS_FRIENDS_GRPC_HOST` |
| `portOf(FriendsGRPCAddr)` | `PN_PUYOPUYOTETRIS_FRIENDS_GRPC_PORT` |
| `DatabaseURL` | `PN_PUYOPUYOTETRIS_POSTGRES_URI` |
| `SecureHost` | `PN_PUYOPUYOTETRIS_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_PUYOPUYOTETRIS_SECURE_SERVER_PORT` |

Health: upstream's UDP health check (`PN_PUYOPUYOTETRIS_HEALTH_CHECK_PORT`) stays off; nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_PUYOPUYOTETRIS_CONFIG_S3_ACCESS_KEY`, `PN_PUYOPUYOTETRIS_CONFIG_S3_ACCESS_SECRET`, `PN_PUYOPUYOTETRIS_CONFIG_S3_BUCKET`, `PN_PUYOPUYOTETRIS_CONFIG_S3_ENDPOINT`, `PN_PUYOPUYOTETRIS_CONFIG_S3_INSECURE`, `PN_PUYOPUYOTETRIS_CONFIG_S3_KEY_BASE`, `PN_PUYOPUYOTETRIS_KERBEROS_PASSWORD`, `PN_PUYOPUYOTETRIS_LOCAL_AUTH`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.
