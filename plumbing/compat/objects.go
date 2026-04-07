package compat

import (
	"bytes"
	"fmt"

	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// TranslateStoredObjects iterates over all objects in the given storage and
// computes compat hash mappings for any objects that don't already have one.
// Objects are processed in topological order: blobs first, then trees, then
// commits and tags.
func TranslateStoredObjects(s storer.EncodedObjectStorer, t *Translator) error {
	if err := processObjectsOfType(s, plumbing.BlobObject, func(obj plumbing.EncodedObject) (bool, error) {
		if hasNativeMapping(t, obj.Hash()) {
			return true, nil
		}
		_, err := t.TranslateObject(obj)
		return false, err
	}); err != nil {
		return fmt.Errorf("translate blobs: %w", err)
	}

	if err := processObjectsTopologically(s, plumbing.TreeObject,
		func(obj plumbing.EncodedObject) bool { return hasNativeMapping(t, obj.Hash()) },
		func(obj plumbing.EncodedObject) ([]plumbing.Hash, error) {
			return objectDependencies(obj, t.NativeObjectFormat())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.TranslateObject(obj)
			return err
		},
	); err != nil {
		return fmt.Errorf("translate trees: %w", err)
	}

	if err := processObjectsTopologically(s, plumbing.CommitObject,
		func(obj plumbing.EncodedObject) bool { return hasNativeMapping(t, obj.Hash()) },
		func(obj plumbing.EncodedObject) ([]plumbing.Hash, error) {
			return objectDependencies(obj, t.NativeObjectFormat())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.TranslateObject(obj)
			return err
		},
	); err != nil {
		return fmt.Errorf("translate commits: %w", err)
	}

	if err := processObjectsTopologically(s, plumbing.TagObject,
		func(obj plumbing.EncodedObject) bool { return hasNativeMapping(t, obj.Hash()) },
		func(obj plumbing.EncodedObject) ([]plumbing.Hash, error) {
			return objectDependencies(obj, t.NativeObjectFormat())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.TranslateObject(obj)
			return err
		},
	); err != nil {
		return fmt.Errorf("translate tags: %w", err)
	}

	return nil
}

// ImportStoredObjects iterates over objects stored in compat format in src,
// rewrites them into the translator's native format, stores them in dst, and
// records the bidirectional mappings.
func ImportStoredObjects(src, dst storer.EncodedObjectStorer, t *Translator) error {
	if err := processObjectsOfType(src, plumbing.BlobObject, func(obj plumbing.EncodedObject) (bool, error) {
		if hasCompatMapping(t, obj.Hash()) {
			return true, nil
		}
		_, err := t.ImportObject(obj, dst)
		return false, err
	}); err != nil {
		return fmt.Errorf("import blobs: %w", err)
	}

	if err := processObjectsTopologically(src, plumbing.TreeObject,
		func(obj plumbing.EncodedObject) bool { return hasCompatMapping(t, obj.Hash()) },
		func(obj plumbing.EncodedObject) ([]plumbing.Hash, error) {
			return objectDependencies(obj, t.CompatObjectFormat())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.ImportObject(obj, dst)
			return err
		},
	); err != nil {
		return fmt.Errorf("import trees: %w", err)
	}

	if err := processObjectsTopologically(src, plumbing.CommitObject,
		func(obj plumbing.EncodedObject) bool { return hasCompatMapping(t, obj.Hash()) },
		func(obj plumbing.EncodedObject) ([]plumbing.Hash, error) {
			return objectDependencies(obj, t.CompatObjectFormat())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.ImportObject(obj, dst)
			return err
		},
	); err != nil {
		return fmt.Errorf("import commits: %w", err)
	}

	if err := processObjectsTopologically(src, plumbing.TagObject,
		func(obj plumbing.EncodedObject) bool { return hasCompatMapping(t, obj.Hash()) },
		func(obj plumbing.EncodedObject) ([]plumbing.Hash, error) {
			return objectDependencies(obj, t.CompatObjectFormat())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.ImportObject(obj, dst)
			return err
		},
	); err != nil {
		return fmt.Errorf("import tags: %w", err)
	}

	return nil
}

func processObjectsOfType(
	s storer.EncodedObjectStorer,
	objType plumbing.ObjectType,
	process func(plumbing.EncodedObject) (skip bool, err error),
) error {
	iter, err := s.IterEncodedObjects(objType)
	if err != nil {
		return err
	}
	defer iter.Close()

	return iter.ForEach(func(obj plumbing.EncodedObject) error {
		skip, err := process(obj)
		if skip {
			return nil
		}
		return err
	})
}

func processObjectsTopologically(
	s storer.EncodedObjectStorer,
	objType plumbing.ObjectType,
	isProcessed func(plumbing.EncodedObject) bool,
	deps func(plumbing.EncodedObject) ([]plumbing.Hash, error),
	process func(plumbing.EncodedObject) error,
) error {
	objs, err := collectObjectsOfType(s, objType)
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return nil
	}

	nodes := make(map[plumbing.Hash]*topoNode, len(objs))
	for _, obj := range objs {
		if isProcessed(obj) {
			continue
		}
		nodes[obj.Hash()] = &topoNode{obj: obj}
	}
	if len(nodes) == 0 {
		return nil
	}

	for hash, node := range nodes {
		dependencies, err := deps(node.obj)
		if err != nil {
			return err
		}
		seen := make(map[plumbing.Hash]struct{}, len(dependencies))
		for _, dep := range dependencies {
			if dep.IsZero() {
				continue
			}
			if _, ok := seen[dep]; ok {
				continue
			}
			seen[dep] = struct{}{}

			parent, ok := nodes[dep]
			if !ok {
				continue
			}
			node.pending++
			parent.dependents = append(parent.dependents, hash)
		}
	}

	queue := make([]plumbing.Hash, 0, len(nodes))
	for hash, node := range nodes {
		if node.pending == 0 {
			queue = append(queue, hash)
		}
	}

	processed := 0
	for len(queue) > 0 {
		hash := queue[0]
		queue = queue[1:]

		node := nodes[hash]
		if err := process(node.obj); err != nil {
			return err
		}
		processed++

		for _, dependentHash := range node.dependents {
			dependent := nodes[dependentHash]
			dependent.pending--
			if dependent.pending == 0 {
				queue = append(queue, dependentHash)
			}
		}
	}

	if processed != len(nodes) {
		return fmt.Errorf("unable to process %d %s objects: missing dependencies", len(nodes)-processed, objType)
	}

	return nil
}

