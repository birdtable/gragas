package format

import (
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// V2Folder is the folder of a root that holds the old layout, as the files
// below it name it: the root itself, or its rulebook/ folder. "-" says none
// does, and the files are those of the folder, relative to it.
func V2Folder(entries []model.Entry) (string, []string) {
	for _, dir := range []string{"", "rulebook"} {
		var rel []string
		for _, e := range entries {
			if e.Dir {
				continue
			}
			if p, ok := Below(dir, e.Path); ok {
				rel = append(rel, p)
			}
		}
		if IsV2(rel) {
			return dir, rel
		}
	}
	return "-", nil
}

// Below is path relative to dir, and whether it is inside it; "" is the root.
func Below(dir, path string) (string, bool) {
	if dir == "" {
		return path, true
	}
	return strings.CutPrefix(path, dir+"/")
}

// InFolder is path put inside dir; "" is the root.
func InFolder(dir, path string) string {
	if dir == "" {
		return path
	}
	return dir + "/" + path
}
