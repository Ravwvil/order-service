package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

const (
	defaultMigrationsPath = "file://migrations"
	defaultRetryAttempts  = 10
	defaultRetryTimeout   = 5 * time.Second
)

func main() {
	log.Println("Starting migrator...")

	dbHost := os.Getenv("POSTGRES_HOST")
	if dbHost == "" {
		dbHost = "postgres"
	}
	dbPort := os.Getenv("POSTGRES_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("POSTGRES_USER")
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DATABASE")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbPort, dbName)

	var m *migrate.Migrate
	var err error

	for i := 0; i < defaultRetryAttempts; i++ {
		m, err = migrate.New(defaultMigrationsPath, dsn)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to db, retrying in %s... (%d/%d)", defaultRetryTimeout, i+1, defaultRetryAttempts)
		time.Sleep(defaultRetryTimeout)
	}

	if err != nil {
		log.Fatalf("failed to create migrate instance after multiple retries: %v", err)
	}

	log.Println("Applying migrations...")

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %v", err)
	}

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		log.Printf("migrate source error on close: %v", srcErr)
	}
	if dbErr != nil {
		log.Printf("migrate database error on close: %v", dbErr)
	}

	if err == migrate.ErrNoChange {
		log.Println("No new migrations to apply.")
	} else {
		log.Println("Migrations applied successfully!")
	}
} 