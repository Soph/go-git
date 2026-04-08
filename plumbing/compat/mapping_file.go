package compat

import (
	"bufio"
	"container/list"
	"fmt"
	"io"
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
// The file remains the source of truth; a bounded in-memory cache keeps
// repeated lookups fast without retaining the full mapping set.
type FileMapping struct {
	mu   sync.RWMutex
	fs   billy.Filesystem
	path string // directory containing the idx file

	appendFile     billy.File
	nativeToCompat *mappingCache
	compatToNative *mappingCache
}

const fileMappingCacheSize = 4096

// NewFileMapping creates a FileMapping backed by the given filesystem and
// directory path (typically the objects directory, e.g. ".git/objects").
func NewFileMapping(fs billy.Filesystem, path string) *FileMapping {
	return &FileMapping{
		fs:             fs,
		path:           path,
		nativeToCompat: newMappingCache(fileMappingCacheSize),
		compatToNative: newMappingCache(fileMappingCacheSize),
	}
}

func (m *FileMapping) idxPath() string {
	return m.fs.Join(m.path, looseObjectIdxFile)
}

func (m *FileMapping) openIdx() (billy.File, error) {
	f, err := m.fs.Open(m.idxPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open loose-object-idx: %w", err)
	}
	return f, nil
}

func (m *FileMapping) NativeToCompat(native plumbing.Hash) (plumbing.Hash, error) {
	m.mu.RLock()
	if h, ok := m.nativeToCompat.get(native); ok {
		m.mu.RUnlock()
		m.promoteNativeToCompat(native, h)
		return h, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if h, ok := m.nativeToCompat.get(native); ok {
		return h, nil
	}

	h, err := m.scanNativeToCompat(native)
	if err != nil {
		return plumbing.Hash{}, err
	}

	m.nativeToCompat.add(native, h)
	m.compatToNative.add(h, native)
	return h, nil
}

func (m *FileMapping) CompatToNative(compat plumbing.Hash) (plumbing.Hash, error) {
	m.mu.RLock()
	if h, ok := m.compatToNative.get(compat); ok {
		m.mu.RUnlock()
		m.promoteCompatToNative(compat, h)
		return h, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if h, ok := m.compatToNative.get(compat); ok {
		return h, nil
	}

	h, err := m.scanCompatToNative(compat)
	if err != nil {
		return plumbing.Hash{}, err
	}

	m.compatToNative.add(compat, h)
	m.nativeToCompat.add(h, compat)
	return h, nil
}

func (m *FileMapping) scanNativeToCompat(target plumbing.Hash) (plumbing.Hash, error) {
	f, err := m.openIdx()
	if err != nil {
		return plumbing.Hash{}, err
	}
	if f == nil {
		return plumbing.Hash{}, plumbing.ErrObjectNotFound
	}
	defer f.Close()

	var (
		found  bool
		compat plumbing.Hash
	)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		native, currentCompat, ok := parseMappingLine(scanner.Text())
		if !ok {
			continue
		}
		if native.Equal(target) {
			compat = currentCompat
			found = true
			continue
		}
		if found && currentCompat.Equal(compat) {
			found = false
		}
	}
	if err := scanner.Err(); err != nil {
		return plumbing.Hash{}, err
	}
	if !found {
		return plumbing.Hash{}, plumbing.ErrObjectNotFound
	}
	return compat, nil
}

func (m *FileMapping) scanCompatToNative(target plumbing.Hash) (plumbing.Hash, error) {
	f, err := m.openIdx()
	if err != nil {
		return plumbing.Hash{}, err
	}
	if f == nil {
		return plumbing.Hash{}, plumbing.ErrObjectNotFound
	}
	defer f.Close()

	var (
		found  bool
		native plumbing.Hash
	)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		currentNative, compat, ok := parseMappingLine(scanner.Text())
		if !ok {
			continue
		}
		if compat.Equal(target) {
			native = currentNative
			found = true
			continue
		}
		if found && currentNative.Equal(native) {
			found = false
		}
	}
	if err := scanner.Err(); err != nil {
		return plumbing.Hash{}, err
	}
	if !found {
		return plumbing.Hash{}, plumbing.ErrObjectNotFound
	}
	return native, nil
}

func (m *FileMapping) Add(native, compat plumbing.Hash) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.nativeToCompat.get(native); ok && existing.Equal(compat) {
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

	if existing, ok := m.nativeToCompat.get(native); ok {
		m.compatToNative.delete(existing)
	}
	if existing, ok := m.compatToNative.get(compat); ok {
		m.nativeToCompat.delete(existing)
	}
	m.nativeToCompat.add(native, compat)
	m.compatToNative.add(compat, native)

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

// Close releases the cached append handle, if one has been opened.
func (m *FileMapping) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.appendFile == nil {
		return nil
	}

	closer, ok := m.appendFile.(io.Closer)
	if !ok {
		m.appendFile = nil
		return nil
	}

	m.appendFile = nil
	return closer.Close()
}

func (m *FileMapping) Count() int {
	f, err := m.openIdx()
	if err != nil || f == nil {
		return 0
	}
	defer f.Close()

	nativeToCompat := make(map[plumbing.Hash]plumbing.Hash)
	compatToNative := make(map[plumbing.Hash]plumbing.Hash)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		native, compat, ok := parseMappingLine(scanner.Text())
		if !ok {
			continue
		}
		if existing, ok := nativeToCompat[native]; ok {
			delete(compatToNative, existing)
		}
		if existing, ok := compatToNative[compat]; ok {
			delete(nativeToCompat, existing)
		}
		nativeToCompat[native] = compat
		compatToNative[compat] = native
	}

	return len(nativeToCompat)
}

