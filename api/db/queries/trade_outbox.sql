-- Todos os horários do outbox vêm do relógio da API (como hold_until), nunca
-- do now() do banco: os dois relógios podem divergir.
-- name: EnqueueTradeOffer :exec
INSERT INTO trade_outbox (transaction_id, next_attempt_at, created_at)
VALUES (@transaction_id, @now::timestamptz, @now::timestamptz);

-- Pega até max_rows linhas vencidas e sem lease válido, e reserva cada uma
-- até lease_until. SKIP LOCKED: dois workers nunca pegam a mesma linha, e
-- nenhum fica esperando o outro. O lock dura só este UPDATE; a chamada à
-- Steam acontece depois, protegida pelo lease, não por lock do banco.
-- transaction_id opcional: a compra tenta despachar a própria linha na hora.
-- name: ClaimTradeOutbox :many
UPDATE trade_outbox
SET attempts     = attempts + 1,
    locked_until = @lease_until::timestamptz
WHERE transaction_id IN (
    SELECT transaction_id FROM trade_outbox
    WHERE done_at IS NULL
      AND next_attempt_at <= @now::timestamptz
      AND (locked_until IS NULL OR locked_until <= @now::timestamptz)
      AND (sqlc.narg(transaction_id)::bigint IS NULL OR transaction_id = sqlc.narg(transaction_id)::bigint)
    ORDER BY next_attempt_at
    LIMIT @max_rows
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: RescheduleTradeOutbox :exec
UPDATE trade_outbox
SET next_attempt_at = @next_attempt_at::timestamptz,
    last_error      = @last_error,
    locked_until    = NULL
WHERE transaction_id = @transaction_id AND done_at IS NULL;

-- name: CloseTradeOutbox :exec
UPDATE trade_outbox
SET done_at = @now::timestamptz, locked_until = NULL
WHERE transaction_id = @transaction_id AND done_at IS NULL;
