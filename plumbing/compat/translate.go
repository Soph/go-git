package compat

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-git/go-git/v6/plumbing"
	format "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// Translator converts objects between native and compat hash formats,
// computing the compat-format hash and recording the mapping.
//
// Objects must be translated in topological order: blobs first, then
// trees, then commits and tags. Each object's referenced objects must
// already have mappings recorded before the object itself is translated.
type Translator struct {
	formats      Formats
	nativeHasher *plumbing.ObjectHasher
	compatHasher *plumbing.ObjectHasher
	mapping      HashMapping
}

// NewTranslator creates a Translator for the given format pair and mapping.
func NewTranslator(f Formats, m HashMapping) *Translator {
	return &Translator{
		formats:      f,
		nativeHasher: plumbing.FromObjectFormat(f.Native),
		compatHasher: plumbing.FromObjectFormat(f.Compat),
		mapping:      m,
	}
}

// Mapping returns the underlying HashMapping.
func (t *Translator) Mapping() HashMapping {
	return t.mapping
}

// TranslateObject computes the compat-format hash for an object stored in
// native format. It translates internal hash references (in trees, commits,
// and tags) using the mapping, then hashes the translated content with the
// compat hasher. The resulting mapping is recorded.
//
// For blobs, content is identical across formats; only the hash differs.
func (t *Translator) TranslateObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	content, err := readObjectContent(obj)
	if err != nil {
		return plumbing.Hash{}, err
	}

	compatContent, err := t.nativeToCompatContent(obj.Type(), content)
	if err != nil {
		return plumbing.Hash{}, err
	}

	compatHash, err := t.compatHasher.Compute(obj.Type(), compatContent)
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("compute compat hash: %w", err)
	}

	if err := t.mapping.Add(obj.Hash(), compatHash); err != nil {
		return plumbing.Hash{}, fmt.Errorf("record mapping: %w", err)
	}

	return compatHash, nil
}

// ImportObject stores an object written in compat format into dst using the
// native object format, then records the native<->compat mapping.
func (t *Translator) ImportObject(obj plumbing.EncodedObject, dst storer.EncodedObjectStorer) (plumbing.Hash, error) {
	content, err := readObjectContent(obj)
	if err != nil {
		return plumbing.Hash{}, err
	}

	nativeContent, err := t.compatToNativeContent(obj.Type(), content)
	if err != nil {
		return plumbing.Hash{}, err
	}

	nativeHash, err := storeObject(dst, obj.Type(), nativeContent)
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("store native object: %w", err)
	}

	if err := t.mapping.Add(nativeHash, obj.Hash()); err != nil {
		return plumbing.Hash{}, fmt.Errorf("record mapping: %w", err)
	}

	return nativeHash, nil
}

func readObjectContent(obj plumbing.EncodedObject) ([]byte, error) {
	reader, err := obj.Reader()
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read object content: %w", err)
	}

	return content, nil
}

func storeObject(dst storer.EncodedObjectStorer, objType plumbing.ObjectType, content []byte) (plumbing.Hash, error) {
	obj := dst.NewEncodedObject()
	obj.SetType(objType)
	obj.SetSize(int64(len(content)))

	w, err := obj.Writer()
	if err != nil {
		return plumbing.Hash{}, fmt.Errorf("open object writer: %w", err)
	}

	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.Hash{}, fmt.Errorf("write object content: %w", err)
	}
	if err := w.Close(); err != nil {
		return plumbing.Hash{}, fmt.Errorf("close object writer: %w", err)
	}

	return dst.SetEncodedObject(obj)
}

func (t *Translator) nativeToCompatContent(objType plumbing.ObjectType, content []byte) ([]byte, error) {
	switch objType {
	case plumbing.BlobObject:
		return content, nil
	case plumbing.TreeObject:
		compatContent, err := t.translateTree(content)
		if err != nil {
			return nil, fmt.Errorf("translate tree: %w", err)
		}
		return compatContent, nil
	case plumbing.CommitObject:
		compatContent, err := t.translateCommit(content)
		if err != nil {
			return nil, fmt.Errorf("translate commit: %w", err)
		}
		return compatContent, nil
	case plumbing.TagObject:
		compatContent, err := t.translateTag(content)
		if err != nil {
			return nil, fmt.Errorf("translate tag: %w", err)
		}
		return compatContent, nil
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objType)
	}
}

