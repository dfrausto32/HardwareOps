package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/parcel/control-plane/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dir := flag.String("dir", "./migrations", "migrations directory")
	dbURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "database connection URL")
	flag.Parse()

	if *dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), *dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := migrate.Apply(context.Background(), pool, *dir); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	log.Printf("migrations applied from %s", *dir)
}
