-- registration.sql — nn-account `servers` rows for the NEX title containers
-- (platform-wiiu-prd WU-5). Idempotent: safe to re-run, safe to apply over a
-- row that already exists (it is updated, not duplicated).
--
-- Apply on the production box as someone who can read the secrets:
--
--   psql "postgresql:///nn_account" \
--     -v host=<public IP or DNS name of the shared NEX host> \
--     -v aes_key=$(openssl rand -hex 16) \
--     -f registration.sql
--
-- `host` is what consoles should reach the title containers on; every row
-- here points at one box, per-title port block from ports.md (auth = base+0,
-- secure = base+1). `aes_key` signs independent service tokens for the row;
-- it is per-deployment, never committed here.
--
-- Every block below must pass the shared host's startup validator
-- (`host.go validate`: a multiple of ten in 22100-22199 or 22300-22599),
-- and the deployed container must run with the same NEX_PORT_BASE, or the
-- row points at a port nothing binds. `registration_test.go` checks the
-- ranges, the low-32 derivation, and duplicate ids/ports; run it before
-- every deploy pass that touches this file.
--
-- Two facts the file relies on, both observed not assumed:
--
--   * A Wii U asks nn-account by game server id, and Nintendo's Wii U game
--     server id is the low 32 bits of the title id — it holds for all 41
--     entries of Kinnay's Wii U list (kinnay.github.io, data/nexwiiu.json,
--     checked 2026-09-12). The rows in section A derive their id that way
--     from public title ids (kinnay.github.io data/wiiu.json; Wii U Chat's
--     system-applet ids from wiiubrew.org Title database).
--   * A 3DS sends its game server id in the NASC request and nn-account
--     rejects a mismatch ("152"), so a 3DS row cannot be registered before
--     the id has been observed from a console or a public source. Section B
--     holds the prepared rows; fill in gsid and uncomment.
--
-- Row-per-region: the same game asks with a different game server id per
-- region, and the table is keyed by (game_server_id, access_mode), so each
-- region is its own row pointing at the same container.

\set ON_ERROR_STOP on

