package main

import (
	"time"
)

type RateLimiter struct {
	ticker *time.Ticker
	done   chan struct{}
}

func NewRateLimiter(rps int) *RateLimiter {
	if rps <= 0 {
		rps = 2
	}
	rl := &RateLimiter{
		ticker: time.NewTicker(time.Second / time.Duration(rps)),
		done:   make(chan struct{}),
	}
	return rl
}

func (rl *RateLimiter) Wait() {
	<-rl.ticker.C
}

func (rl *RateLimiter) Stop() {
	rl.ticker.Stop()
	close(rl.done)
}
