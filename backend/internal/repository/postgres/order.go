package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

type orderWithItemsRow struct {
	orderRow
	// Item fields (nullable)
	ChrtID          sql.NullInt64  `db:"chrt_id"`
	ItemTrackNumber sql.NullString `db:"item_track_number"`
	Price           sql.NullInt64  `db:"price"`
	Rid             sql.NullString `db:"rid"`
	ItemName        sql.NullString `db:"item_name"`
	Sale            sql.NullInt64  `db:"sale"`
	Size            sql.NullString `db:"size"`
	TotalPrice      sql.NullInt64  `db:"total_price"`
	NmID            sql.NullInt64  `db:"nm_id"`
	Brand           sql.NullString `db:"brand"`
	Status          sql.NullInt64  `db:"status"`
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

	// Подготавливаем запрос для массовой вставки
	query, args, err := r.buildBulkInsertItemsQuery(orderUID, items)
	if err != nil {
		return err
	}

	// Выполняем запрос
	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		r.logger.Error("failed to bulk insert items",
			slog.String("order_uid", orderUID),
			slog.Any("error", err))
		return err
	}

	return nil
}

func (r *OrderRepository) buildBulkInsertItemsQuery(orderUID string, items []domain.Item) (string, []interface{}, error) {
	if len(items) == 0 {
		return "", nil, errors.New("no items to insert")
	}

	var valueStrings []string
	var valueArgs []interface{}
	i := 1
	for _, item := range items {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i, i+1, i+2, i+3, i+4, i+5, i+6, i+7, i+8, i+9, i+10, i+11))
		valueArgs = append(valueArgs, orderUID, item.ChrtID, item.TrackNumber, item.Price, item.Rid, item.Name, item.Sale, item.Size, item.TotalPrice, item.NmID, item.Brand, item.Status)
		i += 12
	}

	query := fmt.Sprintf("%s %s", insertItemQuery, strings.Join(valueStrings, ","))
	return query, valueArgs, nil
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
	var rows []orderWithItemsRow
	err := r.db.SelectContext(ctx, &rows, selectAllOrdersWithItemsQuery)
	if err != nil {
		r.logger.Error("failed to get all orders with items", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get all orders with items: %w", err)
	}

	ordersMap := make(map[string]*domain.Order)
	orderedUIDs := make([]string, 0)

	for _, row := range rows {
		if _, exists := ordersMap[row.OrderUID]; !exists {
			order := row.orderRow.toDomainOrder()
			order.Items = []domain.Item{} // Initialize items slice
			ordersMap[row.OrderUID] = order
			orderedUIDs = append(orderedUIDs, row.OrderUID)
		}

		if row.ChrtID.Valid {
			item := domain.Item{
				OrderUID:    row.OrderUID,
				ChrtID:      int(row.ChrtID.Int64),
				TrackNumber: row.ItemTrackNumber.String,
				Price:       int(row.Price.Int64),
				Rid:         row.Rid.String,
				Name:        row.ItemName.String,
				Sale:        int(row.Sale.Int64),
				Size:        row.Size.String,
				TotalPrice:  int(row.TotalPrice.Int64),
				NmID:        int(row.NmID.Int64),
				Brand:       row.Brand.String,
				Status:      int(row.Status.Int64),
			}
			order := ordersMap[row.OrderUID]
			order.Items = append(order.Items, item)
		}
	}

	// Preserve original order
	orders := make([]*domain.Order, len(orderedUIDs))
	for i, uid := range orderedUIDs {
		orders[i] = ordersMap[uid]
	}

	r.logger.Info("retrieved all orders successfully",
		slog.Int("count", len(orders)))

	return orders, nil
}
