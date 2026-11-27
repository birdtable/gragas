package journeys

import "testing"

// genFixture is the conformance fixture's base book under rulebook/, a
// repository gen reads in format 3. With current it holds the rules.json
// the book makes (written by gen itself, in a run of its own), and without it
// holds none, so gen has the file to write.
func genFixture(t *testing.T, current bool) string {
	t.Helper()
	files := map[string]string{}
	for name, body := range fixtureCopy(t, "base") {
		files["rulebook/"+name] = body
	}
	dir := rulebookFrom(t, files)
	if current {
		env, _ := folders(t)
		if r := run(t, dir, env, "gen", "--apply"); r.code != 0 {
			t.Fatalf("make rules.json: exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	}
	return dir
}
