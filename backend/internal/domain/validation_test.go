package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

func TestOrder_Validate(t *testing.T) {
	t.Run("valid order", func(t *testing.T) {
		order := validOrder()
		result := order.Validate()
		assert.True(t, result.Valid)
		assert.False(t, result.HasErrors())
	})

	t.Run("missing required fields", func(t *testing.T) {
		testCases := []struct {
			name  string
			field *string
		}{
			{"order_uid", &validOrder().OrderUID},
			{"track_number", &validOrder().TrackNumber},
			{"entry", &validOrder().Entry},
			{"locale", &validOrder().Locale},
			{"customer_id", &validOrder().CustomerID},
		}

		for _, tc := range testCases {
			order := validOrder()
			*tc.field = ""
			result := order.Validate()
			assert.False(t, result.Valid)
			assert.True(t, result.HasErrors())
			assert.Equal(t, tc.name, result.Errors[0].Field)
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

func TestDelivery_Validate(t *testing.T) {
	t.Run("valid delivery", func(t *testing.T) {
		delivery := validDelivery()
		result := delivery.Validate()
		assert.True(t, result.Valid)
	})

	t.Run("missing required fields", func(t *testing.T) {
		testCases := []struct {
			name  string
			field *string
		}{
			{"name", &validDelivery().Name},
			{"phone", &validDelivery().Phone},
			{"city", &validDelivery().City},
			{"address", &validDelivery().Address},
		}

		for _, tc := range testCases {
			delivery := validDelivery()
			*tc.field = "  " // Test with whitespace
			result := delivery.Validate()
			assert.False(t, result.Valid)
			assert.Equal(t, tc.name, result.Errors[0].Field)
		}
	})
}

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
			name  string
			field *int
		}{
			{"amount", &validPayment().Amount},
			{"delivery_cost", &validPayment().DeliveryCost},
			{"goods_total", &validPayment().GoodsTotal},
			{"custom_fee", &validPayment().CustomFee},
		}
		for _, tc := range testCases {
			payment := validPayment()
			*tc.field = -1
			result := payment.Validate()
			assert.False(t, result.Valid)
			assert.Equal(t, tc.name, result.Errors[0].Field)
		}
	})
}

func TestItem_Validate(t *testing.T) {
	t.Run("valid item", func(t *testing.T) {
		item := validItem()
		result := item.Validate()
		assert.True(t, result.Valid)
	})

	t.Run("invalid numeric fields", func(t *testing.T) {
		testCases := []struct {
			name      string
			transform func(*Item)
		}{
			{"chrt_id", func(i *Item) { i.ChrtID = 0 }},
			{"price", func(i *Item) { i.Price = -1 }},
			{"sale", func(i *Item) { i.Sale = -1 }},
			{"total_price", func(i *Item) { i.TotalPrice = -1 }},
			{"nm_id", func(i *Item) { i.NmID = 0 }},
			{"status", func(i *Item) { i.Status = -1 }},
		}
		for _, tc := range testCases {
			item := validItem()
			tc.transform(&item)
			result := item.Validate()
			assert.False(t, result.Valid, "failed on field "+tc.name)
			assert.Equal(t, tc.name, result.Errors[0].Field)
		}
	})

	t.Run("missing required string fields", func(t *testing.T) {
		testCases := []struct {
			name  string
			field *string
		}{
			{"track_number", &validItem().TrackNumber},
			{"rid", &validItem().Rid},
			{"name", &validItem().Name},
			{"size", &validItem().Size},
			{"brand", &validItem().Brand},
		}

		for _, tc := range testCases {
			item := validItem()
			*tc.field = "  "
			result := item.Validate()
			assert.False(t, result.Valid)
			assert.Equal(t, tc.name, result.Errors[0].Field)
		}
	})
} 