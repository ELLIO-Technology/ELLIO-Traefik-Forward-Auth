package logs

import (
	"sync"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/utils"
)

type LeakyBucket struct {
	capacity   int64
	tokens     int64
	refillRate int64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewLeakyBucket(capacity, refillRate int64) *LeakyBucket {
	return &LeakyBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (lb *LeakyBucket) Allow(tokens int64) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.refill()

	if lb.tokens >= tokens {
		lb.tokens -= tokens
		return true
	}

	return false
}

func (lb *LeakyBucket) AllowN(n int) bool {
	return lb.Allow(int64(n))
}

func (lb *LeakyBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(lb.lastRefill)
	tokensToAdd := int64(elapsed.Seconds() * float64(lb.refillRate))

	if tokensToAdd > 0 {
		lb.tokens = utils.MinInt64(lb.capacity, lb.tokens+tokensToAdd)
		lb.lastRefill = now
	}
}

func (lb *LeakyBucket) AvailableTokens() int64 {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.refill()
	return lb.tokens
}

func (lb *LeakyBucket) Capacity() int64 {
	return lb.capacity
}

func (lb *LeakyBucket) RefillRate() int64 {
	return lb.refillRate
}

func (lb *LeakyBucket) WaitTime(tokens int64) time.Duration {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.refill()

	if lb.tokens >= tokens {
		return 0
	}

	tokensNeeded := tokens - lb.tokens
	secondsToWait := float64(tokensNeeded) / float64(lb.refillRate)
	return time.Duration(secondsToWait * float64(time.Second))
}
