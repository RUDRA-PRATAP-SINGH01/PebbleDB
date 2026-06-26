package iterator

type source struct {
	it       Iterator
	priority int
}

// MergeIterator merges multiple iterators; newer sources (higher priority) win.
// Tombstones are skipped and never returned to the caller.
type MergeIterator struct {
	sources []source
	key     []byte
	value   []byte
	valid   bool
	err     error
}

// NewMergeIterator creates a merge iterator over sources with matching priorities.
// priorities[i] applies to sources[i]; larger values are newer.
func NewMergeIterator(sources []Iterator, priorities []int) (*MergeIterator, error) {
	if len(sources) != len(priorities) {
		return nil, ErrPriorityMismatch
	}
	m := &MergeIterator{
		sources: make([]source, len(sources)),
	}
	for i := range sources {
		m.sources[i] = source{it: sources[i], priority: priorities[i]}
	}
	return m, nil
}

// Seek positions all sources and loads the first visible entry at or after key.
func (m *MergeIterator) Seek(key []byte) error {
	m.err = nil
	for _, s := range m.sources {
		if err := s.it.Seek(key); err != nil {
			m.err = err
			m.valid = false
			return err
		}
	}
	return m.advance()
}

func (m *MergeIterator) Valid() bool {
	return m.valid && m.err == nil
}

func (m *MergeIterator) Key() []byte {
	return m.key
}

func (m *MergeIterator) Value() []byte {
	return m.value
}

func (m *MergeIterator) IsTombstone() bool {
	return false
}

func (m *MergeIterator) Next() error {
	if m.err != nil {
		return m.err
	}
	return m.advance()
}

func (m *MergeIterator) Close() error {
	var first error
	for _, s := range m.sources {
		if err := s.it.Close(); err != nil && first == nil {
			first = err
		}
	}
	m.valid = false
	return first
}

func (m *MergeIterator) Err() error {
	return m.err
}

func (m *MergeIterator) advance() error {
	key, value, _, ok, err := mergeStep(m.sources, true)
	if err != nil {
		m.err = err
		m.valid = false
		return err
	}
	if !ok {
		m.valid = false
		m.key = nil
		m.value = nil
		return m.err
	}

	m.key = append(m.key[:0], key...)
	if value == nil {
		m.value = nil
	} else {
		m.value = append(m.value[:0], value...)
	}
	m.valid = true
	return nil
}

func (m *MergeIterator) minKey() []byte {
	return minKeyAcrossSources(m.sources)
}
