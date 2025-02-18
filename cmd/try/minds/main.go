package main

/*
import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/handlers"
	"github.com/chriscow/minds/middleware"
	"github.com/chriscow/minds/middleware/retry"
	"github.com/chriscow/minds/providers/gemini"
	"github.com/joho/godotenv"
	"golang.org/x/exp/rand"
)

type SimplificationType string

const (
	C1   SimplificationType = "c1"
	B2   SimplificationType = "b2"
	B1   SimplificationType = "b1"
	A2A1 SimplificationType = "a2a1"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	if err := run(); err != nil {
		log.Fatalf("Error running flow: %v", err)
	}
}

func run() error {
	persist := NewPersistenceMiddleware("persist", false, false)


	llm, err := gemini.NewProvider(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create Gemini provider: %w", err)
	}

	// Flow Steps
	preprocessingStep := NewPreprocessing("preprocessing", false)
	readabilityStep := NewReadability("readability", false)

	retryCriteria := func(tc minds.ThreadContext, attempt int, err error) bool {
		currentLanguageLevel, exists := tc.Metadata()["current_language_level"]
		if !exists {
			return false
		}
		targetLanguageLevel, exists := tc.Metadata()["target_language_level"]
		if !exists {
			return false
		}

		// Retry if current_language_level is not target_language_level
		return currentLanguageLevel != targetLanguageLevel
	}

	retry := middleware.Retry("retry",
		retry.WithAttempts(3),
		retry.WithRetryCriteria(retryCriteria),
	)

	c1 := NewSimplification(llm,C1,"c1")
	b2 := NewSimplification(llm, B2, "b2")
	b1 := NewSimplification(llm, B1, "b1")
	a2a1 := NewSimplification(llm, A2A1, "a2a1")

	translate := NewTranslation(llm, _translation, "english")


	// This is the condition that determines if a text should be translated to English
	shouldTx := func(tc minds.ThreadContext) (bool, error) {
		detectedLanguage, exists := tc.Metadata()["detected_language"]
		if !exists {
			return false, nil
		}

		return detectedLanguage != "english", nil
	}

	txToEnglish := handlers.NewIf("translate-to-english", shouldTx, englishTxStep, nil)


	// Preprocess, translate and evaluate readability
	preprocessor := handlers.NewSequence("workflow", preprocessingStep, txToEnglish, readabilityStep)



	// Simplification steps
	progress := []SimplificationType{C1, B2, B1, A2A1}

	workflow := handlers.NewRange("simplify", simplifyHandler, progress)

	// Build the flow with conditional branching based on readability
	// Sequential handlers are executed in order, while Switch handlers
	// allow for conditional branching based on metadata.
	flow.Handle(
		handlers.Sequential(
			"main",
			preprocessingStep,
			txToEnglish,
			readabilityStep,
			handlers.Switch(
				"branch",
				defaultHandler{},
				handlers.SwitchCase{
					Condition: handlers.MetadataEquals{
						Key:   "current_language_level",
						Value: "c2",
					},
					Handler: handlers.Sequential(
						"c2_flow",
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "c1",
						}),
						handlers.ConditionalRetry(
							"c2_flow_c1_retry",
							3, // Max retries
							handlers.Sequential("c2_flow_c1_retry_flow", c1Step, readabilityStep),
						,	)handlers.ParallelWithFocus(
							"c2_flow_c1_storage",
							handlers.NewMetadataUpdater(map[string]any{
								"target_language_level": "b2",
							}),
						),
						handlers.ConditionalRetry(
							"c2_flow_b2_retry",
							3, // Max retries
							handlers.Sequential("c2_flow_b2_retry_flow", b2SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "b1",
						}),
						handlers.ConditionalRetry(
							"c2_flow_b1_retry",
							3, // Max retries
							handlers.Sequential("c2_flow_b1_retry_flow", b1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "a2a1",
						}),
						handlers.ConditionalRetry(
							"c2_flow_a2a1_retry",
							3, // Max retries
							handlers.Sequential("c2_flow_a2a1_retry_flow", a2a1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
					),
				},
				handlers.SwitchCase{
					Condition: handlers.MetadataEquals{
						Key:   "current_language_level",
						Value: "c1",
					},
					Handler: handlers.Sequential(
						"c1_flow",
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "b2",
						}),
						handlers.ConditionalRetry(
							"c1_flow_b2_retry",
							3, // Max retries
							handlers.Sequential("c1_flow_b2_retry_flow", b2SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "b1",
						}),
						handlers.ConditionalRetry(
							"c1_flow_b1_retry",
							3, // Max retries
							handlers.Sequential("c1_flow_b1_retry_flow", b1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "a2a1",
						}),
						handlers.ConditionalRetry(
							"c1_flow_a2a1_retry",
							3, // Max retries
							handlers.Sequential("c1_flow_a2a1_retry_flow", a2a1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
					),
				},
				handlers.SwitchCase{
					Condition: handlers.MetadataEquals{
						Key:   "current_language_level",
						Value: "b2",
					},
					Handler: handlers.Sequential(
						"b2_flow",
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "b1",
						}),
						handlers.ConditionalRetry(
							"b2_flow_b1_retry",
							3, // Max retries
							handlers.Sequential("b2_flow_b1_retry_flow", b1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "a2a1",
						}),
						handlers.ConditionalRetry(
							"b2_flow_a2a1_retry",
							3, // Max retries
							handlers.Sequential("b2_flow_a2a1_retry_flow", a2a1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
					),
				},
				handlers.SwitchCase{
					Condition: handlers.MetadataEquals{
						Key:   "current_language_level",
						Value: "b1",
					},
					Handler: handlers.Sequential(
						"b1_flow",
						handlers.NewMetadataUpdater(map[string]any{
							"target_language_level": "a2a1",
						}),
						handlers.ConditionalRetry(
							"b1_flow_a2a1_retry",
							3, // Max retries
							handlers.Sequential("b1_flow_a2a1_retry_flow", a2a1SimplificationStep, readabilityStep),
							conditionalRetryFn,
						),
					),
				},
			),
		),
	)

	// Execute the flow
	ctx := context.Background()
	tc := minds.NewThreadContext(ctx)
	tc.AppendMessages(minds.Message{
		Role:    minds.RoleUser,
		Content: "Your complex text here...",
	})

	// result, err := flow.HandleThread(tc, nil)
	if err != nil {
		panic(err)
	}

	return nil
}

// PersistenceMiddleware simulates data persistence operations.
type PersistenceMiddleware struct {
	name       string
	store      map[string]string
	mu         sync.Mutex
	failOnSave bool // Simulate failure during saving
	failOnLoad bool // Simulate failure during loading
}

// NewPersistenceMiddleware creates a mock persistence middleware.
//
// Options:
//   - `failOnSave`: Simulates failure when saving data.
//   - `failOnLoad`: Simulates failure when loading data.
//
// Example:
// ```go
// pm := NewPersistenceMiddleware("mock-db", false, true) // Fail on load, succeed on save
// flow.Use(pm)
// ```
func NewPersistenceMiddleware(name string, failOnSave, failOnLoad bool) minds.Middleware {
	return &PersistenceMiddleware{
		name:       name,
		store:      make(map[string]string),
		failOnSave: failOnSave,
		failOnLoad: failOnLoad,
	}
}

// Wrap applies the persistence middleware to a handler.
func (p *PersistenceMiddleware) Wrap(next minds.ThreadHandler) minds.ThreadHandler {
	return &persistenceHandler{
		persistence: p,
		next:        next,
	}
}

// persistenceHandler wraps a handler and injects persistence logic.
type persistenceHandler struct {
	persistence *PersistenceMiddleware
	next        minds.ThreadHandler
}

// HandleThread simulates persistence operations during thread execution.
func (ph *persistenceHandler) HandleThread(tc minds.ThreadContext, _ minds.ThreadHandler) (minds.ThreadContext, error) {
	key := "persisted_key"

	// Simulate loading data
	ph.persistence.mu.Lock()
	value, exists := ph.persistence.store[key]
	ph.persistence.mu.Unlock()

	if !exists && ph.persistence.failOnLoad {
		return tc, fmt.Errorf("%s: failed to load data", ph.persistence.name)
	}

	if exists {
		tc.SetKeyValue("loaded_data", value)
	}

	// Execute the next handler
	result, err := ph.next.HandleThread(tc, nil)
	if err != nil {
		return result, err
	}

	// Simulate saving data
	if ph.persistence.failOnSave {
		return result, fmt.Errorf("%s: failed to save data", ph.persistence.name)
	}

	ph.persistence.mu.Lock()
	ph.persistence.store[key] = "mock_value"
	ph.persistence.mu.Unlock()

	return result, nil
}

// PreprocessingHandler simulates preprocessing before executing the next handler.
type PreprocessingHandler struct {
	name          string
	failOnProcess bool // If true, simulates a failure
}

// NewPreprocessing creates a mock preprocessing handler.
//
// Options:
//   - `failOnProcess`: If true, it simulates a failure before calling the next handler.
//
// Example:
// ```go
// preprocess := NewPreprocessing("sanitize-input", false) // Normal execution
// handlerChain.Use(preprocess)
//
// failingPreprocess := NewPreprocessing("failing-preprocess", true) // Simulates failure
// handlerChain.Use(failingPreprocess)
// ```
func NewPreprocessing(name string, failOnProcess bool) minds.ThreadHandler {
	return &PreprocessingHandler{
		name:          name,
		failOnProcess: failOnProcess,
	}
}

// HandleThread applies preprocessing before executing the next handler.
func (p *PreprocessingHandler) HandleThread(tc minds.ThreadContext, next minds.ThreadHandler) (minds.ThreadContext, error) {
	// Simulate preprocessing failure if enabled
	if p.failOnProcess {
		return tc, fmt.Errorf("%s: preprocessing failed", p.name)
	}

	// Add metadata for preprocessing tracking
	meta := tc.Metadata()
	meta["preprocessing"] = fmt.Sprintf("%s completed", p.name)
	tc = tc.WithMetadata(meta)

	// Call the next handler if available
	if next != nil {
		return next.HandleThread(tc, nil)
	}
	return tc, nil
}


func NewReadability(name string, failOnAnalyze bool) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, next minds.ThreadHandler) (minds.ThreadContext, error) {
		// Simulate analysis failure if enabled
		if r.failOnAnalyze {
			return tc, fmt.Errorf("%s: readability analysis failed", r.name)
		}

		// Simulate readability metrics
		readabilityScore := rand.Float64()*50 + 50 // Random score between 50-100
		complexWords := rand.Intn(10) + 5          // Random complex words count

		// Add analysis results to metadata
		meta := tc.Metadata()
		meta["readability_score"] = readabilityScore
		meta["complex_words"] = complexWords
		meta["readability"] = fmt.Sprintf("Score: %.2f, Complex Words: %d", readabilityScore, complexWords)
		tc = tc.WithMetadata(meta)

		// Call the next handler if available
		if next != nil {
			return next.HandleThread(tc, nil)
		}
		return tc, nil
	})
}

// NewSimplification creates a mock handler that processes simplification steps
func NewSimplification(llm minds.ContentGenerator, st SimplificationType, simplificationType string) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, next minds.ThreadHandler) (minds.ThreadContext, error) {
		// Example of processing simplification logic
		fmt.Printf("Processing simplification: %s with type %s\n", simplificationType, st)

		// Here, you can add the actual logic for simplification based on the arguments
		// This is just a mock for demonstration purposes.

		// Continue with the next handler if provided
		if next != nil {
			return next.HandleThread(tc, nil)
		}

		return tc, nil
	})
}

func NewTranslation(llm minds.ContentGenerator, st SimplificationType, language string) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, next minds.ThreadHandler) (minds.ThreadContext, error) {
		// Example of processing translation logic
		fmt.Printf("translating to english\n")

		// Here, you can add the actual logic for translation
		// This is just a mock for demonstration purposes.

		// Continue with the next handler if provided
		if next != nil {
			return next.HandleThread(tc, nil)
		}

		return tc, nil
	})
}
*/
