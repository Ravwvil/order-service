CREATE TABLE IF NOT EXISTS orders (
    order_uid TEXT PRIMARY KEY, -- Поидее можно использовать UUID для уникальности
    track_number TEXT NOT NULL,
    entry TEXT NOT NULL,
    locale TEXT NOT NULL,
    internal_signature TEXT,
    customer_id TEXT NOT NULL,
    delivery_service TEXT,
    shardkey TEXT,
    sm_id INTEGER,
    date_created TIMESTAMPTZ NOT NULL,
    oof_shard TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deliveries (
    order_uid TEXT PRIMARY KEY REFERENCES orders(order_uid) ON DELETE CASCADE,
    name TEXT NOT NULL,
    phone TEXT NOT NULL,
    zip TEXT NOT NULL,
    city TEXT NOT NULL,
    address TEXT NOT NULL,
    region TEXT NOT NULL,
    email TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS payments (
    order_uid TEXT PRIMARY KEY REFERENCES orders(order_uid) ON DELETE CASCADE,
    transaction TEXT NOT NULL,
    request_id TEXT,
    currency TEXT NOT NULL,
    provider TEXT NOT NULL,
    amount INTEGER NOT NULL CHECK (amount >= 0),
    payment_dt BIGINT NOT NULL CHECK (payment_dt > 0),
    bank TEXT NOT NULL,
    delivery_cost INTEGER NOT NULL CHECK (delivery_cost >= 0),
    goods_total INTEGER NOT NULL CHECK (goods_total >= 0),
    custom_fee INTEGER NOT NULL DEFAULT 0 CHECK (custom_fee >= 0)
);

CREATE TABLE IF NOT EXISTS order_items (
    order_uid TEXT NOT NULL REFERENCES orders(order_uid) ON DELETE CASCADE,
    chrt_id INTEGER NOT NULL,
    track_number TEXT NOT NULL,
    price INTEGER NOT NULL CHECK (price >= 0),
    rid TEXT NOT NULL,
    name TEXT NOT NULL,
    sale INTEGER NOT NULL CHECK (sale >= 0),
    size TEXT,
    total_price INTEGER NOT NULL CHECK (total_price >= 0),
    nm_id INTEGER NOT NULL,
    brand TEXT NOT NULL,
    status INTEGER NOT NULL,
    PRIMARY KEY (order_uid, rid) 
    -- Предполагается, что несколько одинаковых товаров (chrt_id) могут быть в одном заказе. 
    -- Можно также использовать отдельный id для order_items в такой имлементации.
    -- Если предполагается, что один товар может быть в одном заказе только один раз,
    -- То можно использовать order_uid и chrt_id как составной первичный ключ.
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE INDEX IF NOT EXISTS idx_orders_date_created ON orders(date_created);
CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_order_items_order_uid ON order_items(order_uid);
CREATE INDEX IF NOT EXISTS idx_order_items_nm_id ON order_items(nm_id);

CREATE TRIGGER trg_orders_updated_at
  BEFORE UPDATE ON orders
  FOR EACH ROW
  EXECUTE FUNCTION set_updated_at();

