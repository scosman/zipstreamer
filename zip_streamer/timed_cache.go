package zip_streamer

import (
	"sync"
	"time"
)

type LinkCache struct {
	cache   sync.Map
	timeout *time.Duration
}

func NewLinkCache(timeout *time.Duration) LinkCache {
	return LinkCache{
		cache:   sync.Map{},
		timeout: timeout,
	}
}

func (c *LinkCache) Get(linkKey string) *ZipDescriptor {
	result, ok := c.cache.Load(linkKey)
	if ok {
		return result.(*ZipDescriptor)
	} else {
		return nil
	}
}

func (c *LinkCache) Set(linkKey string, descriptor *ZipDescriptor) {
	c.cache.Store(linkKey, descriptor)

	if c.timeout != nil {
		go func() {
			time.Sleep(*c.timeout)
			c.cache.Delete(linkKey)
		}()
	}
}
