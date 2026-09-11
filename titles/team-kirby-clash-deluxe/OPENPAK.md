# Team Kirby Clash Deluxe (3DS) on OpenPak

Adapted from Pretendo's `github.com/PretendoNetwork/team-kirby-clash-deluxe` (AGPL-3.0); `README.md` beside this file is upstream's.
Runs as `NEX_TITLE=team-kirby-clash-deluxe` in the shared `ghcr.io/openpak/nex` image, one container per title.

| From the host Config | Upstream variable |
| --- | --- |
| `AccountGRPCKey` | `PN_TKCD_ACCOUNT_GRPC_API_KEY` |
| `hostOf(AccountGRPCAddr)` | `PN_TKCD_ACCOUNT_GRPC_HOST` |
| `portOf(AccountGRPCAddr)` | `PN_TKCD_ACCOUNT_GRPC_PORT` |
| `strconv.Itoa(AuthPort())` | `PN_TKCD_AUTHENTICATION_SERVER_PORT` |
| `DatabaseURL` | `PN_TKCD_POSTGRES_URI` |
| `SecureHost` | `PN_TKCD_SECURE_SERVER_HOST` |
| `strconv.Itoa(SecurePort())` | `PN_TKCD_SECURE_SERVER_PORT` |

Health: nexd answers `/healthz` on the block's `+2` port.

No other variables.

Consoles reach it through nn-account: the title's row in nn-account's `servers` table names this
container's host and the block's `+0` port.