-- ============================================================
-- Section A0 — friends server (apply now)
-- ============================================================
-- nn-friends serves Wii U and 3DS friend sessions from one NEX listener pair.
-- Consoles ask nn-account for game server 00003200; the lookup is by
-- (game_server_id, access_mode) only, so one row serves both consoles and
-- `device`/`title_ids` are informational here (friends sessions are
-- per-console, not per-title; the token path never consults title_ids).
-- Port 22360/22361 per the platform PRD (nn-friends NEX, UDP); the
-- friends container must listen there in prod (its example.env 60000/60001
-- are dev defaults). `host` must be the friends container's public host —
-- same box as the title containers unless the deploy splits them.
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('00003200', 'prod', 1, 'friends', 'friends', 'NEX', '{}', :'host', 22360, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- ============================================================
-- Section A — Wii U, game server id derived (apply now)
-- ============================================================

-- Super Mario 3D World, block 22570 (auth 22570/udp, secure 22571/udp)
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('10106100', 'prod', 1, 'super-mario-3d-world', 'super-mario-3d-world', 'NEX', ARRAY['0005000010106100'], :'host', 22570, :'aes_key'),
  ('10145C00', 'prod', 1, 'super-mario-3d-world', 'super-mario-3d-world', 'NEX', ARRAY['0005000010145C00'], :'host', 22570, :'aes_key'),
  ('10145D00', 'prod', 1, 'super-mario-3d-world', 'super-mario-3d-world', 'NEX', ARRAY['0005000010145D00'], :'host', 22570, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- Dr. Luigi, block 22520 (auth 22520/udp, secure 22521/udp)
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('10150300', 'prod', 1, 'dr-luigi', 'dr-luigi', 'NEX', ARRAY['0005000010150300'], :'host', 22520, :'aes_key'),
  ('10152A00', 'prod', 1, 'dr-luigi', 'dr-luigi', 'NEX', ARRAY['0005000010152A00'], :'host', 22520, :'aes_key'),
  ('10152B00', 'prod', 1, 'dr-luigi', 'dr-luigi', 'NEX', ARRAY['0005000010152B00'], :'host', 22520, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- Wii U Chat (system applet), block 22390 (auth 22390/udp, secure 22391/udp).
-- Moved from 22700 on 2026-10-27: 22700 fails the shared host's startup
-- validator (host.go: 22100-22199 or 22300-22599), so no container could bind
-- it; 22360 was considered but belongs to nn-friends (PRD). The deployed
-- wiiu-chat container must run with NEX_PORT_BASE=22390.
-- The low-32 rule has only been observed on games; these three are the rule
-- extended to a system applet, marked unobserved. A console that asks and
-- misses behaves exactly as it does today (nn-account answers 1021).
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('1005A000', 'prod', 1, 'wiiu-chat', 'wiiu-chat', 'NEX', ARRAY['000500101005A000'], :'host', 22390, :'aes_key'),
  ('1005A100', 'prod', 1, 'wiiu-chat', 'wiiu-chat', 'NEX', ARRAY['000500101005A100'], :'host', 22390, :'aes_key'),
  ('1005A200', 'prod', 1, 'wiiu-chat', 'wiiu-chat', 'NEX', ARRAY['000500101005A200'], :'host', 22390, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- Mario Kart 8, block 22410 (auth 22410/udp, secure 22411/udp)
-- Title ids (kinnay.github.io data/wiiu.json, checked 2026-10-27):
-- JPN 000500001010EB00 (WUP-P-AMKJ), USA 000500001010EC00 (WUP-P-AMKE),
-- EUR 000500001010ED00 (WUP-P-AMKP). Game-server ids are the low 32 bits;
-- JPN 1010EB00 is confirmed in data/nexwiiu.json. Deployed container must run
-- with NEX_PORT_BASE=22410.
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('1010EB00', 'prod', 1, 'mario-kart-8', 'mario-kart-8', 'NEX', ARRAY['000500001010EB00'], :'host', 22410, :'aes_key'),
  ('1010EC00', 'prod', 1, 'mario-kart-8', 'mario-kart-8', 'NEX', ARRAY['000500001010EC00'], :'host', 22410, :'aes_key'),
  ('1010ED00', 'prod', 1, 'mario-kart-8', 'mario-kart-8', 'NEX', ARRAY['000500001010ED00'], :'host', 22410, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- Mario vs. Donkey Kong: Tipping Stars (Wii U regions), block 22420
-- (auth 22420/udp, secure 22421/udp)
-- Title ids (kinnay.github.io data/wiiu.json, checked 2026-10-27):
-- USA 0005000010149300 (WUP-P-WAFE), EUR 0005000010178E00 (WUP-P-WAFP),
-- JPN 0005000010179200 (WUP-P-WAFJ). Game-server ids are the low 32 bits.
-- The title's 3DS regions (000400000012C800/CA00, nex-viewer) stay unregistered:
-- 3DS ids must be observed from a console (NASC rejects mismatches, 152).
-- Deployed container must run with NEX_PORT_BASE=22420.
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('10149300', 'prod', 1, 'mario-vs-donkey-kong-tipping-stars', 'mario-vs-donkey-kong-tipping-stars', 'NEX', ARRAY['0005000010149300'], :'host', 22420, :'aes_key'),
  ('10178E00', 'prod', 1, 'mario-vs-donkey-kong-tipping-stars', 'mario-vs-donkey-kong-tipping-stars', 'NEX', ARRAY['0005000010178E00'], :'host', 22420, :'aes_key'),
  ('10179200', 'prod', 1, 'mario-vs-donkey-kong-tipping-stars', 'mario-vs-donkey-kong-tipping-stars', 'NEX', ARRAY['0005000010179200'], :'host', 22420, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- Minecraft: Wii U Edition, block 22430 (auth 22430/udp, secure 22431/udp)
-- Title ids (kinnay.github.io data/wiiu.json, checked 2026-10-27):
-- USA 00050000101D9D00 (WUP-P-AUME), EUR 00050000101D7500 (WUP-P-AUMP),
-- JPN 00050000101DBE00 (WUP-P-AUMJ). Not Story Mode (separate title ids).
-- Game-server ids are the low 32 bits; USA 101D9D00 is confirmed in
-- data/nexwiiu.json. Deployed container must run with NEX_PORT_BASE=22430.
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('101D9D00', 'prod', 1, 'minecraft-wiiu', 'minecraft-wiiu', 'NEX', ARRAY['00050000101D9D00'], :'host', 22430, :'aes_key'),
  ('101D7500', 'prod', 1, 'minecraft-wiiu', 'minecraft-wiiu', 'NEX', ARRAY['00050000101D7500'], :'host', 22430, :'aes_key'),
  ('101DBE00', 'prod', 1, 'minecraft-wiiu', 'minecraft-wiiu', 'NEX', ARRAY['00050000101DBE00'], :'host', 22430, :'aes_key')
ON CONFLICT (game_server_id, access_mode) DO UPDATE
  SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;

-- ============================================================
-- Section B — 3DS, game server id unobserved (do not apply yet)
-- ============================================================
-- Title ids are public (3dsdb: github.com/hax0kartik/3dsdb, jsons/list_*.json,
-- checked 2026-09-12); the game server id each console sends is not. When one
-- is observed (a NASC debug log naming the requested id is enough — nn-account
-- logs it as a 152), copy the block, replace GSID with the observed id, and
-- move it above this line.
--
-- Sources per title:
--   F1 2011                        EUR 000400000006A600, JPN 0004000000067B00
--   Dr. Mario: Miracle Cure        USA+EUR 000400000013BB00
--   Monster Hunter 4 Ultimate      USA 0004000000126300, EUR 0004000000126100, JPN(4G) 000400000011D700
--   Monster Hunter Generations     USA 0004000000187000, EUR 0004000000185B00, JPN(X) 0004000000155400
--   Monster Hunter XX              JPN 0004000000197100
--   Super Street Fighter IV 3D     USA 0004000000032D00, EUR 0004000000033C00, JPN 0004000000030500
--   Monster Hunter 4 (Japan)       JPN 000400000004B500
--
-- Example shape (F1 2011, block 22400, device 2 = CTR):
-- INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
-- VALUES ('GSID', 'prod', 2, 'f1-2011', 'f1-2011', 'NEX',
--         ARRAY['000400000006A600','0004000000067B00'], :'host', 22400, :'aes_key')
-- ON CONFLICT (game_server_id, access_mode) DO UPDATE
--   SET title_ids = EXCLUDED.title_ids, ip = EXCLUDED.ip, port = EXCLUDED.port;
