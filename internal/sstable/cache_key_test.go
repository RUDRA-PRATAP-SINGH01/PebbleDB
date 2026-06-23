package sstable

import "testing"

func TestBlockCacheKeyNoCollisionAcrossLargeOffsets(t *testing.T) {
	k1 := makeBlockCacheKey(1, 0)
	k2 := makeBlockCacheKey(1, 1<<32)
	if k1 == k2 {
		t.Fatal("cache keys must differ when offset differs by 4GiB")
	}
	k3 := makeBlockCacheKey(2, 0)
	if k1 == k3 {
		t.Fatal("cache keys must differ for different file IDs")
	}
}
