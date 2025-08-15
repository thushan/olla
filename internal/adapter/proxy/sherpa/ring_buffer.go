package sherpa

import "sync"

// SimpleRingBuffer implements a thread-safe ring buffer for storing the last N bytes
// This is much simpler than hand-rolling complex buffer logic
type SimpleRingBuffer struct {
	data     []byte
	capacity int
	mu       sync.Mutex
}

// NewSimpleRingBuffer creates a new ring buffer with specified capacity
func NewSimpleRingBuffer(capacity int) *SimpleRingBuffer {
	return &SimpleRingBuffer{
		data:     make([]byte, 0, capacity),
		capacity: capacity,
	}
}

// Write adds data to the ring buffer, keeping only the last 'capacity' bytes
func (rb *SimpleRingBuffer) Write(p []byte) (n int, err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n = len(p)

	// If incoming data is larger than capacity, only keep the tail
	if n >= rb.capacity {
		rb.data = make([]byte, rb.capacity)
		copy(rb.data, p[n-rb.capacity:])
		return n, nil
	}

	// Calculate total size after append
	totalSize := len(rb.data) + n

	if totalSize <= rb.capacity {
		// Simple append, still under capacity
		rb.data = append(rb.data, p...)
	} else {
		// Need to drop old data to maintain capacity
		keepBytes := rb.capacity - n
		if keepBytes > 0 && len(rb.data) > keepBytes {
			// Shift existing data
			copy(rb.data[:keepBytes], rb.data[len(rb.data)-keepBytes:])
			rb.data = rb.data[:keepBytes]
		} else {
			rb.data = rb.data[:0]
		}
		rb.data = append(rb.data, p...)
	}

	return n, nil
}

// Bytes returns a copy of the current buffer contents
func (rb *SimpleRingBuffer) Bytes() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.data) == 0 {
		return nil
	}

	result := make([]byte, len(rb.data))
	copy(result, rb.data)
	return result
}

// Reset clears the buffer
func (rb *SimpleRingBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.data = rb.data[:0]
}

// Len returns the current size of data in the buffer
func (rb *SimpleRingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return len(rb.data)
}
