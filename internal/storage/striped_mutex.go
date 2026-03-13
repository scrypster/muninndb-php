package storage

import (
	"hash/fnv"
	"sync"
)

const lockStripes = 256

// stripedMutex is a fixed-size array of mutexes for lock striping.
// Replaces unbounded sync.Map-based lock pools. Memory usage is constant at
// lockStripes × sizeof(sync.Mutex) ≈ 6 KB regardless of key count.
//
// Two different keys may map to the same mutex stripe (false sharing), but at
// 256 stripes this is negligible in practice and safe — contention causes
// brief waiting, never data corruption.
type stripedMutex struct {
	mu [lockStripes]sync.Mutex
}

// For returns the mutex for the given byte key using FNV-32a hashing.
func (s *stripedMutex) For(key []byte) *sync.Mutex {
	h := fnv.New32a()
	h.Write(key)
	return &s.mu[h.Sum32()%lockStripes]
}
