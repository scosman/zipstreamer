package zip_streamer

import (
  "time"
  "sync"
)

type LinkCache struct {
  cache sync.Map
  timeout *time.Duration
}

func NewLinkCache(timeout *time.Duration) (LinkCache) {
  return LinkCache{
    cache: sync.Map{},
    timeout: timeout,
  }
}

func (c* LinkCache) Get(linkKey string) (entries []*FileEntry) {
  result, ok := c.cache.Load(linkKey)
  if ok {
    return result.([]*FileEntry)
  } else {
    return nil
  }
}

func (c* LinkCache) Set(linkKey string, entries []*FileEntry) {
  c.cache.Store(linkKey, entries)

  if c.timeout != nil {
    go func() {
      time.Sleep(*c.timeout)
      c.cache.Delete(linkKey)
    }()
  }
}

