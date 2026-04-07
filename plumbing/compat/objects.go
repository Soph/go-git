package compat

import (
	"fmt"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// TranslateStoredObjects iterates over all objects in the given storage and
// computes compat hash mappings for any objects that don't already have one.
// Objects are processed in topological order: blobs first, then trees, then
// commits and tags.
//
// This is useful after a batch import (e.g. initial clone) to populate
// the mapping table for all stored objects.
func TranslateStoredObjects(s storer.EncodedObjectStorer, t *Translator) error {
	// Phase 1: Translate blobs (no dependencies).
	if err := processObjectsOfType(s, plumbing.BlobObject, func(obj plumbing.EncodedObject) (bool, error) {
		if hasNativeMapping(t, obj.Hash()) {
			return true, nil
		}
		_, err := t.TranslateObject(obj)
		return false, err
	}); err != nil {
		return fmt.Errorf("translate blobs: %w", err)
	}

	// Phase 2: Translate trees (depend on blobs and other trees).
	// Trees may reference other trees, so we iterate until no new
	// translations are made.
	if err := processObjectsWithRetry(s, plumbing.TreeObject,
		func(obj plumbing.EncodedObject) bool {
			return hasNativeMapping(t, obj.Hash())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.TranslateObject(obj)
			return err
		},
	); err != nil {
		return fmt.Errorf("translate trees: %w", err)
	}

	// Phase 3: Translate commits (depend on trees and other commits).
	if err := processObjectsWithRetry(s, plumbing.CommitObject,
		func(obj plumbing.EncodedObject) bool {
			return hasNativeMapping(t, obj.Hash())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.TranslateObject(obj)
			return err
		},
	); err != nil {
		return fmt.Errorf("translate commits: %w", err)
	}

	// Phase 4: Translate tags (depend on any object type).
	if err := processObjectsWithRetry(s, plumbing.TagObject,
		func(obj plumbing.EncodedObject) bool {
			return hasNativeMapping(t, obj.Hash())
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
// records the bidirectional mappings. Objects are processed in topological
// order so internal references can be rewritten safely.
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

	if err := processObjectsWithRetry(src, plumbing.TreeObject,
		func(obj plumbing.EncodedObject) bool {
			return hasCompatMapping(t, obj.Hash())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.ImportObject(obj, dst)
			return err
		},
	); err != nil {
		return fmt.Errorf("import trees: %w", err)
	}

	if err := processObjectsWithRetry(src, plumbing.CommitObject,
		func(obj plumbing.EncodedObject) bool {
			return hasCompatMapping(t, obj.Hash())
		},
		func(obj plumbing.EncodedObject) error {
			_, err := t.ImportObject(obj, dst)
			return err
		},
	); err != nil {
		return fmt.Errorf("import commits: %w", err)
	}

	if err := processObjectsWithRetry(src, plumbing.TagObject,
		func(obj plumbing.EncodedObject) bool {
			return hasCompatMapping(t, obj.Hash())
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

func processObjectsWithRetry(
	s storer.EncodedObjectStorer,
	objType plumbing.ObjectType,
	isProcessed func(plumbing.EncodedObject) bool,
	process func(plumbing.EncodedObject) error,
) error {
	maxIterations, err := countObjectsOfType(s, objType)
	if err != nil {
		return err
	}

	for iteration := 0; ; iteration++ {
		if iteration > maxIterations {
			return fmt.Errorf("unable to translate %s objects after %d passes", objType, maxIterations)
		}
		translated := 0
		skipped := 0

		iter, err := s.IterEncodedObjects(objType)
		if err != nil {
			return err
		}

		err = iter.ForEach(func(obj plumbing.EncodedObject) error {
			if isProcessed(obj) {
				return nil
			}

			if err := process(obj); err != nil {
				// Dependencies not yet translated; skip for now.
				skipped++
				return nil
			}
			translated++
			return nil
		})
		iter.Close()

		if err != nil {
			return err
		}

		// If nothing was translated and nothing was skipped, we're done.
		if translated == 0 && skipped == 0 {
			return nil
		}

		// If nothing was translated but some were skipped, we have
		// unresolvable dependencies.
		if translated == 0 && skipped > 0 {
			return fmt.Errorf("unable to translate %d %s objects: missing dependencies", skipped, objType)
		}

		// If everything was translated, we're done.
		if skipped == 0 {
			return nil
		}

		// Otherwise, retry to catch objects whose deps were just translated.
	}
}

func hasNativeMapping(t *Translator, h plumbing.Hash) bool {
	_, err := t.mapping.NativeToCompat(h)
	return err == nil
}

func hasCompatMapping(t *Translator, h plumbing.Hash) bool {
	_, err := t.mapping.CompatToNative(h)
	return err == nil
}

func countObjectsOfType(s storer.EncodedObjectStorer, objType plumbing.ObjectType) (int, error) {
	iter, err := s.IterEncodedObjects(objType)
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	count := 0
	err = iter.ForEach(func(plumbing.EncodedObject) error {
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}

	return count, nil
}