func (m *FileMapping) promoteNativeToCompat(native, compat plumbing.Hash) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cached, ok := m.nativeToCompat.get(native); ok && cached.Equal(compat) {
		m.nativeToCompat.touch(native)
		m.compatToNative.touch(compat)
	}
}

func (m *FileMapping) promoteCompatToNative(compat, native plumbing.Hash) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cached, ok := m.compatToNative.get(compat); ok && cached.Equal(native) {
		m.compatToNative.touch(compat)
		m.nativeToCompat.touch(native)
	}
}

func parseMappingLine(line string) (plumbing.Hash, plumbing.Hash, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return plumbing.Hash{}, plumbing.Hash{}, false
	}

	native, nok := plumbing.FromHex(fields[0])
	compat, cok := plumbing.FromHex(fields[1])
	if !nok || !cok {
		return plumbing.Hash{}, plumbing.Hash{}, false
	}

	return native, compat, true
}

type mappingCache struct {
	capacity int
	order    *list.List
	entries  map[plumbing.Hash]*list.Element
}

type mappingCacheEntry struct {
	key   plumbing.Hash
	value plumbing.Hash
}

func newMappingCache(capacity int) *mappingCache {
	return &mappingCache{
		capacity: capacity,
		order:    list.New(),
		entries:  make(map[plumbing.Hash]*list.Element),
	}
}

func (c *mappingCache) get(key plumbing.Hash) (plumbing.Hash, bool) {
	elem, ok := c.entries[key]
	if !ok {
		return plumbing.Hash{}, false
	}
	return elem.Value.(*mappingCacheEntry).value, true
}

func (c *mappingCache) touch(key plumbing.Hash) {
	elem, ok := c.entries[key]
	if !ok {
		return
	}
	c.order.MoveToBack(elem)
}

func (c *mappingCache) add(key, value plumbing.Hash) {
	if c.capacity <= 0 {
		return
	}
	if elem, ok := c.entries[key]; ok {
		elem.Value.(*mappingCacheEntry).value = value
		return
	}

	elem := c.order.PushBack(&mappingCacheEntry{key: key, value: value})
	c.entries[key] = elem
	if len(c.entries) <= c.capacity {
		return
	}

	oldest := c.order.Front()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.entries, oldest.Value.(*mappingCacheEntry).key)
}

func (c *mappingCache) delete(key plumbing.Hash) {
	elem, ok := c.entries[key]
	if !ok {
		return
	}
	c.order.Remove(elem)
	delete(c.entries, key)
}
