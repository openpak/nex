# kirbys-dream-buffet

Kirby's Dream Buffet (`0100A8E016236000`) on the shared NEX host. **Scaffold.** Port block `22380`
([ports.md](https://docs.openpak.org/contributors/ports/)): auth `22380`, secure `22381`, `/healthz` `22382`.

## What has to be found out before a console can use it

| Fact | Status | Where it comes from |
| --- | --- | --- |
| NEX access key (`NEX_ACCESS_KEY`) | **unknown**; the host refuses to start without one | the game binary, or a measured CONNECT signature (see `titles/minecraft/docs/provenance.md` for how that verification looks) |
| Game-server id, i.e. the `g<id>-lp1.s.n.srv.nintendo.net` host (`NEX_SECURE_HOST`) | **unknown** | the SNI the console dials on 443; not in Kinnay's Game Server List (checked 2026-09-11) |
| NEX library version | unknown, default `4.4.0` | `apiVersionGeneral` in the first packets |
| Protocol set actually called | unknown | `Core::NotImplemented` lines once a console talks to it |

## Run

```sh
NEX_TITLE=kirbys-dream-buffet NEX_PORT_BASE=22380 NEX_ACCESS_KEY=… NEX_SECURE_HOST=g…-lp1.s.n.srv.nintendo.net \
KDB_DATABASE_URL=postgres://… NX_INTERNAL_URL=… NX_INTERNAL_KEY=… go run ../../cmd/nexd
```
