INSERT INTO payments (
    order_uid, transaction, request_id, currency, provider, amount,
    payment_dt, bank, delivery_cost, goods_total, custom_fee
) VALUES (
    :order_uid, :transaction, :request_id, :currency, :provider, :amount,
    :payment_dt, :bank, :delivery_cost, :goods_total, :custom_fee
)
