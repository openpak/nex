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

**Registration (2026-09-11):** row in nn-account's `servers` table (title ids from Pretendo's nex-viewer title database; Wii U game-server ids from Kinnay's Wii U list, where the id is the low 32 bits of the title id). Deployed as one container from the shared image. Not console-verified.

**Event course metadata (2026-09-11):** upstream refuses to start unless `900000.bin` exists in the S3 bucket (the reserved DataID 900000 that opens Course World). Production holds an **empty placeholder** so the server runs; Course World needs the real event-course metadata blob uploaded over it.
