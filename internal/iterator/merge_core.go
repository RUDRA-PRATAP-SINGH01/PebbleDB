package iterator

import "bytes"

// ForEachMerged walks all keys in merge order across sources. priorities[i]
// applies to sources[i]; larger values are newer and win on duplicate keys.
// When omitTombstones is true, tombstone keys are consumed but not passed to fn
// (scan semantics). When false, fn receives tombstone entries (compaction semantics).
func ForEachMerged(sources []Iterator, priorities []int, omitTombstones bool, fn func(key, value []byte, tombstone bool) error) error {
	if len(sources) != len(priorities) {
		return ErrPriorityMismatch
	}
	srcs := make([]source, len(sources))
	for i := range sources {
		srcs[i] = source{it: sources[i], priority: priorities[i]}
	}
	for {
		key, value, tomb, ok, err := mergeStep(srcs, omitTombstones)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := fn(key, value, tomb); err != nil {
			return err
		}
	}
}

// mergeStep advances all sources at the current minimum key and returns the
// winning entry. When omitTombstones is true and the winner is a tombstone,
// the step repeats without returning until a live key or exhaustion.
func mergeStep(srcs []source, omitTombstones bool) (key, value []byte, tombstone bool, ok bool, err error) {
	for {
		minKey := minKeyAcrossSources(srcs)
		if minKey == nil {
			return nil, nil, false, false, nil
		}

		bestPri := -1
		var winnerKey, winnerVal []byte
		winnerTomb := false
		var toAdvance []Iterator

		for _, s := range srcs {
			if !s.it.Valid() {
				continue
			}
			if !bytes.Equal(s.it.Key(), minKey) {
				continue
			}
			if s.priority > bestPri {
				bestPri = s.priority
				winnerKey = s.it.Key()
				winnerVal = s.it.Value()
				winnerTomb = s.it.IsTombstone()
			}
			toAdvance = append(toAdvance, s.it)
		}

		for _, it := range toAdvance {
			if err := it.Next(); err != nil {
				return nil, nil, false, false, err
			}
		}

		if winnerTomb && omitTombstones {
			continue
		}

		return winnerKey, winnerVal, winnerTomb, true, nil
	}
}

func minKeyAcrossSources(srcs []source) []byte {
	var minKey []byte
	for _, s := range srcs {
		if !s.it.Valid() {
			continue
		}
		k := s.it.Key()
		if minKey == nil || bytes.Compare(k, minKey) < 0 {
			minKey = k
		}
	}
	return minKey
}
