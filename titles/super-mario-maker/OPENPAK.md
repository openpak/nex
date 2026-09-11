# Super Mario Maker (Wii U) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/super-mario-maker` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=super-mario-maker` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_SMM_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_SMM_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_SMM_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_SMM_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_SMM_POSTGRES_URI` |
| `SecureHost` | `PN_SMM_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SMM_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_SMM_CONFIG_S3_ACCESS_KEY`, `PN_SMM_CONFIG_S3_ACCESS_SECRET`, `PN_SMM_CONFIG_S3_BUCKET`, `PN_SMM_CONFIG_S3_ENDPOINT`, `PN_SMM_CONFIG_S3_KEY_BASE`, `PN_SMM_CONFIG_S3_SECURE`, `PN_SMM_GRPC_API_KEY`, `PN_SMM_GRPC_SERVER_PORT`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.
