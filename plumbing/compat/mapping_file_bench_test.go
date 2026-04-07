package compat

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6/plumbing"
)

func BenchmarkFileMappingLookup(b *testing.B) {
	const entries = 50000

	for _, hexWidth := range []int{40, 64} {
		b.Run(fmt.Sprintf("hex-%d", hexWidth), func(b *testing.B) {
			fs := osfs.New(b.TempDir(), osfs.WithBoundOS())
			if err := fs.MkdirAll("objects", 0755); err != nil {
				b.Fatal(err)
			}

			f, err := fs.OpenFile("objects/"+looseObjectIdxFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < entries; i++ {
				native := plumbing.NewHash(fmt.Sprintf("%0*x", hexWidth, i+1))
				compat := plumbing.NewHash(fmt.Sprintf("%0*x", hexWidth, i+entries+1))
				if _, err := fmt.Fprintf(f, "%s %s\n", native, compat); err != nil {
					_ = f.Close()
					b.Fatal(err)
				}
			}
			if err := f.Close(); err != nil {
				b.Fatal(err)
			}

			targetNative := plumbing.NewHash(fmt.Sprintf("%0*x", hexWidth, entries))
			targetCompat := plumbing.NewHash(fmt.Sprintf("%0*x", hexWidth, entries*2))

			b.Run("native-to-compat/cached", func(b *testing.B) {
				m := NewFileMapping(fs, "objects")
				if _, err := m.NativeToCompat(targetNative); err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := m.NativeToCompat(targetNative); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("native-to-compat/new-mapping-first-lookup", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					m := NewFileMapping(fs, "objects")
					if _, err := m.NativeToCompat(targetNative); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("compat-to-native/cached", func(b *testing.B) {
				m := NewFileMapping(fs, "objects")
				if _, err := m.CompatToNative(targetCompat); err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := m.CompatToNative(targetCompat); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("compat-to-native/new-mapping-first-lookup", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					m := NewFileMapping(fs, "objects")
					if _, err := m.CompatToNative(targetCompat); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
