package pglock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// CREATE TABLE pglocks (key TEXT PRIMARY KEY);

var (
	ErrAlreadyLocked = errors.New("already locked")
)

type Lock struct {
	db           *sql.DB
	tx           *sql.Tx
	insertSQL    string
	lockSQL      string
	checkLockSQL string
	key          string
}

func New(db *sql.DB, key string) *Lock {
	return &Lock{
		db:           db,
		insertSQL:    "INSERT INTO pglocks (key) VALUES ($1) ON CONFLICT DO NOTHING",
		lockSQL:      "SELECT 1 FROM pglocks WHERE key = $1 FOR UPDATE SKIP LOCKED", // non-blocking
		checkLockSQL: "SELECT 1 FROM pglocks WHERE key = $1 FOR UPDATE NOWAIT",
		key:          key,
	}
}

func (l *Lock) createLockRecord(ctx context.Context) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Make sure we don't wait forever in case another instance has locked the row.
	if _, err := tx.ExecContext(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}

	if _, err := tx.ExecContext(ctx, l.insertSQL, []any{l.key}...); err != nil {
		if isLockTimeout(err) {
			// This means the lock record probably already exists,
			// so we don't need to do anything else.
			// Note: tx.Rollback will be called via defer.
			return nil
		}

		return fmt.Errorf("insert lock record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (l *Lock) Lock(ctx context.Context) error {
	if l.tx != nil {
		return fmt.Errorf("lock tx already active")
	}

	if err := l.createLockRecord(ctx); err != nil {
		return fmt.Errorf("create lock record: %w", err)
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start tx: %w", err)
	}

	if err := tx.QueryRowContext(ctx, l.lockSQL, []any{l.key}...).Scan(new(int)); err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAlreadyLocked
		}

		return fmt.Errorf("lock row: %w", err)
	}
	l.tx = tx

	return nil
}

func (l *Lock) IsLocked(ctx context.Context) (bool, error) {
	// Note: don't use a read replica for this
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx")
	}
	defer tx.Rollback()

	if err = tx.QueryRowContext(ctx, l.checkLockSQL, []any{l.key}...).Scan(new(int)); err != nil {
		if isLockTimeout(err) {
			// Another instance has locked the row
			return true, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check lock: %w", err)
	}

	return false, nil
}

func (l *Lock) Unlock() error {
	if l.tx == nil {
		return fmt.Errorf("no lock tx")
	}

	return l.tx.Rollback()
}

func isLockTimeout(err error) bool {
	pqErr := &pq.Error{}
	if errors.As(err, &pqErr) {
		return pqErr.Code == "55P03"
	}

	return false
}
