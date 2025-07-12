package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/jmoiron/sqlx"
)

// orderRow - результат JOIN запроса для получения заказа с delivery и payment
type orderRow struct {
	// Order fields
	OrderUID          string    `db:"order_uid"`
	TrackNumber       string    `db:"track_number"`
	Entry             string    `db:"entry"`
	Locale            string    `db:"locale"`
	InternalSignature string    `db:"internal_signature"`
	CustomerID        string    `db:"customer_id"`
	DeliveryService   string    `db:"delivery_service"`
	ShardKey          string    `db:"shardkey"`
	SmID              int       `db:"sm_id"`
	DateCreated       time.Time `db:"date_created"`
	OofShard          string    `db:"oof_shard"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`

	// Delivery fields (nullable из-за LEFT JOIN)
	DeliveryName    sql.NullString `db:"delivery_name"`
	DeliveryPhone   sql.NullString `db:"delivery_phone"`
	DeliveryZip     sql.NullString `db:"delivery_zip"`
	DeliveryCity    sql.NullString `db:"delivery_city"`
	DeliveryAddress sql.NullString `db:"delivery_address"`
	DeliveryRegion  sql.NullString `db:"delivery_region"`
	DeliveryEmail   sql.NullString `db:"delivery_email"`

	// Payment fields (nullable из-за LEFT JOIN)
	Transaction  sql.NullString `db:"transaction"`
	RequestID    sql.NullString `db:"request_id"`
	Currency     sql.NullString `db:"currency"`
	Provider     sql.NullString `db:"provider"`
	Amount       sql.NullInt64  `db:"amount"`
	PaymentDt    sql.NullInt64  `db:"payment_dt"`
	Bank         sql.NullString `db:"bank"`
	DeliveryCost sql.NullInt64  `db:"delivery_cost"`
	GoodsTotal   sql.NullInt64  `db:"goods_total"`
	CustomFee    sql.NullInt64  `db:"custom_fee"`
}

// toDomainOrder преобразует orderRow в domain.Order
func (row *orderRow) toDomainOrder() *domain.Order {
	order := &domain.Order{
		OrderUID:          row.OrderUID,
		TrackNumber:       row.TrackNumber,
		Entry:             row.Entry,
		Locale:            row.Locale,
		InternalSignature: row.InternalSignature,
		CustomerID:        row.CustomerID,
		DeliveryService:   row.DeliveryService,
		ShardKey:          row.ShardKey,
		SmID:              row.SmID,
		DateCreated:       row.DateCreated,
		OofShard:          row.OofShard,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}

	// Заполняем delivery если валидно
	if row.DeliveryName.Valid {
		order.Delivery = domain.Delivery{
			OrderUID: row.OrderUID,
			Name:     row.DeliveryName.String,
			Phone:    row.DeliveryPhone.String,
			Zip:      row.DeliveryZip.String,
			City:     row.DeliveryCity.String,
			Address:  row.DeliveryAddress.String,
			Region:   row.DeliveryRegion.String,
			Email:    row.DeliveryEmail.String,
		}
	}

	// Заполняем payment если валидно
	if row.Transaction.Valid {
		order.Payment = domain.Payment{
			OrderUID:     row.OrderUID,
			Transaction:  row.Transaction.String,
			RequestID:    row.RequestID.String,
			Currency:     row.Currency.String,
			Provider:     row.Provider.String,
			Amount:       int(row.Amount.Int64),
			PaymentDt:    row.PaymentDt.Int64,
			Bank:         row.Bank.String,
			DeliveryCost: int(row.DeliveryCost.Int64),
			GoodsTotal:   int(row.GoodsTotal.Int64),
			CustomFee:    int(row.CustomFee.Int64),
		}
	}

	return order
}

type OrderRepository struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewOrderRepository(db *sqlx.DB, logger *slog.Logger) *OrderRepository {
	return &OrderRepository{
		db:     db,
		logger: logger,
	}
}

func (r *OrderRepository) Create(ctx context.Context, order *domain.Order) error {
	// Проверяем валидность заказа
	validationResult := order.Validate()
	if validationResult.HasErrors() {
		r.logger.Error("received invalid order data",
			slog.String("order_uid", order.OrderUID),
			slog.Any("validation_errors", validationResult.Errors))
		return fmt.Errorf("validation failed: %w", validationResult.GetFirstError())
	}

	// Начинаем транзакцию
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		r.logger.Error("failed to begin transaction", slog.Any("error", err))
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				r.logger.Error("failed to rollback transaction", slog.Any("error", rollbackErr))
			}
		}
	}()

	// Устанавливаем временные метки, если они ещё не были заданы
	now := time.Now()
	if order.CreatedAt.IsZero() {
		order.CreatedAt = now
	}
	if order.UpdatedAt.IsZero() {
		order.UpdatedAt = now
	}

	// 1. Создаем основной заказ
	if err = r.createOrder(ctx, tx, order); err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	// 2. Создаем delivery
	if err = r.createDelivery(ctx, tx, order.OrderUID, &order.Delivery); err != nil {
		return fmt.Errorf("failed to create delivery: %w", err)
	}

	// 3. Создаем payment
	if err = r.createPayment(ctx, tx, order.OrderUID, &order.Payment); err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	// 4. Создаем items
	if err = r.createItems(ctx, tx, order.OrderUID, order.Items); err != nil {
		return fmt.Errorf("failed to create items: %w", err)
	}

	// Коммитим транзакцию
	if err = tx.Commit(); err != nil {
		r.logger.Error("failed to commit transaction", slog.Any("error", err))
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("order created successfully",
		slog.String("order_uid", order.OrderUID),
		slog.Int("items_count", len(order.Items)))

	return nil
}

