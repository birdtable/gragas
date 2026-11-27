package proof

import "github.com/birdtable/gragas/internal/model/plugin"

// Within is within over bare paths, for the table test of the folder's
// containment, whose rows hold another system's spellings.
func Within(dir string, paths []string) ([]string, error) {
	fs := make([]plugin.Finding, 0, len(paths))
	for _, p := range paths {
		fs = append(fs, plugin.Finding{Key: "k", Path: &p})
	}
	got, err := within(dir, fs)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(got))
	for _, f := range got {
		out = append(out, f.path)
	}
	return out, nil
}
