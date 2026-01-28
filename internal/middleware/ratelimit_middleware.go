package middleware

import (
    "sync"
    "time"
)

// Rate limiter ONLY for invalid auth attempts
type InvalidAuthRateLimiter struct {
    mu       sync.Mutex
    attempts map[string]*attemptInfo
}

type attemptInfo struct {
    count   int
    firstAt time.Time
}

func NewInvalidAuthRateLimiter() *InvalidAuthRateLimiter {
    rl := &InvalidAuthRateLimiter{
        attempts: make(map[string]*attemptInfo),
    }
    go rl.cleanup()
    return rl
}

// Allow checks if IP can make another attempt
// Limit: 5 attempts per minute
func (r *InvalidAuthRateLimiter) Allow(ip string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    info, exists := r.attempts[ip]
    if !exists {
        r.attempts[ip] = &attemptInfo{count: 1, firstAt: now}
        return true
    }

    // Reset if window expired
    if now.Sub(info.firstAt) > time.Minute {
        r.attempts[ip] = &attemptInfo{count: 1, firstAt: now}
        return true
    }

    if info.count >= 5 {
        return false
    }
    info.count++
    return true
}

func (r *InvalidAuthRateLimiter) cleanup() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        r.mu.Lock()
        now := time.Now()
        for ip, info := range r.attempts {
            if now.Sub(info.firstAt) > time.Minute {
                delete(r.attempts, ip)
            }
        }
        r.mu.Unlock()
    }
}
