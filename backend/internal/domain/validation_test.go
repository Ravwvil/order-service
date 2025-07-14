package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestValidationResult тестирует методы ValidationResult.
func TestValidationResult(t *testing.T) {
	t.Run("AddError", func(t *testing.T) {
		vr := &ValidationResult{Valid: true}
		assert.True(t, vr.Valid)
		assert.False(t, vr.HasErrors())

		vr.AddError("field1", "message1")
		assert.False(t, vr.Valid)
		assert.True(t, vr.HasErrors())
		assert.Len(t, vr.Errors, 1)
		assert.Equal(t, "field1", vr.Errors[0].Field)
		assert.Equal(t, "message1", vr.Errors[0].Message)
	})

	t.Run("GetFirstError", func(t *testing.T) {
		vr := &ValidationResult{Valid: true}
		assert.Nil(t, vr.GetFirstError())

		vr.AddError("field1", "message1")
		err := vr.GetFirstError()
		assert.NotNil(t, err)
		assert.Equal(t, "field1: message1", err.Error())
	})
}

// validItem возвращает валидный Item для тестов.
func validItem() Item {
	return Item{
		ChrtID:      123,
		TrackNumber: "TRACK123",
		Price:       100,
		Rid:         "RID123",
		Name:        "Test Item",
		Sale:        10,
		Size:        "M",
		TotalPrice:  90,
		NmID:        456,
		Brand:       "Test Brand",
		Status:      200,
	}
}

// validDelivery возвращает валидный Delivery для тестов.
func validDelivery() Delivery {
	return Delivery{
		Name:    "Test User",
		Phone:   "+1234567890",
		Zip:     "12345",
		City:    "Test City",
		Address: "Test Address",
		Region:  "Test Region",
		Email:   "test@example.com",
	}
}

// validPayment возвращает валидный Payment для тестов.
func validPayment() Payment {
	return Payment{
		Transaction:  "TXN123",
		Currency:     "USD",
		Provider:     "test-provider",
		Amount:       150,
		PaymentDt:    time.Now().Unix(),
		Bank:         "Test Bank",
		DeliveryCost: 50,
		GoodsTotal:   100,
		CustomFee:    0,
	}
}

// validOrder возвращает валидный Order для тестов.
func validOrder() *Order {
	return &Order{
		OrderUID:    "ORDER123",
		TrackNumber: "TRACK123",
		Entry:       "ENTRY",
		Locale:      "en",
		CustomerID:  "CUST123",
		Delivery:    validDelivery(),
		Payment:     validPayment(),
		Items:       []Item{validItem()},
	}
}

// TestOrder_Validate тестирует логику валидации для структуры Order.
func TestOrder_Validate(t *testing.T) {
	t.Run("valid order", func(t *testing.T) {
		order := validOrder()
		result := order.Validate()
		assert.True(t, result.Valid)
		assert.False(t, result.HasErrors())
	})

	t.Run("missing required fields", func(t *testing.T) {
		testCases := []struct {
			name    string
			mutator func(*Order)
		}{
			{"order_uid", func(o *Order) { o.OrderUID = "" }},
			{"track_number", func(o *Order) { o.TrackNumber = "" }},
			{"entry", func(o *Order) { o.Entry = "" }},
			{"locale", func(o *Order) { o.Locale = "" }},
			{"customer_id", func(o *Order) { o.CustomerID = "" }},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				order := validOrder()
				tc.mutator(order)
				result := order.Validate()
				assert.False(t, result.Valid)
				assert.True(t, result.HasErrors())
				assert.Equal(t, tc.name, result.Errors[0].Field)
			})
		}
	})

	t.Run("no items", func(t *testing.T) {
		order := validOrder()
		order.Items = []Item{}
		result := order.Validate()
		assert.False(t, result.Valid)
		assert.True(t, result.HasErrors())
		assert.Equal(t, "items", result.Errors[0].Field)
	})

	t.Run("invalid delivery", func(t *testing.T) {
		order := validOrder()
		order.Delivery.Name = ""
		result := order.Validate()
		assert.False(t, result.Valid)
		assert.True(t, result.HasErrors())
		assert.Equal(t, "delivery.name", result.Errors[0].Field)
	})

	t.Run("invalid payment", func(t *testing.T) {
		order := validOrder()
		order.Payment.Amount = -1
		result := order.Validate()
		assert.False(t, result.Valid)
		assert.True(t, result.HasErrors())
		assert.Equal(t, "payment.amount", result.Errors[0].Field)
	})

	t.Run("invalid item", func(t *testing.T) {
		order := validOrder()
		order.Items[0].ChrtID = 0
		result := order.Validate()
		assert.False(t, result.Valid)
		assert.True(t, result.HasErrors())
		assert.Equal(t, "items[0].chrt_id", result.Errors[0].Field)
	})
}

