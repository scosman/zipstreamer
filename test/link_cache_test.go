package testing

import (
	"testing"
	"time"

	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
)

func TestLinkCache(t *testing.T) {
	cache := zip_streamer.NewLinkCache(nil)
	if cache.Get("a") != nil {
		t.Fatal("cache not empty")
	}
	r := zip_streamer.NewZipDescriptor()
	cache.Set("a", r)
	if cache.Get("a") == nil {
		t.Fatal("cache didn't store entry")
	}
}

func TestLinkCacheTimeout(t *testing.T) {
	timeout := time.Millisecond * 30
	cache := zip_streamer.NewLinkCache(&timeout)
	cache.Set("a", zip_streamer.NewZipDescriptor())
	if cache.Get("a") == nil {
		t.Fatal("cache didn't store entry")
	}
	time.Sleep(timeout * 2)
	if cache.Get("a") != nil {
		t.Fatal("cache not cleared on timeout")
	}
}
