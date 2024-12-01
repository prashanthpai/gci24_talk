package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"pglock"
	_ "pglock"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db, err := sql.Open("postgres", "postgresql://ppai:ppai@localhost:5432/ppai?sslmode=disable")
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	if err := doWithLock(ctx, db); err != nil {
		log.Fatalf("do with lock: %v", err)
	}
}

// START OMIT

func doWithLock(ctx context.Context, db *sql.DB) (err error) {
	lock := pglock.New(db, "lock_key")

	if err := lock.Lock(ctx); err != nil {
		return err
	}
	log.Printf("acquired lock")

	defer func() {
		if lerr := lock.Unlock(); lerr != nil {
			err = lerr
		}
		log.Printf("released lock")
	}()

	do(ctx)

	return nil
}

// END OMIT

func do(ctx context.Context) {
	log.Printf("doing stuff")
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
}