type topoNode struct {
	obj        plumbing.EncodedObject
	pending    int
	dependents []plumbing.Hash
}

func collectObjectsOfType(s storer.EncodedObjectStorer, objType plumbing.ObjectType) ([]plumbing.EncodedObject, error) {
	iter, err := s.IterEncodedObjects(objType)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var objs []plumbing.EncodedObject
	err = iter.ForEach(func(obj plumbing.EncodedObject) error {
		objs = append(objs, obj)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return objs, nil
}

func objectDependencies(obj plumbing.EncodedObject, objectFormat formatcfg.ObjectFormat) ([]plumbing.Hash, error) {
	content, err := readObjectContent(obj)
	if err != nil {
		return nil, err
	}

	switch obj.Type() {
	case plumbing.BlobObject:
		return nil, nil
	case plumbing.TreeObject:
		return treeDependencies(content, objectFormat.Size())
	case plumbing.CommitObject:
		return commitDependencies(content, objectFormat.HexSize())
	case plumbing.TagObject:
		return tagDependencies(content, objectFormat.HexSize())
	default:
		return nil, nil
	}
}

func treeDependencies(content []byte, hashSize int) ([]plumbing.Hash, error) {
	var deps []plumbing.Hash
	buf := content
	for len(buf) > 0 {
		nullIdx := bytes.IndexByte(buf, 0)
		if nullIdx < 0 {
			return nil, fmt.Errorf("malformed tree entry: missing null byte")
		}
		buf = buf[nullIdx+1:]
		if len(buf) < hashSize {
			return nil, fmt.Errorf("malformed tree entry: truncated hash (have %d, want %d)", len(buf), hashSize)
		}
		h, _ := plumbing.FromBytes(buf[:hashSize])
		deps = append(deps, h)
		buf = buf[hashSize:]
	}
	return deps, nil
}

func commitDependencies(content []byte, hashHexSize int) ([]plumbing.Hash, error) {
	return parseTextObjectDependencies(content, hashHexSize, []string{"tree", "parent"}, true)
}

func tagDependencies(content []byte, hashHexSize int) ([]plumbing.Hash, error) {
	return parseTextObjectDependencies(content, hashHexSize, []string{"object"}, false)
}

func parseTextObjectDependencies(content []byte, hashHexSize int, hashFields []string, parseMergeTag bool) ([]plumbing.Hash, error) {
	var deps []plumbing.Hash
	remaining := content
	headerDone := false

	for len(remaining) > 0 {
		next := nextLineInfo(remaining)
		line := next.line
		remaining = next.rest

		if headerDone {
			continue
		}
		if len(line) == 0 {
			headerDone = true
			continue
		}

		if parseMergeTag && bytes.HasPrefix(line, []byte("mergetag ")) {
			payloadLines := [][]byte{line[len("mergetag "):]}
			for len(remaining) > 0 {
				next = nextLineInfo(remaining)
				if len(next.line) == 0 || next.line[0] != ' ' {
					break
				}
				payloadLines = append(payloadLines, next.line[1:])
				remaining = next.rest
			}
			mergeDeps, err := parseTextObjectDependencies(bytes.Join(payloadLines, []byte("\n")), hashHexSize, []string{"object"}, false)
			if err != nil {
				return nil, err
			}
			deps = append(deps, mergeDeps...)
			continue
		}

		for _, field := range hashFields {
			prefix := []byte(field + " ")
			if !bytes.HasPrefix(line, prefix) || len(line) != len(prefix)+hashHexSize {
				continue
			}
			h, ok := plumbing.FromHex(string(line[len(prefix):]))
			if !ok {
				return nil, fmt.Errorf("invalid hash on %s line: %q", field, line[len(prefix):])
			}
			deps = append(deps, h)
			break
		}
	}

	return deps, nil
}

func hasNativeMapping(t *Translator, h plumbing.Hash) bool {
	_, err := t.mapping.NativeToCompat(h)
	return err == nil
}

func hasCompatMapping(t *Translator, h plumbing.Hash) bool {
	_, err := t.mapping.CompatToNative(h)
	return err == nil
}
