# Dr. Mario: Miracle Cure (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/miracle-cure` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=miracle-cure` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_POSTGRES_URI` |
| `SecureHost` | `PN_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Pass-through variables (set them in the container environment as upstream documents): `PN_KERBEROS_PASSWORD`

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port; the console asks nn-account for the game server by
`game_server_id` and gets that address, its NEX password and a token.

**Registration (2026-09-12):** 3DS title ids found (Dr. Mario: Miracle Cure, USA+EUR 000400000013BB00, 3dsdb); game-server id unobserved — prepared row in [`servers/nex/registration.sql`](../../registration.sql) section B until observed.
