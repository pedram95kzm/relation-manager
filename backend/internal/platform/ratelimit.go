package platform

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type bucket struct {
	Count   int
	Expires time.Time
}

// Limiter is process-local: deploy one backend replica unless an edge/distributed limiter is added.
// Database quotas separately protect generation across concurrent requests and replicas.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	max     int
	window  time.Duration
	now     func() time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{buckets: make(map[string]bucket), max: max, window: window, now: time.Now}
}
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	_, exists := l.buckets[key]
	if !exists && len(l.buckets) >= 10000 {
		for k, v := range l.buckets {
			if !v.Expires.After(now) {
				delete(l.buckets, k)
			}
		}
		if len(l.buckets) >= 10000 {
			return false
		}
	}
	b := l.buckets[key]
	if !b.Expires.After(now) {
		b = bucket{Expires: now.Add(l.window)}
	}
	if b.Count >= l.max {
		return false
	}
	b.Count++
	l.buckets[key] = b
	return true
}
func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			c.Header("Retry-After", strconv.Itoa(int(l.window.Seconds())))
			Fail(c, Err(429, "rate_limited", "Too many requests. Try again later."))
			return
		}
		c.Next()
	}
}
