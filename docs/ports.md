# Ports — nex (trimmed)

> Trimmed copy for this repository; the canonical document lives at
> `Openpak/ports.md` and governs. Last synchronised 2026-09-15.

One block per concern, nothing below 20000, nothing at or above 27000 (Photon-Reference and
Steam live there). Every service reads its listeners from `<SVC>_HTTP_ADDR`, `<SVC>_GRPC_ADDR`,
`<SVC>_METRICS_ADDR`; the values below are the defaults and the local-run convention. Each
service owns a block of ten: +0 HTTP, +1 gRPC, +2 metrics/pprof, +3..+9 spare.

## Game transport (22xxx)

| Port(s) | Service |
| --- | --- |
| Port(s) | Service |
| --- | --- |
| 22100–22199 | NEX title blocks 1–10 (auth +0, secure +1) |
| 22300–22599 | NEX title blocks 11–40 |
| 22700–22899 | NEX titles continued (Wii U / 3DS, UDP) |

### NEX blocks allocated

Every NEX title is a package under `servers/nex/titles/` built into the one `nexd` image
(`ghcr.io/openpak/nex`); a container runs one title, selected by `NEX_TITLE`. The host uses `+0`
authentication, `+1` secure and `+2` `/healthz` (HTTP).

Deployed shape (2026-09-10): the console dials `g<id>-lp1.s.n.srv.nintendo.net:443` for
authentication; Traefik has one `HostSNI` passthrough router per title to `openpak-<title>:+0`,
and each title terminates TLS itself with `deploy/certs/nex.pem` (one OpenPak-CA leaf:
`*.s.n.srv.nintendo.net` plus every title's host). Two-listener titles publish `+1` on the host
because the secure station URL names it directly. `minecraft` has no `NEX_SECURE_HOST` yet, so
no router: its game-server id is not public (Splatoon 2 and Mario Maker 2 got theirs from
Kinnay's Game Server List on 2026-09-11).

| Block | Repository | auth | secure | healthz |
| --- | --- | --- | --- | --- |
| 22100 | `servers/nex` — shared host reference instance | 22100 | 22101 | 22102 |
| 22110 | `servers/nex/titles/mario-party-superstars` — Mario Party Superstars (NEX 4.6.5) | 22110 | 22111 | 22112 |
| 22120 | `servers/nex/titles/monster-hunter-generations-ultimate` — Monster Hunter Generations Ultimate (NEX 4.4.0) | 22120 | 22121 | 22122 |
| 22130 | `servers/nex/titles/mario-golf-super-rush` — Mario Golf: Super Rush (NEX 4.6.5); wss, one listener carrying both streams | 22130 | — (stream 2 on 22130) | 22132 |
| 22140 | `servers/nex/titles/arms` — ARMS (NEX 4.3.5) | 22140 | 22141 | 22142 |
| 22150 | `servers/nex/titles/mario-kart-8-deluxe` — Mario Kart 8 Deluxe (NEX 4.0.0, ws) | 22150 | 22151 | 22152 |
| 22160 | `servers/nex/titles/super-mario-bros-35` — Super Mario Bros. 35 (NEX 4.6.0); ws, one listener carrying both streams | 22160 | — (stream 2 on 22160) | 22162 |
| 22170 | `servers/nex/titles/mario-tennis-aces` — Mario Tennis Aces (NEX 4.3.5, unverified); wss, two listeners | 22170 | 22171 | 22172 |
| 22180 | `servers/nex/titles/animal-crossing-new-horizons` — Animal Crossing: New Horizons (porting) | 22180 | 22181 | 22182 |
| 22190 | `servers/nex/titles/luigis-mansion-3` — Luigi's Mansion 3 (porting) | 22190 | 22191 | 22192 |
| 22300 | `servers/nex/titles/mario-strikers` — Mario Strikers: Battle League (porting) | 22300 | 22301 | 22302 |
| 22310 | `servers/nex/titles/minecraft` — Minecraft (porting) | 22310 | 22311 | 22312 |
| 22320 | `servers/nex/titles/splatoon-2` — Splatoon 2 (porting) | 22320 | 22321 | 22322 |
| 22330 | `servers/nex/titles/super-mario-maker-2` — Super Mario Maker 2 (porting; DataStore-heavy) | 22330 | 22331 | 22332 |
| 22340 | `servers/nex/titles/super-mario-odyssey` — Super Mario Odyssey (porting) | 22340 | 22341 | 22342 |
| 22350 | `servers/nex/titles/super-smash-bros-ultimate` — Super Smash Bros. Ultimate (porting) | 22350 | 22351 | 22352 |
| 22370 | `servers/nex/titles/super-mario-3d-world-bowsers-fury` — Super Mario 3D World + Bowser's Fury (scaffold; access key and game-server id unknown) | 22370 | 22371 | 22372 |
| 22380 | `servers/nex/titles/kirbys-dream-buffet` — Kirby's Dream Buffet (scaffold; access key and game-server id unknown) | 22380 | 22381 | 22382 |
| 22390 | `servers/nex/titles/animal-crossing-new-leaf` — Animal Crossing: New Leaf (3DS), Pretendo-derived, UDP | 22390 | 22391 | 22392 |
| 22400 | `servers/nex/titles/f1-2011` — F1 2011 (3DS), Pretendo-derived, UDP | 22400 | 22401 | 22402 |
| 22410 | `servers/nex/titles/ironfall-invasion` — IRONFALL Invasion (3DS), Pretendo-derived, UDP | 22410 | 22411 | 22412 |
| 22420 | `servers/nex/titles/luigis-mansion-2` — Luigi's Mansion: Dark Moon (3DS), Pretendo-derived, UDP | 22420 | 22421 | 22422 |
| 22430 | `servers/nex/titles/metroid-prime-federation-force` — Metroid Prime: Federation Force (3DS), Pretendo-derived, UDP | 22430 | 22431 | 22432 |
| 22440 | `servers/nex/titles/miracle-cure` — Dr. Mario: Miracle Cure (3DS), Pretendo-derived, UDP | 22440 | 22441 | 22442 |
| 22450 | `servers/nex/titles/monster-hunter-4-ultimate` — Monster Hunter 4 Ultimate (3DS), Pretendo-derived, UDP | 22450 | 22451 | 22452 |
| 22460 | `servers/nex/titles/monster-hunter-generations` — Monster Hunter Generations (3DS), Pretendo-derived, UDP | 22460 | 22461 | 22462 |
| 22470 | `servers/nex/titles/monster-hunter-xx` — Monster Hunter XX (3DS), Pretendo-derived, UDP | 22470 | 22471 | 22472 |
| 22480 | `servers/nex/titles/pokemon-gen6` — Pokémon X/Y/OR/AS (3DS), Pretendo-derived, UDP | 22480 | 22481 | 22482 |
| 22490 | `servers/nex/titles/steel-diver-sub-wars` — Steel Diver: Sub Wars (3DS), Pretendo-derived, UDP | 22490 | 22491 | 22492 |
| 22500 | `servers/nex/titles/super-smash-bros-3ds` — Super Smash Bros. for Nintendo 3DS, Pretendo-derived, UDP | 22500 | 22501 | 22502 |
| 22510 | `servers/nex/titles/super-street-fighter-iv` — Super Street Fighter IV 3D Edition (3DS), Pretendo-derived, UDP | 22510 | 22511 | 22512 |
| 22520 | `servers/nex/titles/dr-luigi` — Dr. Luigi (Wii U), Pretendo-derived, UDP | 22520 | 22521 | 22522 |
| 22530 | `servers/nex/titles/pikmin-3` — Pikmin 3 (Wii U), Pretendo-derived, UDP | 22530 | 22531 | 22532 |
| 22540 | `servers/nex/titles/pokken-tournament` — Pokkén Tournament (Wii U), Pretendo-derived, UDP | 22540 | 22541 | 22542 |
| 22550 | `servers/nex/titles/puyo-puyo-tetris` — Puyo Puyo Tetris (Wii U), Pretendo-derived, UDP | 22550 | 22551 | 22552 |
| 22560 | `servers/nex/titles/splatoon` — Splatoon (Wii U), Pretendo-derived, UDP | 22560 | 22561 | 22562 |
| 22570 | `servers/nex/titles/super-mario-3d-world` — Super Mario 3D World (Wii U), Pretendo-derived, UDP | 22570 | 22571 | 22572 |
| 22580 | `servers/nex/titles/super-mario-maker` — Super Mario Maker (Wii U), Pretendo-derived, UDP | 22580 | 22581 | 22582 |
| 22590 | `servers/nex/titles/super-smash-bros-wiiu` — Super Smash Bros. for Wii U, Pretendo-derived, UDP | 22590 | 22591 | 22592 |
| 22700 | `servers/nex/titles/wiiu-chat` — Wii U Chat, Pretendo-derived, UDP | 22700 | 22701 | 22702 |
| 22710 | `servers/nex/titles/ice-station-z` — Ice Station Z (3DS), Pretendo-derived, UDP | 22710 | 22711 | 22712 |
| 22720 | `servers/nex/titles/kid-icarus-uprising` — Kid Icarus: Uprising (3DS), Pretendo-derived, UDP | 22720 | 22721 | 22722 |
| 22730 | `servers/nex/titles/mario-kart-7` — Mario Kart 7 (3DS), Pretendo-derived, UDP | 22730 | 22731 | 22732 |
| 22740 | `servers/nex/titles/minecraft-wiiu` — Minecraft: Wii U Edition, Pretendo-derived, UDP | 22740 | 22741 | 22742 |
| 22750 | `servers/nex/titles/monster-hunter-4` — Monster Hunter 4 (3DS), Pretendo-derived, UDP | 22750 | 22751 | 22752 |
| 22760 | `servers/nex/titles/monster-hunter-stories` — Monster Hunter Stories (3DS), Pretendo-derived, UDP | 22760 | 22761 | 22762 |
| 22770 | `servers/nex/titles/pokemon-rumble-world` — Pokémon Rumble World (3DS), Pretendo-derived, UDP | 22770 | 22771 | 22772 |
| 22780 | `servers/nex/titles/puyo-puyo-20th` — Puyo Puyo!! 20th Anniversary (3DS), Pretendo-derived, UDP | 22780 | 22781 | 22782 |
| 22790 | `servers/nex/titles/resident-evil-the-mercenaries-3d` — Resident Evil: The Mercenaries 3D (3DS), Pretendo-derived, UDP | 22790 | 22791 | 22792 |
| 22800 | `servers/nex/titles/team-kirby-clash-deluxe` — Team Kirby Clash Deluxe (3DS), Pretendo-derived, UDP | 22800 | 22801 | 22802 |
| 22810 | `servers/nex/titles/tetris-axis` — Tetris: Axis (3DS), Pretendo-derived, UDP | 22810 | 22811 | 22812 |
| 22820 | `servers/nex/titles/tri-force-heroes` — The Legend of Zelda: Tri Force Heroes (3DS), Pretendo-derived, UDP | 22820 | 22821 | 22822 |
| 22830 | `servers/nex/titles/yo-kai-watch-2` — Yo-kai Watch 2 (3DS), Pretendo-derived, UDP | 22830 | 22831 | 22832 |
| 22840 | `servers/nex/titles/yo-kai-watch-blasters` — Yo-kai Watch Blasters (3DS), Pretendo-derived, UDP | 22840 | 22841 | 22842 |
| 22850 | `servers/nex/titles/mario-kart-8` — Mario Kart 8 (Wii U) — OpenPak's Go port of Pretendo's Python server, UDP | 22850 | 22851 | 22852 |
| 22860 | `servers/nex/titles/mario-vs-donkey-kong-tipping-stars` — Mario vs. Donkey Kong: Tipping Stars (Wii U + 3DS, one server) — OpenPak's Go rewrite of Pretendo's v1 server, UDP | 22860 | 22861 | 22862 |
| 22870–22890 | NEX titles, one block of ten each, in order of creation | | | |

Other services' rows live in the canonical ports.md.
