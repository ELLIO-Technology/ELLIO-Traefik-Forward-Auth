package logs

import (
	"sync"
)

type RingBuffer struct {
	buffer   []*AccessEvent
	capacity int
	head     int
	tail     int
	size     int
	mu       sync.Mutex
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buffer:   make([]*AccessEvent, capacity),
		capacity: capacity,
		head:     0,
		tail:     0,
		size:     0,
	}
}

func (rb *RingBuffer) Add(event *AccessEvent) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size >= rb.capacity {
		return false
	}

	rb.buffer[rb.tail] = event
	rb.tail = (rb.tail + 1) % rb.capacity
	rb.size++

	return true
}

func (rb *RingBuffer) Get() (*AccessEvent, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		return nil, false
	}

	event := rb.buffer[rb.head]
	rb.buffer[rb.head] = nil
	rb.head = (rb.head + 1) % rb.capacity
	rb.size--

	return event, true
}

func (rb *RingBuffer) Drain(maxItems int) []*AccessEvent {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		return nil
	}

	count := rb.size
	if count > maxItems {
		count = maxItems
	}

	events := make([]*AccessEvent, 0, count)

	for i := 0; i < count; i++ {
		event := rb.buffer[rb.head]
		rb.buffer[rb.head] = nil
		rb.head = (rb.head + 1) % rb.capacity
		rb.size--
		events = append(events, event)
	}

	return events
}

func (rb *RingBuffer) DrainAll() []*AccessEvent {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		return nil
	}

	events := make([]*AccessEvent, 0, rb.size)

	for rb.size > 0 {
		event := rb.buffer[rb.head]
		rb.buffer[rb.head] = nil
		rb.head = (rb.head + 1) % rb.capacity
		rb.size--
		events = append(events, event)
	}

	return events
}

func (rb *RingBuffer) Size() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size
}

func (rb *RingBuffer) Capacity() int {
	return rb.capacity
}

func (rb *RingBuffer) IsFull() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size >= rb.capacity
}

func (rb *RingBuffer) IsEmpty() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size == 0
}