func (t *Translator) compatToNativeContent(objType plumbing.ObjectType, content []byte) ([]byte, error) {
	switch objType {
	case plumbing.BlobObject:
		return content, nil
	case plumbing.TreeObject:
		nativeContent, err := t.reverseTranslateTree(content)
		if err != nil {
			return nil, fmt.Errorf("translate tree: %w", err)
		}
		return nativeContent, nil
	case plumbing.CommitObject:
		nativeContent, err := t.reverseTranslateCommit(content)
		if err != nil {
			return nil, fmt.Errorf("translate commit: %w", err)
		}
		return nativeContent, nil
	case plumbing.TagObject:
		nativeContent, err := t.reverseTranslateTag(content)
		if err != nil {
			return nil, fmt.Errorf("translate tag: %w", err)
		}
		return nativeContent, nil
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objType)
	}
}

// translateTree rewrites binary hashes in tree entries from native to compat format.
// Tree entry format: <mode-octal> <name>\0<binary-hash>
func (t *Translator) translateTree(content []byte) ([]byte, error) {
	nativeSize := t.formats.Native.Size()
	compatSize := t.formats.Compat.Size()

	var out bytes.Buffer
	buf := content

	for len(buf) > 0 {
		// Find the null byte separating "mode name" from the binary hash.
		nullIdx := bytes.IndexByte(buf, 0)
		if nullIdx < 0 {
			return nil, fmt.Errorf("malformed tree entry: missing null byte")
		}

		// Copy everything up to and including the null byte.
		out.Write(buf[:nullIdx+1])
		buf = buf[nullIdx+1:]

		if len(buf) < nativeSize {
			return nil, fmt.Errorf("malformed tree entry: truncated hash (have %d, want %d)", len(buf), nativeSize)
		}

		// Read the native-format binary hash.
		nativeHash, _ := plumbing.FromBytes(buf[:nativeSize])
		buf = buf[nativeSize:]

		// Look up the compat hash.
		compatHash, err := t.mapping.NativeToCompat(nativeHash)
		if err != nil {
			return nil, fmt.Errorf("tree entry hash %s: no compat mapping: %w", nativeHash, err)
		}

		// Write the compat-format binary hash.
		out.Write(compatHash.Bytes()[:compatSize])
	}

	return out.Bytes(), nil
}

// translateCommit rewrites hex hashes on "tree" and "parent" lines.
func (t *Translator) translateCommit(content []byte) ([]byte, error) {
	return t.translateTextObject(content, []string{"tree", "parent"})
}

// translateTag rewrites the hex hash on the "object" line.
func (t *Translator) translateTag(content []byte) ([]byte, error) {
	return t.translateTextObject(content, []string{"object"})
}

// reverseTranslateCommit rewrites compat-format hex hashes on "tree" and
// "parent" lines back to native-format hashes.
func (t *Translator) reverseTranslateCommit(content []byte) ([]byte, error) {
	return t.reverseTranslateTextObject(content, []string{"tree", "parent"})
}

// reverseTranslateTag rewrites the compat-format hex hash on the "object" line
// back to the native object hash.
func (t *Translator) reverseTranslateTag(content []byte) ([]byte, error) {
	return t.reverseTranslateTextObject(content, []string{"object"})
}

// translateTextObject rewrites hex hashes on specified header lines.
// It processes lines until it hits an empty line (the header/body separator).
func (t *Translator) translateTextObject(content []byte, hashFields []string) ([]byte, error) {
	nativeHexSize := t.formats.Native.HexSize()

	var out bytes.Buffer
	remaining := content
	headerDone := false

	for len(remaining) > 0 {
		nlIdx := bytes.IndexByte(remaining, '\n')
		var line []byte
		if nlIdx >= 0 {
			line = remaining[:nlIdx]
			remaining = remaining[nlIdx+1:]
		} else {
			line = remaining
			remaining = nil
		}

		if !headerDone {
			if len(line) == 0 {
				// Empty line = end of header.
				headerDone = true
				out.WriteByte('\n')
				continue
			}

			replaced := false
			for _, field := range hashFields {
				prefix := field + " "
				if bytes.HasPrefix(line, []byte(prefix)) && len(line) == len(prefix)+nativeHexSize {
					hexStr := string(line[len(prefix):])
					nativeHash, ok := plumbing.FromHex(hexStr)
					if !ok {
						return nil, fmt.Errorf("invalid hash on %s line: %q", field, hexStr)
					}

					compatHash, err := t.mapping.NativeToCompat(nativeHash)
					if err != nil {
						return nil, fmt.Errorf("%s hash %s: no compat mapping: %w", field, nativeHash, err)
					}

					out.WriteString(prefix)
					out.WriteString(compatHash.String()[:t.formats.Compat.HexSize()])
					out.WriteByte('\n')
					replaced = true
					break
				}
			}

			if !replaced {
				out.Write(line)
				out.WriteByte('\n')
			}
		} else {
			// Body: copy verbatim.
			out.Write(line)
			if nlIdx >= 0 {
				out.WriteByte('\n')
			}
		}
	}

	return out.Bytes(), nil
}

