package internal

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	tools "talent-rodeo/internal/llmtools"
)

const (
	costTransactionsCollection = "cost_transactions"

	SystemUserID = "00000000-0000-0000-0000-000000000000"
)

// CostTransactionType represents different types of cost transactions
type CostTransactionType string

const (
	CostTypeLLM       CostTransactionType = "llm"
	CostTypeTelephony CostTransactionType = "telephony"
	CostTypeEmail     CostTransactionType = "email"
	CostTypeStorage   CostTransactionType = "storage"
	CostTypeOther     CostTransactionType = "other"
)

// CostTransaction represents a single cost transaction for tracking expenses
type CostTransaction struct {
	ID          string
	UserID      string
	Type        CostTransactionType
	Description string
	CostUSD     float64
	CreatedAt   time.Time

	// LLM-specific fields (optional)
	Model            *string
	PromptTokens     *int
	CompletionTokens *int
	DiscountApplied  *bool
	DiscountPercent  *int

	// Feature/context tracking (optional)
	FeatureArea *string
	JobID       *string

	// Other cost-specific fields (optional)
	Quantity *float64
	Unit     *string
}

// ModelPricing represents the pricing structure for an LLM model
type ModelPricing struct {
	InputCostPer1M       float64 // Cost per 1 million input tokens
	OutputCostPer1M      float64 // Cost per 1 million output tokens
	CachedInputCostPer1M float64 // Cost per 1 million cached input tokens (if applicable)
}

// GetModelPricing returns the pricing information for a given model
func GetModelPricing(model string) (ModelPricing, error) {
	pricing := map[string]ModelPricing{
		tools.GPT6Luna: {
			InputCostPer1M:       0.10,
			OutputCostPer1M:      0.50,
			CachedInputCostPer1M: 0.01,
		},
		tools.GPT41Nano: {
			InputCostPer1M:       0.10,
			OutputCostPer1M:      0.40,
			CachedInputCostPer1M: 0.025,
		},
		"gpt-4.1-mini": {
			InputCostPer1M:       0.40,
			OutputCostPer1M:      1.60,
			CachedInputCostPer1M: 0.10,
		},
		tools.GPT41: {
			InputCostPer1M:       2.00,
			OutputCostPer1M:      8.00,
			CachedInputCostPer1M: 0.50,
		},
		tools.GPT4oMini: {
			InputCostPer1M:       0.15,
			OutputCostPer1M:      0.60,
			CachedInputCostPer1M: 0.075,
		},
		tools.O4Mini: {
			InputCostPer1M:       1.10,
			OutputCostPer1M:      4.40,
			CachedInputCostPer1M: 0.275,
		},
		tools.O3Mini: {
			InputCostPer1M:       1.10,
			OutputCostPer1M:      4.40,
			CachedInputCostPer1M: 0.55,
		},
		tools.DeepSeekFlash: {
			InputCostPer1M:       0.30,
			OutputCostPer1M:      1.20,
			CachedInputCostPer1M: 0.006,
		},
		"deepseek-chat": {
			InputCostPer1M:       0.27,
			OutputCostPer1M:      1.10,
			CachedInputCostPer1M: 0.07,
		},
		tools.DeepSeekReasoner: {
			InputCostPer1M:       0.55,
			OutputCostPer1M:      2.19,
			CachedInputCostPer1M: 0.14,
		},
	}

	if p, exists := pricing[model]; exists {
		return p, nil
	}

	return ModelPricing{}, fmt.Errorf("unknown model: %s", model)
}

// IsDiscountPeriod checks if the given time falls within a discount period for the model
func IsDiscountPeriod(model string, t time.Time) (bool, int) {
	// Convert to UTC for discount time checking
	utc := t.UTC()
	hour := utc.Hour()
	if model == tools.DeepSeekFlash {
		// Flash off-peak is outside weekday 01:00-04:00 and 06:00-10:00 UTC.
		// DeepSeek also treats Chinese public holidays as off-peak; those dates
		// cannot be identified from the timestamp alone.
		weekday := utc.Weekday()
		peak := weekday != time.Saturday && weekday != time.Sunday &&
			((hour >= 1 && hour < 4) || (hour >= 6 && hour < 10))
		if !peak {
			return true, 50
		}
		return false, 0
	}

	// Historical DeepSeek Chat and Reasoner prices used the old UTC window.
	if model == "deepseek-chat" || model == tools.DeepSeekReasoner {
		// Between 16:30 (4:30 PM) and 23:59 (11:59 PM) same day
		// OR between 00:00 (midnight) and 00:30 (12:30 AM) next day
		isInDiscountPeriod := (hour >= 16 && (utc.Minute() >= 30 || hour > 16)) ||
			(hour == 0 && utc.Minute() <= 30)

		if isInDiscountPeriod {
			switch model {
			case "deepseek-chat":
				return true, 50 // 50% off
			case tools.DeepSeekReasoner:
				return true, 75 // 75% off
			}
		}
	}

	return false, 0
}

