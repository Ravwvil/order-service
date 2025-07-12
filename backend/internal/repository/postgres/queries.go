package postgres

import (
	_ "embed"
)

// Embedded SQL queries
var (
	//go:embed queries/insert_order.sql
	insertOrderQuery string

	//go:embed queries/insert_delivery.sql
	insertDeliveryQuery string

	//go:embed queries/insert_payment.sql
	insertPaymentQuery string

	//go:embed queries/insert_item.sql
	insertItemQuery string

	//go:embed queries/select_by_uid.sql
	selectOrderByUIDQuery string

	//go:embed queries/select_items_by_uid.sql
	selectItemsByUIDQuery string

	//go:embed queries/select_all_orders.sql
	selectAllOrdersQuery string
)
