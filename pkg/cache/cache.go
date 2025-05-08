package cache

import (
	"sync"
	"time"

	"ai-agent-character-demo/backend/pkg/config"
)

// Item represents a cached item with expiration
type Item struct {
	Value      interface{}
	Expiration int64
}

// Expired checks if the cache item has expired
func (item Item) Expired() bool {
	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

// Cache is a thread-safe in-memory cache with expiration
type Cache struct {
	items             map[string]Item
	mu                sync.RWMutex
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
	maxItems          int
	onEvicted         func(string, interface{})
}

// NewCache creates a new cache with the given default expiration and cleanup interval
func NewCache() *Cache {
	cfg := config.Get()

	defaultExpiration := cfg.Cache.TTL
	cleanupInterval := cfg.Cache.PurgeWindow
	maxItems := cfg.Cache.MaxSize

	cache := &Cache{
		items:             make(map[string]Item),
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
		maxItems:          maxItems,
	}

	// Start cleanup goroutine if cleanup interval > 0
	if cleanupInterval > 0 {
		go cache.startCleanupTimer()
	}

	return cache
}

// Set adds an item to the cache with the default expiration
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithExpiration(key, value, c.defaultExpiration)
}

// SetWithExpiration adds an item to the cache with a specific expiration time
func (c *Cache) SetWithExpiration(key string, value interface{}, d time.Duration) {
	var exp int64
	if d > 0 {
		exp = time.Now().Add(d).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict an item first
	if len(c.items) >= c.maxItems && c.maxItems > 0 && c.items[key].Value == nil {
		c.evictOldest()
	}

	c.items[key] = Item{
		Value:      value,
		Expiration: exp,
	}
}

// Get retrieves an item from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	// Check if the item has expired
	if item.Expired() {
		return nil, false
	}

	return item.Value, true
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, found := c.items[key]; found && c.onEvicted != nil {
		c.onEvicted(key, item.Value)
	}

	delete(c.items, key)
}

// Flush removes all items from the cache
func (c *Cache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Call onEvicted for each item if it exists
	if c.onEvicted != nil {
		for k, v := range c.items {
			c.onEvicted(k, v.Value)
		}
	}

	c.items = make(map[string]Item)
}

// Count returns the number of items in the cache (including expired items)
func (c *Cache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// SetOnEvicted sets the callback to be called when an item is evicted
func (c *Cache) SetOnEvicted(f func(string, interface{})) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.onEvicted = f
}

// startCleanupTimer starts the cleanup ticker
func (c *Cache) startCleanupTimer() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.deleteExpired()
		}
	}
}

// deleteExpired deletes all expired items from the cache
func (c *Cache) deleteExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()
	for k, v := range c.items {
		if v.Expiration > 0 && now > v.Expiration {
			// Call onEvicted if it exists
			if c.onEvicted != nil {
				c.onEvicted(k, v.Value)
			}

			delete(c.items, k)
		}
	}
}

// evictOldest finds and removes the oldest item in the cache
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime int64

	// Find the oldest item
	firstRun := true
	for k, v := range c.items {
		if firstRun || v.Expiration < oldestTime || oldestTime == 0 {
			oldestKey = k
			oldestTime = v.Expiration
			firstRun = false
		}
	}

	// If we found an item to evict and have an eviction callback, call it
	if oldestKey != "" && c.onEvicted != nil {
		c.onEvicted(oldestKey, c.items[oldestKey].Value)
	}

	// Delete the oldest item
	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}