// createOrder создает основну заказа в транзакции
func (r *OrderRepository) createOrder(ctx context.Context, tx *sqlx.Tx, order *domain.Order) error {
	// Выполняем запрос на вставку заказа
	_, err := tx.NamedExecContext(ctx, insertOrderQuery, order)
	if err != nil {
		r.logger.Error("failed to insert order",
			slog.String("order_uid", order.OrderUID),
			slog.Any("error", err))
		return err
	}
	return nil
}

// createDelivery создает запись доставки в транзакции
func (r *OrderRepository) createDelivery(ctx context.Context, tx *sqlx.Tx, orderUID string, delivery *domain.Delivery) error {
	// Устанавливаем order_uid для связи
	delivery.OrderUID = orderUID

	_, err := tx.NamedExecContext(ctx, insertDeliveryQuery, delivery)
	if err != nil {
		r.logger.Error("failed to insert delivery",
			slog.String("order_uid", orderUID),
			slog.Any("error", err))
		return err
	}
	return nil
}

// createPayment создает запись платежа в транзакции
func (r *OrderRepository) createPayment(ctx context.Context, tx *sqlx.Tx, orderUID string, payment *domain.Payment) error {
	// Устанавливаем order_uid для связи
	payment.OrderUID = orderUID

	_, err := tx.NamedExecContext(ctx, insertPaymentQuery, payment)
	if err != nil {
		r.logger.Error("failed to insert payment",
			slog.String("order_uid", orderUID),
			slog.Any("error", err))
		return err
	}
	return nil
}

// createItems создает записи товаров в транзакции
func (r *OrderRepository) createItems(ctx context.Context, tx *sqlx.Tx, orderUID string, items []domain.Item) error {
	if len(items) == 0 {
		return nil
	}

	for _, item := range items {
		// Устанавливаем order_uid для связи
		item.OrderUID = orderUID

		_, err := tx.NamedExecContext(ctx, insertItemQuery, item)
		if err != nil {
			r.logger.Error("failed to insert item",
				slog.String("order_uid", orderUID),
				slog.Int("chrt_id", item.ChrtID),
				slog.Any("error", err))
			return err
		}
	}
	return nil
}

func (r *OrderRepository) GetByUID(ctx context.Context, uid string) (*domain.Order, error) {
	var row orderRow
	err := r.db.GetContext(ctx, &row, selectOrderByUIDQuery, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.Debug("order not found", slog.String("order_uid", uid))
			return nil, fmt.Errorf("order with uid %s not found", uid)
		}
		r.logger.Error("failed to get order",
			slog.String("order_uid", uid),
			slog.Any("error", err))
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Создаем объект заказа
	order := row.toDomainOrder()

	// Получаем товары заказа
	items, err := r.getOrderItems(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("failed to get order items: %w", err)
	}
	order.Items = items

	r.logger.Debug("order retrieved successfully",
		slog.String("order_uid", uid),
		slog.Int("items_count", len(items)))

	return order, nil
}

// getOrderItems получает все товары заказа
func (r *OrderRepository) getOrderItems(ctx context.Context, orderUID string) ([]domain.Item, error) {
	var items []domain.Item
	err := r.db.SelectContext(ctx, &items, selectItemsByUIDQuery, orderUID)
	if err != nil {
		r.logger.Error("failed to get order items",
			slog.String("order_uid", orderUID),
			slog.Any("error", err))
		return nil, err
	}

	// Устанавливаем OrderUID для каждого товара
	for i := range items {
		items[i].OrderUID = orderUID
	}

	return items, nil
}

func (r *OrderRepository) GetAll(ctx context.Context) ([]*domain.Order, error) {
	var orderRows []orderRow
	err := r.db.SelectContext(ctx, &orderRows, selectAllOrdersQuery)
	if err != nil {
		r.logger.Error("failed to get all orders", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get all orders: %w", err)
	}

	orders := make([]*domain.Order, 0, len(orderRows))

	// Обрабатываем каждую строку
	for _, row := range orderRows {
		order := row.toDomainOrder()

		// Получаем items для каждого заказа
		items, err := r.getOrderItems(ctx, row.OrderUID)
		if err != nil {
			r.logger.Error("failed to get order items",
				slog.String("order_uid", row.OrderUID),
				slog.Any("error", err))
			return nil, fmt.Errorf("failed to get items for order %s: %w", row.OrderUID, err)
		}
		order.Items = items

		orders = append(orders, order)
	}

	r.logger.Info("retrieved all orders successfully",
		slog.Int("count", len(orders)))

	return orders, nil
}
