SELECT 
    chrt_id, track_number, price, rid, name, 
    sale, size, total_price, nm_id, brand, status
FROM order_items 
WHERE order_uid = $1
ORDER BY chrt_id
