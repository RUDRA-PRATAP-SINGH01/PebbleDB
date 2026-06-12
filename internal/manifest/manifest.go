package manifest

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	currentFileName = "CURRENT"
	manifestPrefix  = "MANIFEST-"
	initialManifest = "MANIFEST-000001"

	compactRecordThreshold = 64
	compactSizeThreshold   = 64 << 10 // 64 KiB
)

// Log is the append-only manifest tracking the live SSTable set.
type Log struct {
	dir         string
	path        string
	file        *os.File
	mu          sync.Mutex
	liveSet     map[uint64]struct{}
	recordCount int
}

// Open opens or creates the manifest in dir.
func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	manifestFile, err := readCurrentManifest(dir)
	if err != nil {
		return nil, err
	}
	if manifestFile == "" {
		manifestFile = initialManifest
		if err := writeCurrent(dir, manifestFile); err != nil {
			return nil, err
		}
	}

	manifestPath := filepath.Join(dir, manifestFile)
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

func readCurrentManifest(dir string) (string, error) {
	currentPath := filepath.Join(dir, currentFileName)
	data, err := os.ReadFile(currentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
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

	validEnd := int64(0)
	for {
		recordStart, err := l.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}

		header := make([]byte, 4)
		_, err = io.ReadFull(l.file, header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return l.salvageManifestTail(validEnd, err)
		}

		recordLen := binary.BigEndian.Uint32(header)
		if recordLen < 4 {
			return l.salvageManifestTail(validEnd, io.ErrUnexpectedEOF)
		}

		rest := make([]byte, recordLen)
		if _, err := io.ReadFull(l.file, rest); err != nil {
			return l.salvageManifestTail(validEnd, err)
		}

		record := append(header, rest...)
		payload, err := decodeRecord(record)
		if err != nil {
			return l.salvageManifestTail(validEnd, err)
		}
		if err := applyEdit(l.liveSet, payload); err != nil {
			return err
		}
		l.recordCount++
		validEnd = recordStart + int64(len(record))
	}

	end, err := l.file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if validEnd < end {
		if err := l.truncateTo(validEnd); err != nil {
			return err
		}
	}
	return nil
}

// truncateTo shortens the manifest file. On Windows the file must be closed
// before os.Truncate; reopen for subsequent appends.
func (l *Log) truncateTo(validEnd int64) error {
	if err := l.file.Sync(); err != nil {
		return err
	}
	fi, err := l.file.Stat()
	if err != nil {
		return err
	}
	if validEnd >= fi.Size() {
		_, err := l.file.Seek(0, io.SeekEnd)
		return err
	}
	if err := l.file.Close(); err != nil {
		return err
	}
	if err := os.Truncate(l.path, validEnd); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = f
	_, err = l.file.Seek(0, io.SeekEnd)
	return err
}

func (l *Log) salvageManifestTail(validEnd int64, cause error) error {
	if cause != io.EOF && cause != io.ErrUnexpectedEOF {
		return cause
	}
	return l.truncateTo(validEnd)
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
	l.recordCount++
	return nil
}

// MaybeCompact rewrites the manifest as a single SetFileSet snapshot and rotates
// to a new manifest file when the log grows too large.
func (l *Log) MaybeCompact() error {
	l.mu.Lock()
	need := l.recordCount >= compactRecordThreshold
	var size int64
	if !need {
		if fi, err := l.file.Stat(); err == nil {
			size = fi.Size()
			need = size >= compactSizeThreshold
		}
	}
	if !need {
		l.mu.Unlock()
		return nil
	}
	ids := make([]uint64, 0, len(l.liveSet))
	for id := range l.liveSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	oldPath := l.path
	l.mu.Unlock()

	return l.rotateSnapshot(ids, oldPath)
}

func (l *Log) rotateSnapshot(ids []uint64, oldPath string) error {
	newName, err := nextManifestName(oldPath)
	if err != nil {
		return err
	}
	newPath := filepath.Join(l.dir, newName)
	record := encodeSetFileSet(ids)

	f, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(record); err != nil {
		f.Close()
		os.Remove(newPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(newPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(newPath)
		return err
	}
	if err := writeCurrent(l.dir, newName); err != nil {
		os.Remove(newPath)
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
	}
	opened, err := os.OpenFile(newPath, os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = opened
	l.path = newPath
	l.recordCount = 1
	if oldPath != newPath {
		_ = os.Remove(oldPath)
	}
	return nil
}

func nextManifestName(current string) (string, error) {
	base := filepath.Base(current)
	if !strings.HasPrefix(base, manifestPrefix) {
		return initialManifest, nil
	}
	numStr := strings.TrimPrefix(base, manifestPrefix)
	n, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil || n == 0 {
		return initialManifest, nil
	}
	return fmt.Sprintf("%s%06d", manifestPrefix, n+1), nil
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
