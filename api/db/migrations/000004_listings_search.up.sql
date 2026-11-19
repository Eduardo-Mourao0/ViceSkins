-- Busca por trecho do nome (ILIKE '%redline%') sem varrer a tabela inteira.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX inventory_items_name_trgm_idx
    ON inventory_items USING gin (market_hash_name gin_trgm_ops);

-- Ordenações mais comuns do mercado, só sobre listings ativas.
CREATE INDEX listings_active_price_idx ON listings (price_cents, id) WHERE status = 'active';
