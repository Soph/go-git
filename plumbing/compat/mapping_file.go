package compat

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v6"

	"github.com/go-git/go-git/v6/plumbing"
)

const looseObjectIdxFile = "loose-object-idx"

// FileMapping is a filesystem-backed implementation of HashMapping.
// It reads and writes the loose-object-idx file in the objects directory,
// following the format defined in Git's hash-function-transition document:
//
//	<native-hex> <compat-hex>\n
//
// The file is lazily loaded on first access and cached in memory.
type FileMapping struct {
	mu       sync.RWMutex
	loadOnce sync.Once
	loadErr  error
	fs       billy.Filesystem
	path     string // directory containing the idx file

	appendFile     billy.File
	nativeToCompat map[plumbing.Hash]plumbing.Hash
	compatToNative map[plumbing.Hash]plumbing.Hash
}

// NewFileMapping creates a FileMapping backed by the given filesystem and
// directory path (typically the objects directory, e.g. ".git/objects").
func NewFileMapping(fs billy.Filesystem, path string) *FileMapping {
	return &FileMapping{
		fs:             fs,
		path:           path,
		nativeToCompat: make(map[plumbing.Hash]plumbing.Hash),
		compatToNative: make(map[plumbing.Hash]plumbing.Hash),
	}
}

func (m *FileMapping) idxPath() string {
	return m.fs.Join(m.path, looseObjectIdxFile)
}

// load reads the loose-object-idx file into memory. Must be called
// exactly once via loadOnce before accessing the maps.
func (m *FileMapping) load() error {
	f, err := m.fs.Open(m.idxPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open loose-object-idx: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue // skip malformed lines
		}

		native, nok := plumbing.FromHex(parts[0])
		compat, cok := plumbing.FromHex(parts[1])
		if !nok || !cok {
			continue // skip malformed hashes
		}

		m.nativeToCompat[native] = compat
		m.compatToNative[compat] = native
	}

	return scanner.Err()
}

func (m *FileMapping) ensureLoaded() error {
	m.loadOnce.Do(func() {
		m.loadErr = m.load()
	})
	return m.loadErr
}

func (m *FileMapping) NativeToCompat(native plumbing.Hash) (plumbing.Hash, error) {
	if err := m.ensureLoaded(); err != nil {
		return plumbing.Hash{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	h, ok := m.nativeToCompat[native]
	if !ok {
		return plumbing.Hash{}, plumbing.ErrObjectNotFound
	}
	return h, nil
}

func (m *FileMapping) CompatToNative(compat plumbing.Hash) (plumbing.Hash, error) {
	if err := m.ensureLoaded(); err != nil {
		return plumbing.Hash{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	h, ok := m.compatToNative[compat]
	if !ok {
		return plumbing.Hash{}, plumbing.ErrObjectNotFound
	}
	return h, nil
}

func (m *FileMapping) Add(native, compat plumbing.Hash) error {
	if err := m.ensureLoaded(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.nativeToCompat[native]; ok && existing.Equal(compat) {
		return nil
	}

	f, err := m.ensureAppendFile()
	if err != nil {
		return err
	}

	line := fmt.Sprintf("%s %s\n", native.String(), compat.String())
	if _, err := f.Write([]byte(line)); err != nil {
		return fmt.Errorf("write to loose-object-idx: %w", err)
	}
	if syncer, ok := f.(billy.Syncer); ok {
		if err := syncer.Sync(); err != nil {
			return fmt.Errorf("sync loose-object-idx: %w", err)
		}
	}

	if existing, ok := m.nativeToCompat[native]; ok {
		delete(m.compatToNative, existing)
	}
	if existing, ok := m.compatToNative[compat]; ok {
		delete(m.nativeToCompat, existing)
	}
	m.nativeToCompat[native] = compat
	m.compatToNative[compat] = native

	return nil
}

func (m *FileMapping) ensureAppendFile() (billy.File, error) {
	if m.appendFile != nil {
		return m.appendFile, nil
	}

	f, err := m.fs.OpenFile(m.idxPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open loose-object-idx for append: %w", err)
	}
	m.appendFile = f
	return f, nil
}

func (m *FileMapping) Count() int {
	_ = m.ensureLoaded()

	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.nativeToCompat)
}
