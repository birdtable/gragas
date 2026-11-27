package tool

// The words and the paths of a plugin's step: a
// step names its program by its grant and gives its words and its paths as
// data, and this file is the one reading of them. Nothing here looks at a
// disk.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/birdtable/go-files/confine"

	"github.com/birdtable/gragas/internal/port"
)

const (
	// wordScratch is the one word that names the scratch folder, alone or
	// as the start of a path in it.
	wordScratch = "{scratch}"
	// wordNull is the one word that names the system's null device.
	wordNull = "{null}"
)

// stepWords is a step's words as the program is given them. Each word is
// literal, but for two: {scratch} is the scratch folder, and a word that
// starts with {scratch}/ is the path in it that follows, in the system's own
// form; {null} is the system's null device, os.DevNull, so no system's name
// is written in gragas. Any other {, in any place of any word, is refused,
// matching port.ErrRefused, so no word reaches a placeholder a later contract
// may give a meaning. scratch is the absolute scratch folder.
func stepWords(argv []string, scratch string) ([]string, error) {
	words := make([]string, 0, len(argv))
	for i, w := range argv {
		got, err := stepWord(w, scratch)
		if err != nil {
			return nil, fmt.Errorf("word %d of the step, %q: %w", i, w, err)
		}
		words = append(words, got)
	}
	return words, nil
}

// stepWord is one word of stepWords.
func stepWord(w, scratch string) (string, error) {
	switch {
	case w == wordScratch:
		return scratch, nil
	case w == wordNull:
		return os.DevNull, nil
	case strings.HasPrefix(w, wordScratch+"/"):
		rest := strings.TrimPrefix(w, wordScratch+"/")
		if strings.Contains(rest, "{") {
			return "", fmt.Errorf("a { past %s is no placeholder: %w", wordScratch, port.ErrRefused)
		}
		local, err := stepPath(rest)
		if err != nil {
			return "", err
		}
		return filepath.Join(scratch, local), nil
	case strings.Contains(w, "{"):
		return "", fmt.Errorf("a word holds a { that is neither %s nor %s, alone or as %s/ at its start: %w", wordScratch, wordNull, wordScratch, port.ErrRefused)
	}
	return w, nil
}

// stepPath is a path a step places, collects or reads, in the system's own
// form below the scratch folder: relative, slash-separated, with no . or ..
// step, no backslash, colon or NUL, as go-files' confine.Local holds it, and
// not the scratch folder itself. Anything else is refused, matching
// port.ErrRefused. The path is then opened below the scratch folder through
// confine, so a link on the way leads nowhere out of it.
func stepPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("a path of the step is empty: %w", port.ErrRefused)
	}
	local, err := confine.Local(p)
	if err != nil {
		return "", fmt.Errorf("the path %q of the step is not a relative, slash-separated path that stays below the scratch folder: %w: %w", p, port.ErrRefused, err)
	}
	return local, nil
}
