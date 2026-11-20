-- Dados de DEMONSTRAÇÃO para desenvolvimento. Gerado; não edite à mão.
--
-- Cria 3 usuários demo (steam_id "demo-*"), itens e anúncios, para testar o
-- marketplace sem conta Steam (use o login de demonstração, DEV_LOGIN=true).
-- Os itens usam ícones públicos da Steam; floats, patterns e stickers são fictícios.
--
-- Idempotente: rodar de novo não duplica nada.
-- Uso: docker compose run --rm seed

BEGIN;

-- Trade URLs fictícias: contas demo não existem na Steam (a validação de
-- dono é pulada para elas). Só preenche se ainda não houver uma.
INSERT INTO users (steam_id, display_name, avatar_url, trade_url, inventory_synced_at)
VALUES
    ('demo-1', 'Neon Trader', 'https://avatars.steamstatic.com/fef49e7fa7e1997310d705b2a6158ff8dc1cdfeb_full.jpg', 'https://steamcommunity.com/tradeoffer/new/?partner=1000001&token=demo0001', now()),
    ('demo-2', 'Sunset Collector', 'https://avatars.steamstatic.com/fef49e7fa7e1997310d705b2a6158ff8dc1cdfeb_full.jpg', 'https://steamcommunity.com/tradeoffer/new/?partner=1000002&token=demo0002', now()),
    ('demo-3', 'Ocean Buyer', 'https://avatars.steamstatic.com/fef49e7fa7e1997310d705b2a6158ff8dc1cdfeb_full.jpg', 'https://steamcommunity.com/tradeoffer/new/?partner=1000003&token=demo0003', now())
ON CONFLICT (steam_id) DO UPDATE
SET trade_url = COALESCE(users.trade_url, EXCLUDED.trade_url);

-- Saldo inicial pelo ledger (saldo = soma das movimentações), só na primeira vez.
WITH initial (steam_id, amount) AS (
    VALUES
        ('demo-1', 25000),
        ('demo-2', 12000),
        ('demo-3', 500000)
), fresh AS (
    SELECT users.id, initial.amount
    FROM initial
    JOIN users ON users.steam_id = initial.steam_id
    WHERE NOT EXISTS (SELECT 1 FROM wallet_entries w WHERE w.user_id = users.id)
), credited AS (
    UPDATE users
    SET balance_cents = users.balance_cents + fresh.amount
    FROM fresh
    WHERE users.id = fresh.id
    RETURNING users.id, users.balance_cents
)
INSERT INTO wallet_entries (user_id, amount_cents, balance_after_cents, kind, description)
SELECT fresh.id, fresh.amount, credited.balance_cents, 'deposit', 'Saldo inicial (demo)'
FROM fresh
JOIN credited ON credited.id = fresh.id;

INSERT INTO inventory_items (owner_id, asset_id, class_id, instance_id, market_hash_name, name, item_type,
                             weapon, rarity, rarity_color, exterior, float_value, paint_seed, icon_url,
                             stickers, tradable, marketable)
SELECT users.id, v.asset_id, '0', '0', v.mhn, v.name, v.item_type,
       v.weapon, v.rarity, v.rarity_color, v.exterior, v.float_value::float8, v.paint_seed::int, v.icon_url,
       v.stickers::jsonb, v.tradable, true
