package compat

import (
	"errors"

	"github.com/go-git/go-git/v6/plumbing"
)

type encodedObjectSetter interface {
	SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error)
}

type encodedObjectLookup interface {
	HasEncodedObject(plumbing.Hash) error
	EncodedObject(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error)
}

// HasEncodedObject returns nil if the object exists by native or compat hash.
func HasEncodedObject(s encodedObjectLookup, t *Translator, h plumbing.Hash) error {
	err := s.HasEncodedObject(h)
	if err == nil || t == nil {
		return err
	}

	native, cerr := t.Mapping().CompatToNative(h)
	if cerr != nil {
		return err
	}
	return s.HasEncodedObject(native)
}

// EncodedObject returns the object by native or compat hash.
func EncodedObject(s encodedObjectLookup, t *Translator, ot plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	obj, err := s.EncodedObject(ot, h)
	if err == nil || t == nil {
		return obj, err
	}

	native, cerr := t.Mapping().CompatToNative(h)
	if cerr != nil {
		return nil, err
	}
	return s.EncodedObject(ot, native)
}

// SetEncodedObject stores the object and eagerly records its compat mapping
// unless compat translation is deferred for the current caller.
func SetEncodedObject(s encodedObjectSetter, t *Translator, obj plumbing.EncodedObject, deferTranslation bool) (plumbing.Hash, error) {
	h, err := s.SetEncodedObject(obj)
	if err != nil || t == nil || deferTranslation {
		return h, err
	}

	_, err = t.TranslateObject(obj)
	if err != nil && deferTranslation && errors.Is(err, ErrMissingDependencyMapping) {
		return h, nil
	}
	return h, err
}
