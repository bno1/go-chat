package internal

import (
	"fmt"
	"math"
	"time"
)

type TokenBucket struct {
	// Max tokens to hold
	capacity uint

	// Current number of tokens in bucket
	tokens uint

	// How many tokens are generated per second
	tokens_per_sec float64

	// Leftover fractional tokens from the last call to Update
	residue float64
}

func NewTokenBucket(
	capacity uint,
	inital_tokens uint,
	tokens_per_sec float64,
) (TokenBucket, error) {
	bucket := TokenBucket{
		capacity:       capacity,
		tokens:         inital_tokens,
		tokens_per_sec: tokens_per_sec,
		residue:        0.0,
	}

	if inital_tokens > capacity {
		return bucket, fmt.Errorf("inital_tokens exceeds capacity")
	}

	if tokens_per_sec < 0 {
		return bucket, fmt.Errorf("tokens_per_sec cannot be negative")
	}

	return bucket, nil
}

func (bucket *TokenBucket) UpdateParams(
	capacity uint,
	tokens_per_sec float64,
) {
	bucket.capacity = capacity
	bucket.tokens_per_sec = tokens_per_sec

	// Limit tokens to new capacity
	if bucket.tokens > capacity {
		bucket.tokens = capacity
	}

	// Discard residue if the bucket is full
	if bucket.tokens >= capacity {
		bucket.residue = 0.0
	}
}

func (bucket *TokenBucket) Update(delta_time time.Duration) {
	// Compute how many tokens should be added during delta_time
	// new_tokens_f = whole tokens as float
	// new_residue = fractional residue tokens
	new_tokens_f, new_residue := math.Modf(
		delta_time.Seconds()*bucket.tokens_per_sec + bucket.residue)

	new_tokens := uint(new_tokens_f)

	// This will be 0 when bucket is full
	max_new_tokens := bucket.capacity - bucket.tokens

	// Limit new_tokens to max_new_tokens to avoid integer overflow problems
	if new_tokens > max_new_tokens {
		new_tokens = max_new_tokens
	}

	// Check if residue should be saved
	if new_tokens < max_new_tokens {
		// Save residue for the next call
		bucket.residue = new_residue
	} else {
		// Reached bucket capacity, don't save residue
		bucket.residue = 0
	}

	// Add tokens
	bucket.tokens += new_tokens
}

func (bucket *TokenBucket) TryConsumeToken() bool {
	if bucket.tokens > 0 {
		bucket.tokens -= 1
		return true
	}

	return false
}
