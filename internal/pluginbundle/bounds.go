package pluginbundle

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// ErrOverBound is what a refusal of a file or a bundle over its bound matches:
// the reader's, which reads at most a bound plus one byte of a file, and the
// writer's, which refuses before it writes the same bytes.
var ErrOverBound = errors.New("the file is over its bound")

// Bounds are the sizes a bundle is held to by whoever reads it or adds it, so
// a bundle added is one the reader does not refuse.
type Bounds struct {
	// Module is the most bytes plugin.wasm may hold.
	Module int64
	// File is the most bytes any other file may hold.
	File int64
	// Total is the most bytes the whole bundle, every file, may hold; no more
	// than MaxBundle.
	Total int64
	// Lock is the most bytes the lock may hold.
	Lock int64
}

// Held is b with Total no more than MaxBundle.
func (b Bounds) Held() Bounds {
	b.Total = min(b.Total, MaxBundle)
	return b
}

// Own is the most bytes the file at rel, below its bundle's folder, may hold:
// the manifest's bound, the module's or any other file's.
func (b Bounds) Own(rel string) int64 {
	switch rel {
	case ManifestFile:
		return MaxManifest
	case ModuleFile:
		return b.Module
	}
	return b.File
}

// Holds refuses the files of a bundle that are over a bound: a file over its
// own, or all of them over the total. Each refusal matches ErrOverBound and
// names the file, or says it is the bundle's total.
func (b Bounds) Holds(files []model.File) error {
	var total int64
	for _, f := range files {
		if n := int64(len(f.Data)); n > b.Own(f.Path) {
			return fmt.Errorf("%s is %d bytes, over the %d it may hold: %w", f.Path, n, b.Own(f.Path), ErrOverBound)
		}
		total += int64(len(f.Data))
	}
	if total > b.Total {
		return fmt.Errorf("the bundle holds %d bytes, over the %d a bundle may hold: %w", total, b.Total, ErrOverBound)
	}
	return nil
}

// BundleOf is the bundle the files make, as Judge takes it: the two named
// files apart, the rest by path, and the lock's digest and the read's both the
// sum of the files. Judge refuses a file twice and either named file missing.
func BundleOf(files []model.File) model.PluginBundle {
	var b model.PluginBundle
	for _, f := range files {
		switch f.Path {
		case ManifestFile:
			b.Manifest = f.Data
		case ModuleFile:
			b.Module = f.Data
		default:
			b.Other = append(b.Other, f)
		}
	}
	slices.SortFunc(b.Other, func(x, y model.File) int { return strings.Compare(x.Path, y.Path) })
	b.Locked = Digest(files)
	b.Read = b.Locked
	return b
}

// DefaultBounds are the sizes the product holds a bundle to: a module of at
// most 32 MiB, any other file of 8 MiB, the whole bundle MaxBundle and the lock
// 1 MiB. The reader and the add plan both take them from here.
func DefaultBounds() Bounds {
	return Bounds{Module: 32 << 20, File: 8 << 20, Total: MaxBundle, Lock: 1 << 20}
}
