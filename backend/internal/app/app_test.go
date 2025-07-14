package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	redisClient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocks
type mockDB struct {
	mock.Mock
}

func (m *mockDB) PingContext(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
func (m *mockDB) Close() error { return nil }

type mockRedis struct {
	mock.Mock
}

func (m *mockRedis) Ping(ctx context.Context) *redisClient.StatusCmd {
	args := m.Called(ctx)
	return args.Get(0).(*redisClient.StatusCmd)
}
func (m *mockRedis) Close() error { return nil }

type mockConsumer struct {
	mock.Mock
}

func (m *mockConsumer) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockConsumer) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockConsumer) Stop(ctx context.Context) error { return nil }

func TestApp_Health(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	t.Run("all healthy", func(t *testing.T) {
		dbMock := new(mockDB)
		redisMock := new(mockRedis)
		consumerMock := new(mockConsumer)

		dbMock.On("PingContext", ctx).Return(nil).Once()
		redisMock.On("Ping", ctx).Return(redisClient.NewStatusResult("", nil)).Once()
		consumerMock.On("Health", ctx).Return(nil).Once()

		app := NewApp(logger, nil, nil, dbMock, redisMock, consumerMock, nil)
		err := app.Health(ctx)
		assert.NoError(t, err)

		dbMock.AssertExpectations(t)
		redisMock.AssertExpectations(t)
		consumerMock.AssertExpectations(t)
	})

	t.Run("db unhealthy", func(t *testing.T) {
		dbMock := new(mockDB)
		dbErr := errors.New("db down")
		dbMock.On("PingContext", ctx).Return(dbErr).Once()

		app := NewApp(logger, nil, nil, dbMock, nil, nil, nil)
		err := app.Health(ctx)
		assert.Error(t, err)
		assert.Equal(t, dbErr, err)
		dbMock.AssertExpectations(t)
	})

	t.Run("redis unhealthy", func(t *testing.T) {
		dbMock := new(mockDB)
		redisMock := new(mockRedis)
		redisErr := errors.New("redis down")

		dbMock.On("PingContext", ctx).Return(nil).Once()
		redisMock.On("Ping", ctx).Return(redisClient.NewStatusResult("", redisErr)).Once()

		app := NewApp(logger, nil, nil, dbMock, redisMock, nil, nil)
		err := app.Health(ctx)
		assert.Error(t, err)
		assert.Equal(t, redisErr, err)
		dbMock.AssertExpectations(t)
		redisMock.AssertExpectations(t)
	})

	t.Run("consumer unhealthy", func(t *testing.T) {
		dbMock := new(mockDB)
		redisMock := new(mockRedis)
		consumerMock := new(mockConsumer)
		consumerErr := errors.New("kafka down")

		dbMock.On("PingContext", ctx).Return(nil).Once()
		redisMock.On("Ping", ctx).Return(redisClient.NewStatusResult("", nil)).Once()
		consumerMock.On("Health", ctx).Return(consumerErr).Once()

		app := NewApp(logger, nil, nil, dbMock, redisMock, consumerMock, nil)
		err := app.Health(ctx)
		assert.Error(t, err)
		assert.Equal(t, consumerErr, err)
		dbMock.AssertExpectations(t)
		redisMock.AssertExpectations(t)
		consumerMock.AssertExpectations(t)
	})
} 