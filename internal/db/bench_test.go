package db

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

const (
	benchValueLen          = 128
	benchCompactionHoldoff = 100
)

var benchPayload = func() []byte {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	v := make([]byte, benchValueLen)
	for i := range v {
		v[i] = alphabet[i%len(alphabet)]
	}
	return v
}()

func makeOrderedKeys(n int) [][]byte {
	keys := make([][]byte, n)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("key-%010d", i))
	}
	return keys
}

// benchPreload fills the DB and waits until all records are visible to Get/Scan.
// Async group commit may leave records in the flusher goroutine after pendingBatch drains.
func benchPreload(b *testing.B, opts Options, dataset int) (*DB, [][]byte) {
	b.Helper()

	dir := b.TempDir()
	opts.Dir = dir
	if opts.MemtableSize == 0 {
		opts.MemtableSize = 128 << 20
	}
	if opts.CompactionThreshold == 0 {
		opts.CompactionThreshold = benchCompactionHoldoff
	}

	db, err := Open(opts)
	if err != nil {
		b.Fatalf("open: %v", err)
	}

	keys := makeOrderedKeys(dataset)
	val := benchPayload
	for i := 0; i < dataset; i++ {
		if err := db.Put(keys[i], val); err != nil {
			b.Fatalf("put %d: %v", i, err)
		}
	}
	// Route through the batch flusher so in-flight WAL batches finish applying.
	if err := db.awaitBatchPersist(); err != nil {
		b.Fatalf("await batch: %v", err)
	}
	if _, err := db.Get(keys[0]); err != nil {
		b.Fatalf("verify first key: %v", err)
	}
	if _, err := db.Get(keys[dataset-1]); err != nil {
		b.Fatalf("verify last key: %v", err)
	}
	return db, keys
}

func reportThroughput(b *testing.B, ops int, bytesPerOp int) {
	secs := b.Elapsed().Seconds()
	if secs <= 0 {
		return
	}
	b.ReportMetric(float64(ops)/secs, "ops/sec")
	if bytesPerOp > 0 {
		b.ReportMetric(float64(ops*bytesPerOp)/secs/(1024*1024), "MB/sec")
	}
}

// ---------- Benchmarks ----------
func BenchmarkSequentialWrite(b *testing.B) {
	for _, tc := range []struct {
		name string
		n    int
	}{
		{"100k", 100_000},
		{"500k", 500_000},
		{"1M", 1_000_000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			dir := b.TempDir()
			keys := makeOrderedKeys(tc.n)
			val := benchPayload
			bytesPerOp := len(keys[0]) + len(val)

			db, err := Open(Options{
				Dir:                 dir,
				CompactionThreshold: benchCompactionHoldoff,
			})
			if err != nil {
				b.Fatalf("open: %v", err)
			}
			defer db.Close()

			b.SetBytes(int64(bytesPerOp))
			b.ResetTimer()
			for i := 0; i < tc.n; i++ {
				if err := db.Put(keys[i], val); err != nil {
					b.Fatalf("put %d: %v", i, err)
				}
			}
			b.StopTimer()
			reportThroughput(b, tc.n, bytesPerOp)
		})
	}
}

// BenchmarkRandomRead – runs against memtable only (no close/reopen).
func BenchmarkRandomRead(b *testing.B) {
	const dataset = 50_000

	b.StopTimer()
	db, keys := benchPreload(b, Options{}, dataset)
	defer db.Close()

	bytesPerOp := len(keys[0]) + benchValueLen
	b.SetBytes(int64(bytesPerOp))
	b.SetParallelism(4)
	runtime.GOMAXPROCS(4)

	b.StartTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			k := keys[rng.Intn(len(keys))]
			if _, err := db.Get(k); err != nil {
				b.Fatalf("get: %v", err)
			}
		}
	})
}

// scanOnce helper
func scanOnce(b *testing.B, db *DB, start, end []byte) int {
	b.Helper()
	it, err := db.Scan(start, end)
	if err != nil {
		b.Fatalf("scan: %v", err)
	}
	defer it.Close()
	count := 0
	for it.Valid() {
		_ = it.Key()
		_ = it.Value()
		count++
		if err := it.Next(); err != nil {
			b.Fatalf("next: %v", err)
		}
	}
	return count
}

// BenchmarkScanThroughput – runs against memtable only.
func BenchmarkScanThroughput(b *testing.B) {
	const dataset = 50_000

	b.StopTimer()
	db, keys := benchPreload(b, Options{}, dataset)
	defer db.Close()

	start := keys[0]
	bytesPerEntry := len(keys[0]) + benchValueLen

	for _, tc := range []struct {
		name  string
		count int
	}{
		{"1k", 1_000},
		{"10k", 10_000},
		{"50k", 50_000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var end []byte
			if tc.count < dataset {
				end = keys[tc.count]
			}
			b.SetBytes(int64(bytesPerEntry))
			b.ResetTimer()
			var total int
			for i := 0; i < b.N; i++ {
				got := scanOnce(b, db, start, end)
				total += got
			}
			b.StopTimer()
			if total == 0 {
				b.Fatal("no entries scanned")
			}
			reportThroughput(b, total, bytesPerEntry)
		})
	}
}
