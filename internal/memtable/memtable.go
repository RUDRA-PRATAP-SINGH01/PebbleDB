package memtable

// Memtable is the public interface for the in‑memory write cache.
// For now it's just a skip list. We'll add more later (e.g., size tracking).
type Memtable struct {
	sl *SkipList
}

func New() *Memtable {
	return &Memtable{sl: NewSkipList()}
}

func (m *Memtable) Put(key, value []byte) {
	m.sl.Put(key, value)
}

func (m *Memtable) Get(key []byte) ([]byte, bool, bool) {
	return m.sl.Get(key)
}

func (m *Memtable) Delete(key []byte) {
	m.sl.Delete(key)
}

func (m *Memtable) Size() int64 {
	return m.sl.Size()
}

func (m *Memtable) Len() int {
	return m.sl.Len()
}

func (m *Memtable) Iterator() *SkipListIterator {
	return m.sl.Iterator()
}