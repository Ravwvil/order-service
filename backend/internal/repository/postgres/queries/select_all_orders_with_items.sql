SELECT 
    o.order_uid, o.track_number, o.entry, o.locale, o.internal_signature,
    o.customer_id, o.delivery_service, o.shardkey, o.sm_id, o.date_created,
    o.oof_shard, o.created_at, o.updated_at,
    
    d.name as delivery_name, d.phone as delivery_phone, d.zip as delivery_zip,
    d.city as delivery_city, d.address as delivery_address, d.region as delivery_region,
    d.email as delivery_email,
    
    p.transaction, p.request_id, p.currency, p.provider, p.amount,
    p.payment_dt, p.bank, p.delivery_cost, p.goods_total, p.custom_fee,

    i.chrt_id, i.track_number as item_track_number, i.price, i.rid, i.name as item_name,
    i.sale, i.size, i.total_price, i.nm_id, i.brand, i.status
FROM orders o
LEFT JOIN deliveries d ON o.order_uid = d.order_uid
LEFT JOIN payments p ON o.order_uid = p.order_uid
LEFT JOIN order_items i ON o.order_uid = i.order_uid
ORDER BY o.created_at DESC, o.order_uid, i.chrt_id; 