// CalculateLLMCost calculates the cost of an LLM call including any applicable discounts
func CalculateLLMCost(model string, promptTokens, completionTokens int, callTime time.Time) (float64, bool, int, error) {
	pricing, err := GetModelPricing(model)
	if err != nil {
		return 0, false, 0, err
	}

	// Calculate base cost
	inputCost := (float64(promptTokens) / 1_000_000) * pricing.InputCostPer1M
	outputCost := (float64(completionTokens) / 1_000_000) * pricing.OutputCostPer1M
	totalCost := inputCost + outputCost

	// Check for discount
	discountApplied, discountPercent := IsDiscountPeriod(model, callTime)
	if discountApplied {
		totalCost = totalCost * (1.0 - float64(discountPercent)/100.0)
	}

	return totalCost, discountApplied, discountPercent, nil
}

// CreateCostTransaction records a cost transaction in Firestore
func CreateCostTransaction(ctx context.Context, fs *FirestoreClient, transaction CostTransaction) error {
	if fs == nil {
		return fmt.Errorf("firestore client cannot be nil")
	}

	if transaction.ID == "" {
		transaction.ID = uuid.New().String()
	}

	if transaction.CreatedAt.IsZero() {
		transaction.CreatedAt = time.Now()
	}

	_, err := fs.Collection(costTransactionsCollection).Doc(transaction.ID).Set(ctx, transaction)
	if err != nil {
		return fmt.Errorf("failed to create cost transaction: %w", err)
	}

	return nil
}

// RecordLLMCost records the cost of an LLM call
func RecordLLMCost(ctx context.Context, fs *FirestoreClient, userID *string, model string, response openai.ChatCompletionResponse, featureArea string, jobID *string) error {
	if response.Usage.PromptTokens == 0 && response.Usage.CompletionTokens == 0 {
		// No tokens used, no cost to record
		return nil
	}

	if fs == nil {
		return nil // noop - we don't want to fail the operation
	}

	costUserID := SystemUserID
	if userID != nil {
		costUserID = *userID
	}

	callTime := time.Now()
	cost, discountApplied, discountPercent, err := CalculateLLMCost(
		model,
		response.Usage.PromptTokens,
		response.Usage.CompletionTokens,
		callTime,
	)
	if err != nil {
		return fmt.Errorf("failed to calculate LLM cost: %w", err)
	}
	if response.Usage.PromptTokensDetails != nil {
		cachedTokens := response.Usage.PromptTokensDetails.CachedTokens
		if cachedTokens > response.Usage.PromptTokens {
			cachedTokens = response.Usage.PromptTokens
		}
		if cachedTokens > 0 {
			pricing, err := GetModelPricing(model)
			if err != nil {
				return fmt.Errorf("failed to price cached input: %w", err)
			}
			savings := float64(cachedTokens) / 1_000_000 *
				(pricing.InputCostPer1M - pricing.CachedInputCostPer1M)
			if discountApplied {
				savings *= 1 - float64(discountPercent)/100
			}
			cost -= savings
		}
	}

	description := fmt.Sprintf("LLM call - %s (%d input + %d output tokens)",
		model, response.Usage.PromptTokens, response.Usage.CompletionTokens)

	transaction := CostTransaction{
		UserID:           costUserID,
		Type:             CostTypeLLM,
		Description:      description,
		CostUSD:          cost,
		Model:            &model,
		PromptTokens:     &response.Usage.PromptTokens,
		CompletionTokens: &response.Usage.CompletionTokens,
		DiscountApplied:  &discountApplied,
		DiscountPercent:  &discountPercent,
		FeatureArea:      &featureArea,
		JobID:            jobID,
		CreatedAt:        callTime,
	}

	return CreateCostTransaction(ctx, fs, transaction)
}

// RecordTelephonyCost records the cost of a telephony operation
func RecordTelephonyCost(ctx context.Context, fs *FirestoreClient, userID string, costUSD float64, minutes float64, description string) error {
	unit := "minutes"
	transaction := CostTransaction{
		UserID:      userID,
		Type:        CostTypeTelephony,
		Description: description,
		CostUSD:     costUSD,
		Quantity:    &minutes,
		Unit:        &unit,
		FeatureArea: stringPtr("telephony"),
	}

	return CreateCostTransaction(ctx, fs, transaction)
}

// GetUserCosts retrieves cost transactions for a user within a time range
func GetUserCosts(ctx context.Context, fs *FirestoreClient, userID string, startTime, endTime time.Time) ([]CostTransaction, error) {
	if fs == nil {
		return nil, fmt.Errorf("firestore client cannot be nil")
	}

	docs, err := fs.Collection(costTransactionsCollection).
		Where("user_id", "==", userID).
		Where("created_at", ">=", startTime).
		Where("created_at", "<=", endTime).
		OrderBy("created_at", firestore.Desc).
		Documents(ctx).GetAll()

	if err != nil {
		return nil, fmt.Errorf("failed to get user cost transactions: %w", err)
	}

	var transactions []CostTransaction
	for _, doc := range docs {
		var transaction CostTransaction
		if err := doc.DataTo(&transaction); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cost transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}

	return transactions, nil
}

// GetUserTotalCost calculates the total cost for a user within a time range
func GetUserTotalCost(ctx context.Context, fs *FirestoreClient, userID string, startTime, endTime time.Time) (float64, error) {
	transactions, err := GetUserCosts(ctx, fs, userID, startTime, endTime)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, transaction := range transactions {
		total += transaction.CostUSD
	}

	return total, nil
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
