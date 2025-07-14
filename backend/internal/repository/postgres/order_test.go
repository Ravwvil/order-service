package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	db     *sqlx.DB
	repo   *OrderRepository
	logger *slog.Logger
)

// Test Helpers
func loadOrderFromJSON(t *testing.T, path string) *domain.Order {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read file")
	var order domain.Order
	require.NoError(t, json.Unmarshal(data, &order), "failed to unmarshal order")
	return &order
}

func runMigrations(dsn string) error {
	migrationsPath, err := filepath.Abs("../../../migrations")
	if err != nil {
		return fmt.Errorf("could not get absolute path for migrations: %w", err)
	}

	migrationsPath = filepath.ToSlash(migrationsPath)

	m, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	pgUser := "testuser"
	pgPassword := "testpassword"
	pgDatabase := "testdb"

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16.3-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     pgUser,
			"POSTGRES_PASSWORD": pgPassword,
			"POSTGRES_DB":       pgDatabase,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(5 * time.Minute),
	}

	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		log.Fatalf("could not start postgres container: %v", err)
	}
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Fatalf("failed to terminate container: %s", err.Error())
		}
	}()

	host, err := pgContainer.Host(ctx)
	if err != nil {
		log.Fatalf("failed to get container host: %v", err)
	}
	port, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		log.Fatalf("failed to get mapped port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", pgUser, pgPassword, host, port.Port(), pgDatabase)

	// Run migrations
	if err := runMigrations(dsn); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	db, err = sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("could not connect to postgres: %v", err)
	}

	repo = NewOrderRepository(db, logger)

	code := m.Run()

	os.Exit(code)
}

func clearTables() {
	_, err := db.Exec("TRUNCATE order_items, deliveries, payments, orders RESTART IDENTITY CASCADE")
	if err != nil {
		log.Fatalf("failed to truncate tables: %v", err)
	}
}

func TestOrderRepository_Create(t *testing.T) {
	order := loadOrderFromJSON(t, "../../service/testdata/valid_order.json")
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		clearTables()

		err := repo.Create(ctx, order)
		assert.NoError(t, err)

		// Verify data was created
		var count int
		err = db.Get(&count, "SELECT COUNT(*) FROM orders WHERE order_uid = $1", order.OrderUID)
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("invalid order", func(t *testing.T) {
		clearTables()
		invalidOrder := loadOrderFromJSON(t, "../../service/testdata/valid_order.json")
		invalidOrder.OrderUID = "" // make it invalid

		err := repo.Create(ctx, invalidOrder)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})
}

func TestOrderRepository_GetByUID(t *testing.T) {
	order := loadOrderFromJSON(t, "../../service/testdata/valid_order.json")
	ctx := context.Background()

	clearTables()
	err := repo.Create(ctx, order)
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		retrievedOrder, err := repo.GetByUID(ctx, order.OrderUID)
		require.NoError(t, err)
		require.NotNil(t, retrievedOrder)

		// Timestamps can have timezone differences. We check they are recent,
		// then overwrite them to allow for a clean comparison of the rest of the struct.
		assert.WithinDuration(t, time.Now(), retrievedOrder.CreatedAt, 20*time.Second)
		assert.WithinDuration(t, time.Now(), retrievedOrder.UpdatedAt, 20*time.Second)
		order.CreatedAt = retrievedOrder.CreatedAt
		order.UpdatedAt = retrievedOrder.UpdatedAt

		// The OrderUID in items is not in the JSON, so we set it manually for the comparison
		for i := range order.Items {
			order.Items[i].OrderUID = order.OrderUID
		}

		assert.Equal(t, order, retrievedOrder)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByUID(ctx, "non-existent-uid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestOrderRepository_GetAll(t *testing.T) {
	ctx := context.Background()

	order1 := loadOrderFromJSON(t, "../../service/testdata/valid_order.json")
	order2 := loadOrderFromJSON(t, "../../service/testdata/valid_order.json")
	order2.OrderUID = "another-uid-for-getall"
	order2.TrackNumber = "another-track-number"

	clearTables()
	err := repo.Create(ctx, order1)
	require.NoError(t, err)
	err = repo.Create(ctx, order2)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		orders, err := repo.GetAll(ctx)
		assert.NoError(t, err)
		assert.Len(t, orders, 2)

		// Make the assertion more robust
		expectedOrders := map[string]*domain.Order{
			order1.OrderUID: order1,
			order2.OrderUID: order2,
		}

		for _, retrievedOrder := range orders {
			expectedOrder, ok := expectedOrders[retrievedOrder.OrderUID]
			assert.True(t, ok, "retrieved unexpected order UID: %s", retrievedOrder.OrderUID)

			assert.WithinDuration(t, time.Now(), retrievedOrder.CreatedAt, 20*time.Second)
			assert.WithinDuration(t, time.Now(), retrievedOrder.UpdatedAt, 20*time.Second)
			expectedOrder.CreatedAt = retrievedOrder.CreatedAt
			expectedOrder.UpdatedAt = retrievedOrder.UpdatedAt

			for i := range expectedOrder.Items {
				expectedOrder.Items[i].OrderUID = expectedOrder.OrderUID
			}
			assert.Equal(t, expectedOrder, retrievedOrder)
		}
	})
}
