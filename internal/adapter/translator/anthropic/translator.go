package anthropic

import (
	"bytes"

	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

// Translator converts between Anthropic and OpenAI API formats
// Uses buffer pooling to minimise memory allocations during translation
type Translator struct {
	logger     logger.StyledLogger
	bufferPool *pool.Pool[*bytes.Buffer]
}

// NewTranslator creates a new Anthropic translator instance
// Uses a buffer pool to reduce GC pressure during high-throughput operations
func NewTranslator(log logger.StyledLogger) *Translator {
	// Create buffer pool with 4KB initial capacity
	// This size fits most chat completions without reallocation
	bufferPool, err := pool.NewLitePool(func() *bytes.Buffer {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	})
	if err != nil {
		// This should never happen as the constructor is validated
		log.Error("Failed to create buffer pool", "error", err)
		panic("translator: failed to initialise buffer pool")
	}

	return &Translator{
		logger:     log,
		bufferPool: bufferPool,
	}
}

// Name returns the translator identifier
func (t *Translator) Name() string {
	return "anthropic"
}
