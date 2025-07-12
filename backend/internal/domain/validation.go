package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError содержит ошибки валидации
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidationResult содержит результаты валидации
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// AddError добавляет ошибку валидации
func (vr *ValidationResult) AddError(field, message string) {
	vr.Valid = false
	vr.Errors = append(vr.Errors, ValidationError{Field: field, Message: message})
}

// HasErrors проверяет наличие ошибок
func (vr *ValidationResult) HasErrors() bool {
	return len(vr.Errors) > 0
}

// GetFirstError возвращает первую ошибку или nil
func (vr *ValidationResult) GetFirstError() error {
	if len(vr.Errors) == 0 {
		return nil
	}
	return errors.New(vr.Errors[0].Error())
}

// Validator интерфейс для валидации
type Validator interface {
	Validate() ValidationResult
}

// Validate проверяет валидность заказа
func (o *Order) Validate() ValidationResult {
	result := ValidationResult{Valid: true}

	if o.OrderUID == "" {
		result.AddError("order_uid", "required field")
	}
	if o.TrackNumber == "" {
		result.AddError("track_number", "required field")
	}
	if o.Entry == "" {
		result.AddError("entry", "required field")
	}
	if o.Locale == "" {
		result.AddError("locale", "required field")
	}
	if o.CustomerID == "" {
		result.AddError("customer_id", "required field")
	}
	if len(o.Items) == 0 {
		result.AddError("items", "at least one item required")
	}

	// Валидация delivery
	deliveryResult := o.Delivery.Validate()
	if deliveryResult.HasErrors() {
		for _, err := range deliveryResult.Errors {
			result.AddError("delivery."+err.Field, err.Message)
		}
	}

	// Валидация payment
	paymentResult := o.Payment.Validate()
	if paymentResult.HasErrors() {
		for _, err := range paymentResult.Errors {
			result.AddError("payment."+err.Field, err.Message)
		}
	}

	// Валидация items
	for i, item := range o.Items {
		itemResult := item.Validate()
		if itemResult.HasErrors() {
			for _, err := range itemResult.Errors {
				result.AddError(fmt.Sprintf("items[%d].%s", i, err.Field), err.Message)
			}
		}
	}

	return result
}

// Validate проверяет валидность доставки
func (d *Delivery) Validate() ValidationResult {
	result := ValidationResult{Valid: true}

	if strings.TrimSpace(d.Name) == "" {
		result.AddError("name", "required field")
	}
	if strings.TrimSpace(d.Phone) == "" {
		result.AddError("phone", "required field")
	}
	if strings.TrimSpace(d.City) == "" {
		result.AddError("city", "required field")
	}
	if strings.TrimSpace(d.Address) == "" {
		result.AddError("address", "required field")
	}

	return result
}

// Validate Проверка валидности платежа
func (p *Payment) Validate() ValidationResult {
	result := ValidationResult{Valid: true}

	if strings.TrimSpace(p.Transaction) == "" {
		result.AddError("transaction", "required field")
	}
	if strings.TrimSpace(p.Currency) == "" {
		result.AddError("currency", "required field")
	}
	if strings.TrimSpace(p.Provider) == "" {
		result.AddError("provider", "required field")
	}
	if p.Amount < 0 {
		result.AddError("amount", "must be non-negative")
	}
	if p.PaymentDt <= 0 {
		result.AddError("payment_dt", "must be positive")
	}
	if p.DeliveryCost < 0 {
		result.AddError("delivery_cost", "must be non-negative")
	}
	if p.GoodsTotal < 0 {
		result.AddError("goods_total", "must be non-negative")
	}
	if p.CustomFee < 0 {
		result.AddError("custom_fee", "must be non-negative")
	}

	// Проверка логической целостности
	expectedTotal := p.GoodsTotal + p.DeliveryCost + p.CustomFee
	if p.Amount != expectedTotal {
		result.AddError("amount", fmt.Sprintf("amount (%d) must equal goods_total + delivery_cost + custom_fee (%d)", p.Amount, expectedTotal))
	}

	return result
}

// Validate проверяет валидность товара
func (i *Item) Validate() ValidationResult {
	result := ValidationResult{Valid: true}

	if i.ChrtID <= 0 {
		result.AddError("chrt_id", "must be positive")
	}
	if strings.TrimSpace(i.TrackNumber) == "" {
		result.AddError("track_number", "required field")
	}
	if i.Price < 0 {
		result.AddError("price", "must be non-negative")
	}
	if strings.TrimSpace(i.Rid) == "" {
		result.AddError("rid", "required field")
	}
	if strings.TrimSpace(i.Name) == "" {
		result.AddError("name", "required field")
	}
	if i.Sale < 0 {
		result.AddError("sale", "must be non-negative")
	}
	if strings.TrimSpace(i.Size) == "" {
		result.AddError("size", "required field")
	}
	if i.TotalPrice < 0 {
		result.AddError("total_price", "must be non-negative")
	}
	if i.NmID <= 0 {
		result.AddError("nm_id", "must be positive")
	}
	if strings.TrimSpace(i.Brand) == "" {
		result.AddError("brand", "required field")
	}
	if i.Status < 0 {
		result.AddError("status", "required field")
	}
	return result
}
