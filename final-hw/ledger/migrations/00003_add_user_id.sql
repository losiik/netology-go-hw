-- +goose Up
ALTER TABLE budgets ADD COLUMN user_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE budgets DROP CONSTRAINT budgets_category_key;
ALTER TABLE budgets ADD CONSTRAINT budgets_user_category_key UNIQUE (user_id, category);

ALTER TABLE expenses ADD COLUMN user_id BIGINT NOT NULL DEFAULT 0;
DROP INDEX idx_expenses_category_date;
CREATE INDEX idx_expenses_user_category_date ON expenses (user_id, category, date);

-- +goose Down
DROP INDEX idx_expenses_user_category_date;
CREATE INDEX idx_expenses_category_date ON expenses (category, date);
ALTER TABLE expenses DROP COLUMN user_id;

ALTER TABLE budgets DROP CONSTRAINT budgets_user_category_key;
ALTER TABLE budgets DROP COLUMN user_id;
ALTER TABLE budgets ADD CONSTRAINT budgets_category_key UNIQUE (category);