// reverseTranslateTree rewrites binary hashes in tree entries from compat to
// native format.
func (t *Translator) reverseTranslateTree(content []byte) ([]byte, error) {
	nativeSize := t.formats.Native.Size()
	compatSize := t.formats.Compat.Size()

	var out bytes.Buffer
	buf := content

	for len(buf) > 0 {
		nullIdx := bytes.IndexByte(buf, 0)
		if nullIdx < 0 {
			return nil, fmt.Errorf("malformed tree entry: missing null byte")
		}

		out.Write(buf[:nullIdx+1])
		buf = buf[nullIdx+1:]

		if len(buf) < compatSize {
			return nil, fmt.Errorf("malformed tree entry: truncated hash (have %d, want %d)", len(buf), compatSize)
		}

		compatHash, _ := plumbing.FromBytes(buf[:compatSize])
		buf = buf[compatSize:]

		nativeHash, err := t.mapping.CompatToNative(compatHash)
		if err != nil {
			return nil, fmt.Errorf("tree entry hash %s: no native mapping: %w", compatHash, err)
		}

		out.Write(nativeHash.Bytes()[:nativeSize])
	}

	return out.Bytes(), nil
}

// reverseTranslateTextObject rewrites compat-format hex hashes on the
// specified header lines back to native-format hashes.
func (t *Translator) reverseTranslateTextObject(content []byte, hashFields []string) ([]byte, error) {
	compatHexSize := t.formats.Compat.HexSize()

	var out bytes.Buffer
	remaining := content
	headerDone := false

	for len(remaining) > 0 {
		nlIdx := bytes.IndexByte(remaining, '\n')
		var line []byte
		if nlIdx >= 0 {
			line = remaining[:nlIdx]
			remaining = remaining[nlIdx+1:]
		} else {
			line = remaining
			remaining = nil
		}

		if !headerDone {
			if len(line) == 0 {
				headerDone = true
				out.WriteByte('\n')
				continue
			}

			replaced := false
			for _, field := range hashFields {
				prefix := field + " "
				if bytes.HasPrefix(line, []byte(prefix)) && len(line) == len(prefix)+compatHexSize {
					hexStr := string(line[len(prefix):])
					compatHash, ok := plumbing.FromHex(hexStr)
					if !ok {
						return nil, fmt.Errorf("invalid hash on %s line: %q", field, hexStr)
					}

					nativeHash, err := t.mapping.CompatToNative(compatHash)
					if err != nil {
						return nil, fmt.Errorf("%s hash %s: no native mapping: %w", field, compatHash, err)
					}

					out.WriteString(prefix)
					out.WriteString(nativeHash.String()[:t.formats.Native.HexSize()])
					out.WriteByte('\n')
					replaced = true
					break
				}
			}

			if !replaced {
				out.Write(line)
				out.WriteByte('\n')
			}
		} else {
			out.Write(line)
			if nlIdx >= 0 {
				out.WriteByte('\n')
			}
		}
	}

	return out.Bytes(), nil
}

// ReverseTranslateContent takes object content in native format and returns
// it in compat format. This is the inverse of what TranslateObject does
// internally -- it rewrites hash references from native to compat format.
//
// This is needed for push operations where objects must be sent in the
// compat format to a server that uses that format.
func (t *Translator) ReverseTranslateContent(objType plumbing.ObjectType, nativeContent []byte) ([]byte, error) {
	switch objType {
	case plumbing.BlobObject:
		return nativeContent, nil
	case plumbing.TreeObject:
		return t.translateTree(nativeContent)
	case plumbing.CommitObject:
		return t.translateCommit(nativeContent)
	case plumbing.TagObject:
		return t.translateTag(nativeContent)
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objType)
	}
}

// ComputeNativeHash computes the native-format hash for raw content.
// This is a convenience method for callers that need to hash content
// that is already in native format.
func (t *Translator) ComputeNativeHash(objType plumbing.ObjectType, content []byte) (plumbing.Hash, error) {
	return t.nativeHasher.Compute(objType, content)
}

// ComputeCompatHash computes the compat-format hash for raw content.
func (t *Translator) ComputeCompatHash(objType plumbing.ObjectType, content []byte) (plumbing.Hash, error) {
	return t.compatHasher.Compute(objType, content)
}

// NativeObjectFormat returns the native object format.
func (t *Translator) NativeObjectFormat() format.ObjectFormat {
	return t.formats.Native
}

// CompatObjectFormat returns the compat object format.
func (t *Translator) CompatObjectFormat() format.ObjectFormat {
	return t.formats.Compat
}
