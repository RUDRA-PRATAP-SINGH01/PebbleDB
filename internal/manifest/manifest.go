package manifest

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	currentFileName = "CURRENT"
	manifestName    = "MANIFEST-000001"
)

// Log is the append-only manifest tracking the live SSTable set.
type Log struct {
	dir     string
	path    string
	file    *os.File
	mu      sync.Mutex
	liveSet map[uint64]struct{}
}

// Open opens or creates the manifest in dir.
func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(dir, manifestName)
	if err := ensureCurrent(dir, manifestName); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(manifestPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	l := &Log{
		dir:     dir,
		path:    manifestPath,
		file:    f,
		liveSet: make(map[uint64]struct{}),
	}
	if err := l.replay(); err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}

func ensureCurrent(dir, manifestFile string) error {
	currentPath := filepath.Join(dir, currentFileName)
	data, err := os.ReadFile(currentPath)
	if err == nil {
		name := strings.TrimSpace(string(data))
		if name == manifestFile {
			return nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeCurrent(dir, manifestFile)
}

func writeCurrent(dir, manifestFile string) error {
	currentPath := filepath.Join(dir, currentFileName)
	tmpPath := currentPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	content := []byte(manifestFile + "\n")
	if _, err := f.Write(content); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, currentPath)
}

func (l *Log) replay() error {
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	br := bufio.NewReader(l.file)
	for {
		header := make([]byte, 4)
		if _, err := io.ReadFull(br, header); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		recordLen := binary.BigEndian.Uint32(header)
		if recordLen < 4 {
			return io.ErrUnexpectedEOF
		}
		rest := make([]byte, recordLen)
		if _, err := io.ReadFull(br, rest); err != nil {
			return err
		}
		record := append(header, rest...)
		payload, err := decodeRecord(record)
		if err != nil {
			return err
		}
		if err := applyEdit(l.liveSet, payload); err != nil {
			return err
		}
	}
	if _, err := l.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	return nil
}

// LiveIDs returns sorted live SSTable IDs.
func (l *Log) LiveIDs() []uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := make([]uint64, 0, len(l.liveSet))
	for id := range l.liveSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// AppendNewFile records a new live SSTable.
func (l *Log) AppendNewFile(sstID uint64) error {
	return l.append(encodeNewFile(sstID), func() {
		l.liveSet[sstID] = struct{}{}
	})
}

// AppendSetFileSet atomically replaces the live SSTable set.
func (l *Log) AppendSetFileSet(ids []uint64) error {
	sorted := append([]uint64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return l.append(encodeSetFileSet(sorted), func() {
		clear(l.liveSet)
		for _, id := range sorted {
			l.liveSet[id] = struct{}{}
		}
	})
}

// BootstrapIfEmpty writes an initial file set when upgrading a directory
// that already has SSTables but no manifest records.
func (l *Log) BootstrapIfEmpty(ids []uint64) error {
	l.mu.Lock()
	empty := len(l.liveSet) == 0
	l.mu.Unlock()
	if !empty || len(ids) == 0 {
		return nil
	}
	return l.AppendSetFileSet(ids)
}

func (l *Log) append(record []byte, apply func()) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.file.Write(record); err != nil {
		return err
	}
	if err := l.file.Sync(); err != nil {
		return err
	}
	apply()
	return nil
}

// Close closes the manifest file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// ReplayFile reads a manifest file for tests.
func ReplayFile(path string) ([]uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	liveSet := make(map[uint64]struct{})
	pos := 0
	for pos < len(data) {
		if pos+4 > len(data) {
			return nil, io.ErrUnexpectedEOF
		}
		recordLen := binary.BigEndian.Uint32(data[pos : pos+4])
		total := int(4 + recordLen)
		if pos+total > len(data) {
			return nil, io.ErrUnexpectedEOF
		}
		payload, err := decodeRecord(data[pos : pos+total])
		if err != nil {
			return nil, err
		}
		if err := applyEdit(liveSet, payload); err != nil {
			return nil, err
		}
		pos += total
	}
	ids := make([]uint64, 0, len(liveSet))
	for id := range liveSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// Contains reports whether id is in the live set.
func (l *Log) Contains(id uint64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.liveSet[id]
	return ok
}