FROM (VALUES
    ('demo-1', 'demo-asset-1', 'G3SG1 | Red Jasper (Minimal Wear)', 'G3SG1 | Red Jasper', 'Consumer Grade Sniper Rifle', 'G3SG1', 'Consumer Grade', 'b0c3d9', 'Minimal Wear', 0.09580247, 264, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL2zYXnrB1I4P2Rb_d-J6GsGmidwPp5j-1gSCGn20Ql62_Tztz8dymWPwYoXpYjQ-8Ds0TpmoHnYbzh51aKjYhBzXn23CpXrnE8L3fMNjQ', '[{"kind":"charm","name":"Backsplash","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/weapon_1/kc_wpn_mac10_tile.8614222cda01d262103948671c9d51db779d7c40.png"},{"kind":"charm","name":"Lil'' Crass","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/missinglink/kc_missinglink_guerilla.a52d0ae27a3c87c094816ce28bd488a44ca710ef.png"},{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"},{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"}]', true),
    ('demo-1', 'demo-asset-2', 'Galil AR | Acid Dart (Field-Tested)', 'Galil AR | Acid Dart', 'Industrial Grade Rifle', 'Galil AR', 'Industrial Grade', '5e98d9', 'Field-Tested', 0.3443569, 753, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL2n5rp8SNJ0Pq3bZt0JfyfAXCv1P13tt5kSi26gBBptm3VmIuheHvDZwAoCpJ0TOVf4xawl9a2Nbi25AfcgoxMmSj63XlP6jErvbhldfbY2Q', '[]', true),
    ('demo-1', 'demo-asset-3', 'M4A1-S | Nitro (Field-Tested)', 'M4A1-S | Nitro', 'Restricted Rifle', 'M4A1-S', 'Restricted', '8847ff', 'Field-Tested', 0.34436032, 207, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8ypexwjFS4_ega6F_H-OcMWiCwOBxtd5oTCq2mwk0jDCAnobsLGWQPwMoW5VxTLEMsxK4moazP-vj5ALciIkWzCX72HhL5nts4b1QV6tz5OSJ2HJmNovH', '[]', true),
    ('demo-1', 'demo-asset-4', 'M4A4 | Aeolian Dark (Field-Tested)', 'M4A4 | Aeolian Dark', 'Industrial Grade Rifle', 'M4A4', 'Industrial Grade', '5e98d9', 'Field-Tested', 0.25021092, 569, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8ypexwipC6s2vard5MvGQGlibz-Fij-w9cCS2kRQyvnOEwt6hcSqSPA8oXJN1ROQPsUXpl4bnZu_rtgbd3t5DyX6tj3hLvyY4_a9cBl-cprie', '[]', true),
    ('demo-1', 'demo-asset-5', 'MP9 | Orange Peel (Field-Tested)', 'MP9 | Orange Peel', 'Industrial Grade SMG', 'MP9', 'Industrial Grade', '5e98d9', 'Field-Tested', 0.34340466, 203, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8js_f7jJk-_O-bZtiMvGdCWKvzOtyufRkASrjlEom42mAn9ehcS6fbw8oA8YlR7IJ4EPrkNziNOjg51Hfj4oQmSjgznQe7YSb6Mg', '[]', true),
    ('demo-1', 'demo-asset-6', 'MP9 | Pine (Minimal Wear)', 'MP9 | Pine', 'Consumer Grade SMG', 'MP9', 'Consumer Grade', 'b0c3d9', 'Minimal Wear', 0.08921837, 917, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8js_f7i1e0POieK1jJc-UHGKVz9F6ueZhW2fixUtwtm6Hy9urd33CbQUhDsFzF7RetxS_ldzgY7ywsVbdidkWyi6tkGoXuWJp7z88', '[]', false),
    ('demo-1', 'demo-asset-7', 'Nova | Predator (Field-Tested)', 'Nova | Predator', 'Consumer Grade Shotgun', 'Nova', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.2148361, 864, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL_kYDhwjFL0OirarZsI_GeMWqVxedjva9rHC_nlxx24jnXmY2pJ3qVZw51C5V4EeQJthbtx9K2MruztgOMj4NB02yg2XPmt0BC', '[]', false),
    ('demo-1', 'demo-asset-8', 'PP-Bizon | Thermal Currents (Field-Tested)', 'PP-Bizon | Thermal Currents', 'Consumer Grade SMG', 'PP-Bizon', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.17388169, 448, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLzl4zv8x1T9veRb6FiIvmBCnSv0vt4ouh6Sha-kBkupjDLn9v8IiiUb1IiW5V2QORbtxe5xtHvMOzhtQXa2NkTziv83ChJ7H5j4_FCD_TbmGXaLw', '[{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"}]', true),
    ('demo-1', 'demo-asset-9', 'SSG 08 | Calligrafaux (Minimal Wear)', 'SSG 08 | Calligrafaux', 'Industrial Grade Sniper Rifle', 'SSG 08', 'Industrial Grade', '5e98d9', 'Minimal Wear', 0.11130019, 674, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLijZGwpR1T-828baBSJfSUC3Svx-d4te1gQSyMmRQguynLwov6JHmRagd0WcBxQuYItxWxlYLgM--z4gDXjthGnnmqjXxAvSpqtfFCD_S8UR_TMg', '[]', true),
    ('demo-1', 'demo-asset-10', 'SSG 08 | Sans Comic (Field-Tested)', 'SSG 08 | Sans Comic', 'Consumer Grade Sniper Rifle', 'SSG 08', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.16134881, 897, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLijZGwpR1T9veRa6tgKfOsHW-R0etlj-xsSyCmmFN2tWqAmdmgdyqVawQgWJt2RrEOthS4ldK0Zuq34FTX2I0Tz3j32H5J8G81tCSL3uoY', '[]', true),
    ('demo-1', 'demo-asset-11', 'StatTrak™ Glock-18 | Weasel (Well-Worn)', 'StatTrak™ Glock-18 | Weasel', 'StatTrak™ Restricted Pistol', 'Glock-18', 'Restricted', '8847ff', 'Well-Worn', 0.43111808, 324, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL2kpnj9h1Y-s2pZKtuK6HLMXCVwP1zvN5kSi26gBBp4m2GyY7_cH3FOFAlW5BzELYDuxK_lNCyNrvk5FPY2NgWny36jysYvTErvbhCx69uPA', '[{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"},{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"}]', true),
    ('demo-1', 'demo-asset-12', 'USP-S | Tropical Breeze (Field-Tested)', 'USP-S | Tropical Breeze', 'Mil-Spec Grade Pistol', 'USP-S', 'Mil-Spec Grade', '4b69ff', 'Field-Tested', 0.27696753, 943, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLkjYbf7itX6vytbbZSM_-WMXOCzv5_s-BlcCS2kRQyvnOEy4v6dnzDZ1IgWJFyFu4K5hLqktLuN-jrtASK3o1HxCr_iClO530-_a9cBiau7NtK', '[]', true),
    ('demo-1', 'demo-asset-13', 'XM1014 | Canvas Cloud (Field-Tested)', 'XM1014 | Canvas Cloud', 'Consumer Grade Shotgun', 'XM1014', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.18132013, 902, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLpk8ewrHZk5-uRa6hiNfSsDWadztF7teVgWiT9xxt25GTcwt-pc3uVbw8gC8chQ-8LtRTuwIGzN-634ATY2t4XxHr4jzQJsHhmzMwM-w', '[]', true),
    ('demo-1', 'demo-asset-14', 'Zeus x27 | Earth Mandala (Well-Worn)', 'Zeus x27 | Earth Mandala', 'Mil-Spec Grade Equipment', 'Zeus x27', 'Mil-Spec Grade', '4b69ff', 'Well-Worn', 0.43616149, 710, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLln4Xl7x1T9veRbaV_NPisA2aexe96sd5lTBa-kBkupjDLwtf_cXyUb1IhCcYiE7Nf50K8l9HjZO2x4wbWi4tEyCyr2H9N5i9r4fFCD_QtFzKy7g', '[]', true),
    ('demo-2', 'demo-asset-15', 'AK-47 | Wintergreen (Field-Tested)', 'AK-47 | Wintergreen', 'Mil-Spec Grade Rifle', 'AK-47', 'Mil-Spec Grade', '4b69ff', 'Field-Tested', 0.22456182, 339, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLwlcK3wjFU6s2jbbBsLPyaDViX0-tzvt47cCS2kRQyvnPSydeqcyqVPQUoC8YkRe4PtkK_m4fiY-yw5lCMgt0Xznj_334avHtp_a9cBvXwlabr', '[]', true),
    ('demo-2', 'demo-asset-16', 'G3SG1 | Red Jasper (Minimal Wear)', 'G3SG1 | Red Jasper', 'Consumer Grade Sniper Rifle', 'G3SG1', 'Consumer Grade', 'b0c3d9', 'Minimal Wear', 0.07459013, 532, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL2zYXnrB1I4P2Rb_d-J6GsGmidwPp5j-1gSCGn20Ql62_Tztz8dymWPwYoXpYjQ-8Ds0TpmoHnYbzh51aKjYhBzXn23CpXrnE8L3fMNjQ', '[]', true),
    ('demo-2', 'demo-asset-17', 'M4A4 | Naval Shred Camo (Field-Tested)', 'M4A4 | Naval Shred Camo', 'Industrial Grade Rifle', 'M4A4', 'Industrial Grade', '5e98d9', 'Field-Tested', 0.32634091, 321, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8ypexwipC6s2sZLFoH_3HDzOvzOtyufRkAXjik0omtW7Vno3_eH2SZwIjCJV1RbYC4RW_ktHiZb-w51bbgopEz37gznQeJA60ihA', '[{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"}]', true),
    ('demo-2', 'demo-asset-18', 'MP9 | Bee-Tron (Factory New)', 'MP9 | Bee-Tron', 'Consumer Grade SMG', 'MP9', 'Consumer Grade', 'b0c3d9', 'Factory New', 0.06041638, 360, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8js_f7i1e0OurZKhiN8-RB3OD_uJ_t-l9ASrhzRt062_Qm4n8cn-XbFQkCcMkTLQD5EW8m9TjM--34gXf2Y1Hzn_gznQepx3qArs', '[{"kind":"charm","name":"Backsplash","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/weapon_1/kc_wpn_mac10_tile.8614222cda01d262103948671c9d51db779d7c40.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"}]', true),
    ('demo-2', 'demo-asset-19', 'MP9 | Orange Peel (Field-Tested)', 'MP9 | Orange Peel', 'Industrial Grade SMG', 'MP9', 'Industrial Grade', '5e98d9', 'Field-Tested', 0.34990986, 876, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8js_f7jJk-_O-bZtiMvGdCWKvzOtyufRkASrjlEom42mAn9ehcS6fbw8oA8YlR7IJ4EPrkNziNOjg51Hfj4oQmSjgznQe7YSb6Mg', '[]', true),
    ('demo-2', 'demo-asset-20', 'MP9 | Pine (Minimal Wear)', 'MP9 | Pine', 'Consumer Grade SMG', 'MP9', 'Consumer Grade', 'b0c3d9', 'Minimal Wear', 0.14081645, 636, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL8js_f7i1e0POieK1jJc-UHGKVz9F6ueZhW2fixUtwtm6Hy9urd33CbQUhDsFzF7RetxS_ldzgY7ywsVbdidkWyi6tkGoXuWJp7z88', '[{"kind":"charm","name":"Lil'' Crass","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/missinglink/kc_missinglink_guerilla.a52d0ae27a3c87c094816ce28bd488a44ca710ef.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"},{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"}]', true),
    ('demo-2', 'demo-asset-21', 'PP-Bizon | Thermal Currents (Field-Tested)', 'PP-Bizon | Thermal Currents', 'Consumer Grade SMG', 'PP-Bizon', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.27744615, 92, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLzl4zv8x1T9veRb6FiIvmBCnSv0vt4ouh6Sha-kBkupjDLn9v8IiiUb1IiW5V2QORbtxe5xtHvMOzhtQXa2NkTziv83ChJ7H5j4_FCD_TbmGXaLw', '[{"kind":"charm","name":"Lil'' Crass","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/missinglink/kc_missinglink_guerilla.a52d0ae27a3c87c094816ce28bd488a44ca710ef.png"},{"kind":"charm","name":"Backsplash","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/weapon_1/kc_wpn_mac10_tile.8614222cda01d262103948671c9d51db779d7c40.png"},{"kind":"sticker","name":"Falcons | Budapest 2025","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/bud2025/fal.8803b05e0adc81c5e687a3df6c70e13892d01d0c.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"}]', true),
    ('demo-2', 'demo-asset-22', 'Revolution Case', 'Revolution Case', 'Base Grade Container', '', 'Base Grade', 'b0c3d9', '', NULL, NULL, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGJKz2lu_XsnXwtmkJjSU91dh8bj35VTqVBP4io_frnAVvfb6aqduc_TFVjTCxbx05OU4S3jilE9w4DzRnImtIy2Sa1JzDJEhRPlK7EcO4U8gfA', '[]', true),
    ('demo-2', 'demo-asset-23', 'SSG 08 | Calligrafaux (Minimal Wear)', 'SSG 08 | Calligrafaux', 'Industrial Grade Sniper Rifle', 'SSG 08', 'Industrial Grade', '5e98d9', 'Minimal Wear', 0.09011731, 833, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLijZGwpR1T-828baBSJfSUC3Svx-d4te1gQSyMmRQguynLwov6JHmRagd0WcBxQuYItxWxlYLgM--z4gDXjthGnnmqjXxAvSpqtfFCD_S8UR_TMg', '[{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"},{"kind":"charm","name":"Lil'' Crass","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/keychains/missinglink/kc_missinglink_guerilla.a52d0ae27a3c87c094816ce28bd488a44ca710ef.png"},{"kind":"sticker","name":"Apeks (Glitter) | Copenhagen 2024","imageUrl":"https://cdn.steamstatic.com/apps/730/icons/econ/stickers/cph2024/apex_glitter.77a47f49b9d207e38474c90d72006af2f1cd3b4e.png"}]', true),
    ('demo-2', 'demo-asset-24', 'SSG 08 | Sans Comic (Field-Tested)', 'SSG 08 | Sans Comic', 'Consumer Grade Sniper Rifle', 'SSG 08', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.19255836, 210, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLijZGwpR1T9veRa6tgKfOsHW-R0etlj-xsSyCmmFN2tWqAmdmgdyqVawQgWJt2RrEOthS4ldK0Zuq34FTX2I0Tz3j32H5J8G81tCSL3uoY', '[]', true),
    ('demo-2', 'demo-asset-25', 'Sealed Graffiti | Heart (Desert Amber)', 'Sealed Graffiti | Heart (Desert Amber)', 'Base Grade Graffiti', '', 'Base Grade', 'b0c3d9', '', NULL, NULL, 'https://community.fastly.steamstatic.com/economy/image/IzMF03bi9WpSBq-S-ekoE33L-iLqGFHVaU25ZzQNQcXdB2ozio1RrlIWFK3UfvMYB8UsvjiMXojflsZalyxSh31CIyHz2GZ-KuFpPsrTzBG0qeeMDmC5bDbHf3aNHQk6TbMLZzrcqmH04ruQQz7PQe4tRQBXL6VV9TFPNJiAPwx9itAdqGq0mFZwCxo8e9VKaVK6zyxAYrmVtm8oIQ', '[]', true),
    ('demo-2', 'demo-asset-26', 'Sealed Graffiti | Keep the Change (Frog Green)', 'Sealed Graffiti | Keep the Change (Frog Green)', 'Base Grade Graffiti', '', 'Base Grade', 'b0c3d9', '', NULL, NULL, 'https://community.fastly.steamstatic.com/economy/image/IzMF03bi9WpSBq-S-ekoE33L-iLqGFHVaU25ZzQNQcXdB2ozio1RrlIWFK3UfvMYB8UsvjiMXojflsZalyxSh31CIyHz2GZ-KuFpPsrTzBG0pe2BEHXlJmDHdnWAGQ88ReUMYTnZ92at5OiSF2zKE-ApEgpVL_QE9DBOOZuJOEAjlNlc7Wa3m0tvEwMkZsxWfBbmnixLN-V1bHZsw8c', '[]', true),
    ('demo-2', 'demo-asset-27', 'Sealed Graffiti | Ninja (Bazooka Pink)', 'Sealed Graffiti | Ninja (Bazooka Pink)', 'Base Grade Graffiti', '', 'Base Grade', 'b0c3d9', '', NULL, NULL, 'https://community.fastly.steamstatic.com/economy/image/IzMF03bi9WpSBq-S-ekoE33L-iLqGFHVaU25ZzQNQcXdB2ozio1RrlIWFK3UfvMYB8UsvjiMXojflsZalyxSh31CIyHz2GZ-KuFpPsrTzBG0pO-CI3r-Zj3FYSfdTgw8RLRYM2qIqmalsO6cFj6fSb55RA0HKPcA92EdaJyNNkBp14AVu2u_0UdyEhk6f9BKZAarxm1OM7xzkCNAKzBCcak', '[]', true),
    ('demo-2', 'demo-asset-28', 'Sealed Graffiti | Take Flight (Tiger Orange)', 'Sealed Graffiti | Take Flight (Tiger Orange)', 'Base Grade Graffiti', '', 'Base Grade', 'b0c3d9', '', NULL, NULL, 'https://community.fastly.steamstatic.com/economy/image/IzMF03bi9WpSBq-S-ekoE33L-iLqGFHVaU25ZzQNQcXdB2ozio1RrlIWFK3UfvMYB8UsvjiMXojflsZalyxSh31CIyHz2GZ-KuFpPsrTzBG0tuuDG2e5OjaTLHHcGF0_SrNbZjzQ_2KmsL7GEG3LEO8oQwtSe6YC-mFPP8iINwx9itAdqGq0mFZwCxo8e9VKaVK5kiNCZeVy7ED-9A', '[]', true),
    ('demo-2', 'demo-asset-29', 'StatTrak™ Glock-18 | Weasel (Well-Worn)', 'StatTrak™ Glock-18 | Weasel', 'StatTrak™ Restricted Pistol', 'Glock-18', 'Restricted', '8847ff', 'Well-Worn', 0.39259641, 682, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyL2kpnj9h1Y-s2pZKtuK6HLMXCVwP1zvN5kSi26gBBp4m2GyY7_cH3FOFAlW5BzELYDuxK_lNCyNrvk5FPY2NgWny36jysYvTErvbhCx69uPA', '[]', true),
    ('demo-2', 'demo-asset-30', 'USP-S | Tropical Breeze (Field-Tested)', 'USP-S | Tropical Breeze', 'Mil-Spec Grade Pistol', 'USP-S', 'Mil-Spec Grade', '4b69ff', 'Field-Tested', 0.33563995, 847, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLkjYbf7itX6vytbbZSM_-WMXOCzv5_s-BlcCS2kRQyvnOEy4v6dnzDZ1IgWJFyFu4K5hLqktLuN-jrtASK3o1HxCr_iClO530-_a9cBiau7NtK', '[]', true),
    ('demo-2', 'demo-asset-31', 'XM1014 | Canvas Cloud (Field-Tested)', 'XM1014 | Canvas Cloud', 'Consumer Grade Shotgun', 'XM1014', 'Consumer Grade', 'b0c3d9', 'Field-Tested', 0.30612018, 79, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLpk8ewrHZk5-uRa6hiNfSsDWadztF7teVgWiT9xxt25GTcwt-pc3uVbw8gC8chQ-8LtRTuwIGzN-634ATY2t4XxHr4jzQJsHhmzMwM-w', '[]', true),
    ('demo-2', 'demo-asset-32', 'Zeus x27 | Earth Mandala (Well-Worn)', 'Zeus x27 | Earth Mandala', 'Mil-Spec Grade Equipment', 'Zeus x27', 'Mil-Spec Grade', '4b69ff', 'Well-Worn', 0.43795242, 69, 'https://community.fastly.steamstatic.com/economy/image/i0CoZ81Ui0m-9KwlBY1L_18myuGuq1wfhWSaZgMttyVfPaERSR0Wqmu7LAocGIGz3UqlXOLrxM-vMGmW8VNxu5Dx60noTyLln4Xl7x1T9veRbaV_NPisA2aexe96sd5lTBa-kBkupjDLwtf_cXyUb1IhCcYiE7Nf50K8l9HjZO2x4wbWi4tEyCyr2H9N5i9r4fFCD_QtFzKy7g', '[]', true)
) AS v (owner, asset_id, mhn, name, item_type, weapon, rarity, rarity_color, exterior, float_value, paint_seed, icon_url, stickers, tradable)
JOIN users ON users.steam_id = v.owner
ON CONFLICT (asset_id) DO NOTHING;

-- Anúncios só para itens que nunca foram anunciados (não recria depois de vendidos/cancelados).
INSERT INTO listings (item_id, seller_id, price_cents, created_at)
SELECT inventory_items.id, inventory_items.owner_id, v.price, now() - make_interval(mins => v.age_min)
FROM (VALUES
    ('demo-asset-2', 327, 1643),
    ('demo-asset-3', 6766, 774),
    ('demo-asset-4', 284, 4183),
    ('demo-asset-5', 172, 3639),
    ('demo-asset-8', 209, 735),
    ('demo-asset-9', 154, 260),
    ('demo-asset-10', 96, 4043),
    ('demo-asset-11', 17806, 3678),
    ('demo-asset-12', 728, 3475),
    ('demo-asset-13', 145, 4095),
    ('demo-asset-15', 1140, 3322),
    ('demo-asset-16', 48, 1875),
    ('demo-asset-17', 271, 4226),
    ('demo-asset-19', 327, 1008),
    ('demo-asset-20', 1649, 851),
    ('demo-asset-22', 465, 2022),
    ('demo-asset-23', 500, 3571),
    ('demo-asset-24', 88, 4252),
    ('demo-asset-26', 554, 2143),
    ('demo-asset-27', 250, 384),
    ('demo-asset-28', 70, 282),
    ('demo-asset-29', 8704, 2575),
    ('demo-asset-30', 2116, 3971),
    ('demo-asset-31', 81, 3391),
    ('demo-asset-32', 1322, 3399)
) AS v (asset_id, price, age_min)
JOIN inventory_items ON inventory_items.asset_id = v.asset_id
WHERE NOT EXISTS (SELECT 1 FROM listings WHERE listings.item_id = inventory_items.id);

COMMIT;
