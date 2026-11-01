# Mario Kart 7 (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/mario-kart-7` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=mario-kart-7` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_MK7_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_MK7_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_MK7_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_MK7_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_MK7_POSTGRES_URI` |
| `SecureHost` | `PN_MK7_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_MK7_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

Additional variables (all optional):

| Variable | Meaning |
| --- | --- |
| `MK7_API_ADDR` | read API listen address (production `:22855`); unset = off. See `docs/read-api.md` |
| `MK7_S3_ENDPOINT`, `MK7_S3_BUCKET`, `MK7_S3_ACCESS_KEY`, `MK7_S3_SECRET_KEY`, `MK7_S3_INSECURE=1` | S3 store for ghost bodies; unset = bodies not served |

Rankings (legacy Ranking protocol) and ghosts (DataStore) are stored in the Postgres schema `mk7`;
see `docs/README.md` for what is fact and what is inferred, and the console test.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
