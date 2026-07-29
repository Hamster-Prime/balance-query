package cache

import (
	"testing"
	"time"
)

func TestExpiredEntryIsRemoved(t *testing.T) {
	c := NewWithLimit[string, string](time.Millisecond, 4)
	c.Set("expired", "value")
	time.Sleep(5 * time.Millisecond)
	if _, ok := c.Get("expired"); ok {
		t.Fatal("expired entry was returned")
	}
	if keys := c.Keys(); len(keys) != 0 {
		t.Fatalf("expired keys were retained: %#v", keys)
	}
}

func TestZeroTTLDoesNotStore(t *testing.T) {
	c := NewWithLimit[string, string](0, 4)
	c.Set("key", "value")
	if _, ok := c.Get("key"); ok {
		t.Fatal("cache with zero TTL stored an entry")
	}
}

func TestCacheEvictsEarliestExpiryAtCapacity(t *testing.T) {
	c := NewWithLimit[string, string](time.Hour, 2)
	c.Set("first", "one")
	time.Sleep(time.Millisecond)
	c.Set("second", "two")
	c.Set("third", "three")
	if _, ok := c.Get("first"); ok {
		t.Fatal("earliest-expiring entry was not evicted")
	}
	for _, key := range []string{"second", "third"} {
		if _, ok := c.Get(key); !ok {
			t.Fatalf("entry %q was unexpectedly evicted", key)
		}
	}
}

func TestResetUpdatesTTLAndClearsEntries(t *testing.T) {
	c := NewWithLimit[string, string](time.Hour, 4)
	c.Set("old", "value")
	c.Reset(0)
	if _, ok := c.Get("old"); ok {
		t.Fatal("Reset retained an old entry")
	}
	c.Set("new", "value")
	if _, ok := c.Get("new"); ok {
		t.Fatal("Reset did not update the TTL for future writes")
	}
}
