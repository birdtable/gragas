package main

import (
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"maps"
	"os"
	"slices"
)

// compareStep holds the checksum files of two builds from two clean clones to
// each other (supply.release.11): the same archives, each the same bytes. A
// difference names the first archive, by name, that one build made and the
// other did not or made with other bytes.
func compareStep(first, second string, out *say.Speaker) error {
	a, err := sumsOf(first)
	if err != nil {
		return err
	}
	b, err := sumsOf(second)
	if err != nil {
		return err
	}
	if err := compare(a, b); err != nil {
		return err
	}
	out.Text(fmt.Sprintf("compare: %d archives, byte for byte the same from two clean clones\n", len(a)))
	return nil
}

func sumsOf(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sums, _, err := readSums(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return sums, nil
}

// compare answers the first archive, in name order, on which two builds'
// digests differ, or nil when they are the same archives with the same bytes.
func compare(a, b map[string]string) error {
	both := maps.Clone(a)
	maps.Copy(both, b)
	for _, n := range slices.Sorted(maps.Keys(both)) {
		da, inA := a[n]
		db, inB := b[n]
		switch {
		case !inB:
			return fmt.Errorf("not reproducible: the first build made %s and the second did not", n)
		case !inA:
			return fmt.Errorf("not reproducible: the second build made %s and the first did not", n)
		case da != db:
			return fmt.Errorf("not reproducible: %s differs between the two builds: %s, then %s", n, da, db)
		}
	}
	return nil
}
