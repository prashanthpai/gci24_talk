BEGIN;

SELECT balance FROM user_balances WHERE user_id = ? FOR UPDATE;

-- application check: (balance - withdraw_amount) >= min_balance_required

UPDATE user_balances SET balance = balance - withdraw_amount WHERE item_id = ?;

COMMIT;
