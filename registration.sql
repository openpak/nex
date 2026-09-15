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

-- Wii U Chat (system applet), block 22700 (auth 22700/udp, secure 22701/udp)
-- The low-32 rule has only been observed on games; these three are the rule
-- extended to a system applet, marked unobserved. A console that asks and
-- misses behaves exactly as it does today (nn-account answers 1021).
INSERT INTO servers (game_server_id, access_mode, device, client_id, service_name, service_type, title_ids, ip, port, aes_key)
VALUES
  ('1005A000', 'prod', 1, 'wiiu-chat', 'wiiu-chat', 'NEX', ARRAY['000500101005A000'], :'host', 22700, :'aes_key'),
  ('1005A100', 'prod', 1, 'wiiu-chat', 'wiiu-chat', 'NEX', ARRAY['000500101005A100'], :'host', 22700, :'aes_key'),
  ('1005A200', 'prod', 1, 'wiiu-chat', 'wiiu-chat', 'NEX', ARRAY['000500101005A200'], :'host', 22700, :'aes_key')
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
