# Deploy: nex

## Deployed shape

Production runs the passthrough form: Traefik routes each title's `g<id>-lp1.s.n.srv.nintendo.net`
SNI to the title container's `+0` port and the title terminates TLS with one OpenPak-CA leaf
covering `*.s.n.srv.nintendo.net` (`NEX_TLS_CERT`/`NEX_TLS_KEY`). Two-listener titles also
publish `+1` on the host, because the secure station URL names that port directly. See
[`ports.md`](https://docs.openpak.org/contributors/ports/).

## Per-title settings

A Switch title typically wants:

```sh
# websocket is already the default; if Traefik passes the SNI through rather than
# terminating it, this server needs the cert:
NEX_TLS_CERT=/certs/title.crt NEX_TLS_KEY=/certs/title.key
```

### The per-title values

Three settings are genuinely per title, and **every one of them fails silently or late** —
none produces a red test or a useful error. That is why they are stated here with their
failure mode, not just their value.

| Setting | Value | Known for | Failure mode when wrong |
| --- | --- | --- | --- |
| `NEX_SECURE_STREAM_ID` | `1` (default) — two listeners, `+0` auth and `+1` secure, real dialable secure URL | ARMS, Mario Golf, Mario Party Superstars | Not silent, for once: nex-go logs `trying to connect to unbound PRUDPEndPoint 2` for every packet. **That line means set it to `2`** |
| | `2` — one listener on `+0`, placeholder secure URL, `+1` reserved | Super Mario Bros. 35 | Console reaches nothing on the secure stream |
| `NEX_STATION_SCHEME` | `prudps` (default) | ARMS, Mario Party Superstars, Splatoon 2, SSBU | **Late.** Under `prudp` the handshake completes but CONNECT carries an empty payload, so the title works through menus and dies the moment it first needs the session |
| | `prudp` | Mario Kart 8 Deluxe | As above, mirrored |
| `NEX_SOURCE_KEY_BYTES` | `32` (default) | Mario Kart 8 Deluxe, Mario Party Superstars — both derive it with SHA-256 and return all 32 | **Fully silent.** The login fails with nothing logged on either side |
| | `16` | observed working against a retail Super Mario Bros. 35 | As above |
| `NEX_SECURE_TRANSPORT` | unset (default) — the secure listener takes `NEX_TRANSPORT` | every title but one | — |
| | `udp` alongside `NEX_TRANSPORT=websocket` | Monster Hunter Generations Ultimate: a Switch port of a 3DS game, whose auth stage the console reaches over TLS by SNI while its secure-connect code opens a UDP socket | **Fully silent, and it was wrong for months.** A UDP auth listener behind a TCP SNI route never receives a packet, and nothing logs a connection that was refused by the kernel |

On the source key specifically: the servers that use 32 tend to carry a comment claiming it
*must* be 32, "the Switch kerberos key size". **That generalisation is wrong** — the 16-byte
counter-example is a server that works against a retail console. RC4 takes any key length and
the console is told the key rather than deriving it, so the only thing that must hold is that
the key the ticket is encrypted with is the key advertised in the response. Do not propagate
the "MUST".

Title servers are encouraged to pin all three in their own repository rather than inherit the
default here, so the measured value lives next to the measurement.

## Deploy

Tag `v1.2.3` and the release workflow publishes `ghcr.io/openpak/nex:1.2.3` and `:latest` for
amd64 and arm64; the deploy host runs `podman-auto-update` against `:latest`.

## Test

```sh
go build ./...
go test ./...
go vet ./...
```

The tests cover the parts that are not one-liners: config validation, both spellings of a NEX
version, reading the binding claim out of a token *and* accepting a real console's token that
has none, the nx-baas identity call including the refused-token case, that a login name is an
NSA id rather than a PID, the per-title station scheme, and a full kerberos ticket round trip
— minted by the authentication endpoint, opened with the source key the client is handed, and
the internal ticket opened again by the secure server.