// TestDelivery_Validate тестирует логику валидации для структуры Delivery.
func TestDelivery_Validate(t *testing.T) {
	t.Run("valid delivery", func(t *testing.T) {
		delivery := validDelivery()
		result := delivery.Validate()
		assert.True(t, result.Valid)
	})

	t.Run("missing required fields", func(t *testing.T) {
		testCases := []struct {
			name    string
			mutator func(*Delivery)
		}{
			{"name", func(d *Delivery) { d.Name = " " }},
			{"phone", func(d *Delivery) { d.Phone = " " }},
			{"city", func(d *Delivery) { d.City = " " }},
			{"address", func(d *Delivery) { d.Address = " " }},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				delivery := validDelivery()
				tc.mutator(&delivery)
				result := delivery.Validate()
				assert.False(t, result.Valid)
				assert.Equal(t, tc.name, result.Errors[0].Field)
			})
		}
	})
}

// TestPayment_Validate тестирует логику валидации для структуры Payment.
func TestPayment_Validate(t *testing.T) {
	t.Run("valid payment", func(t *testing.T) {
		payment := validPayment()
		result := payment.Validate()
		assert.True(t, result.Valid)
	})

	t.Run("amount mismatch", func(t *testing.T) {
		payment := validPayment()
		payment.Amount = 1
		result := payment.Validate()
		assert.False(t, result.Valid)
		assert.Equal(t, "amount", result.Errors[0].Field)
	})

	t.Run("negative values", func(t *testing.T) {
		testCases := []struct {
			name          string
			mutator       func(*Payment)
			expectedField string
		}{
			{"amount", func(p *Payment) { p.Amount = -1 }, "amount"},
			{"delivery_cost", func(p *Payment) { p.DeliveryCost = -1 }, "delivery_cost"},
			{"goods_total", func(p *Payment) { p.GoodsTotal = -1 }, "goods_total"},
			{"custom_fee", func(p *Payment) { p.CustomFee = -1 }, "custom_fee"},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				payment := validPayment()
				tc.mutator(&payment)
				result := payment.Validate()
				assert.False(t, result.Valid)
				assert.True(t, result.HasErrors())
				assert.Equal(t, tc.expectedField, result.Errors[0].Field)
			})
		}
	})
}

// TestItem_Validate тестирует логику валидации для структуры Item.
func TestItem_Validate(t *testing.T) {
	t.Run("valid item", func(t *testing.T) {
		item := validItem()
		result := item.Validate()
		assert.True(t, result.Valid)
	})

	t.Run("invalid numeric fields", func(t *testing.T) {
		testCases := []struct {
			name    string
			mutator func(*Item)
		}{
			{"chrt_id", func(i *Item) { i.ChrtID = 0 }},
			{"price", func(i *Item) { i.Price = -1 }},
			{"sale", func(i *Item) { i.Sale = -1 }},
			{"total_price", func(i *Item) { i.TotalPrice = -1 }},
			{"nm_id", func(i *Item) { i.NmID = 0 }},
			{"status", func(i *Item) { i.Status = -1 }},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				item := validItem()
				tc.mutator(&item)
				result := item.Validate()
				assert.False(t, result.Valid, "failed on field "+tc.name)
				assert.Equal(t, tc.name, result.Errors[0].Field)
			})
		}
	})

	t.Run("missing required string fields", func(t *testing.T) {
		testCases := []struct {
			name    string
			mutator func(*Item)
		}{
			{"track_number", func(i *Item) { i.TrackNumber = " " }},
			{"rid", func(i *Item) { i.Rid = " " }},
			{"name", func(i *Item) { i.Name = " " }},
			{"size", func(i *Item) { i.Size = " " }},
			{"brand", func(i *Item) { i.Brand = " " }},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				item := validItem()
				tc.mutator(&item)
				result := item.Validate()
				assert.False(t, result.Valid)
				assert.Equal(t, tc.name, result.Errors[0].Field)
			})
		}
	})
} 