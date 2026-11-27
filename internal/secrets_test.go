package internal

// The plants of the secret scan (gragas#1156). Each case clones a small
// repository, puts a throwaway key made by the test into it, and runs the
// gate's own members through the Taskfile: task secrets-staged on the stage,
// task secrets on the commits past origin/main, and task secrets-scan with
// SCAN=HEAD (the history pass of secrets-full) and SCAN=tree (its tree pass).
// No test holds a copy of the command line: the Taskfile's secrets-scan is
// the one place it is written, and SECRETS_DIR points it at the clone.
//
// A private key is red at every path and in every pass, at the paths
// gitleaks' default allow-list exempts as well; so is one behind a
// gitleaks:allow comment, a -diff attribute, a NUL byte, a merge's own lines
// or a .gitleaksignore. A public key, a file git does not track and a clean
// clone are green. No report holds a line of a key's body. The key lives
// only in the test's temp folder and its memory, and a failure prints the
// output with every body line replaced.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// throwawayKey is a P-256 key made now: its private half as PKCS #8 PEM, its
// public half as PKIX PEM, and the private half's body lines.
func throwawayKey(t *testing.T) (private, public []byte, body []string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	private = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	public = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})
	for line := range strings.Lines(string(private)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "-----") {
			body = append(body, line)
		}
	}
	return private, public, body
}

// runSecrets runs one of the Taskfile's secret members over the repository
// dir, with the settings (NAME=value) after it, and returns its output and
// whether it succeeded.
func runSecrets(t *testing.T, dir, name string, settings ...string) (string, bool) {
	t.Helper()
	return runSecretsFrom(t, "", dir, name, settings...)
}

// runSecretsFrom is runSecrets with the Taskfile at taskfile, when it is not
// empty: a copy of the repository's own beside planted configs, which
// secrets-scan reads from the Taskfile's folder. The pinned tools stay the
// repository's, and the variables that read the repository's files with sh
// are given on the command line, so the copy needs no file but the configs.
func runSecretsFrom(t *testing.T, taskfile, dir, name string, settings ...string) (string, bool) {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(abs, ".tools", "bin")
	if _, err := os.Stat(pinnedPath(t, "gitleaks")); err != nil {
		t.Fatal("the pinned gitleaks is not installed: run task tools")
	}
	cmd := exec.CommandContext(t.Context(), "task")
	cmd.Path, cmd.Err = pinnedTask(t), nil // the clone's own, set after the call: the constant above is what the rule reads
	if taskfile != "" {
		// The copy's folder has no ./cmd/gragas-dev for GATE's go run, so the gate is the repository's (gragas#1655).
		cmd.Args = append(cmd.Args, "--taskfile", taskfile, "TOOLS="+bin, "SMOKE_RUN=^$", `GATE=go run -C "`+filepath.ToSlash(abs)+`" -trimpath ./cmd/gragas-dev`)
	}
	cmd.Args = append(cmd.Args, name, "SECRETS_DIR="+dir)
	cmd.Args = append(cmd.Args, settings...)
	cmd.Dir = abs
	cmd.Env = append(configtest.Inherited(), "PATH="+bin+string(os.PathListSeparator)+inheritedPath())
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// scanPass runs task secrets-scan over dir with SCAN set to scan: HEAD,
// tree or a revision range.
func scanPass(t *testing.T, dir, scan string) (string, bool) {
	t.Helper()
	return runSecrets(t, dir, "secrets-scan", "SCAN="+scan)
}

// scrub is the output with every line of the key's body replaced, so a
// failure can print it.
func scrub(out string, body []string) string {
	for _, b := range body {
		out = strings.ReplaceAll(out, b, "<a line of the key's body>")
	}
	return out
}

// secretsBase is a repository of one commit, the clones' origin/main.
func secretsBase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wideGit(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("a clean file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wideGit(t, dir, "add", "-A")
	wideGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

type secretsPlant struct {
	name string
	// path is the file the plant writes, relative to the clone.
	path string
	// write puts the file's bytes in place, given the clone and the key's
	// two halves; nil writes the private half as it is.
	write func(t *testing.T, dir string, private, public []byte)
	// merge stages the plant as part of a merge of a side branch, so only
	// the merge commit holds it.
	merge bool
	// untracked leaves the plant out of git: a .gitignore names it.
	untracked bool
	// green is what every pass must say; red ones must also name want.
	green bool
	want  string
}

func put(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	file := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func secretsPlants() []secretsPlant {
	// The comment gitleaks honours unless --ignore-gitleaks-allow, spelled
	// in two parts so this file holds no such comment of its own.
	allow := "gitleaks" + ":allow"
	plants := []secretsPlant{}
	// The paths of #1156: build output git ignores here, files gitleaks'
	// default allow-list exempts by name or extension, and its own config.
	for _, p := range []string{"tool", "site/dist/x", "go.sum", "a.svg", "a.png", ".gitleaks.toml"} {
		plants = append(plants, secretsPlant{name: p, path: p, want: "private-key"})
	}
	plants[len(plants)-1].write = func(t *testing.T, dir string, private, _ []byte) {
		t.Helper()
		put(t, dir, ".gitleaks.toml", []byte("x = '''\n"+string(private)+"'''\n"))
	}
	return append(plants,
		secretsPlant{name: "allow", path: "k.txt", want: "private-key", write: func(t *testing.T, dir string, private, _ []byte) {
			t.Helper()
			var b strings.Builder
			b.WriteString("# " + allow + "\n")
			for line := range strings.Lines(string(private)) {
				b.WriteString(strings.TrimSuffix(line, "\n") + " # " + allow + "\n")
			}
			put(t, dir, "k.txt", []byte(b.String()))
		}},
		secretsPlant{name: "diff attribute", path: "k.txt", want: "private-key", write: func(t *testing.T, dir string, private, _ []byte) {
			t.Helper()
			put(t, dir, ".gitattributes", []byte("*.txt -diff\n"))
			put(t, dir, "k.txt", private)
		}},
		secretsPlant{name: "NUL byte", path: "k.txt", want: "private-key", write: func(t *testing.T, dir string, private, _ []byte) {
			t.Helper()
			put(t, dir, "k.txt", append([]byte("\x00\n"), private...))
		}},
		secretsPlant{name: "merge", path: "k.txt", want: "private-key", merge: true},
		secretsPlant{name: "gitleaksignore", path: "k.txt", want: ".gitleaksignore", write: func(t *testing.T, dir string, private, _ []byte) {
			t.Helper()
			put(t, dir, "k.txt", private)
			put(t, dir, ".gitleaksignore", []byte("k.txt:private-key:1\n"))
		}},
		// gragas#2334: the default config's global allow-list skips any path holding gitleaks.toml, and the
		// second pass reads the private-key rule alone, so an access key at one was read by neither pass.
		// The scan refuses the path, in every pass, and names it.
		secretsPlant{name: "gitleaks.toml in a folder", path: "docs/gitleaks.toml", want: "docs/gitleaks.toml", write: func(t *testing.T, dir string, _, _ []byte) {
			t.Helper()
			put(t, dir, "docs/gitleaks.toml", []byte("aws_access_key_id = AKIA"+rand.Text()[:16]+"\n"))
		}},
		secretsPlant{name: "gitleaks.toml in the name", path: "k.gitleaks.toml.txt", want: "k.gitleaks.toml.txt", write: func(t *testing.T, dir string, _, _ []byte) {
			t.Helper()
			put(t, dir, "k.gitleaks.toml.txt", []byte("aws_access_key_id = AKIA"+rand.Text()[:16]+"\n"))
		}},
		secretsPlant{name: "a nested .gitleaks.toml", path: "docs/.gitleaks.toml", want: "docs/.gitleaks.toml", write: func(t *testing.T, dir string, _, _ []byte) {
			t.Helper()
			put(t, dir, "docs/.gitleaks.toml", []byte("aws_access_key_id = AKIA"+rand.Text()[:16]+"\n"))
		}},
		secretsPlant{name: "public key", path: "k.pub", green: true, write: func(t *testing.T, dir string, _, public []byte) {
			t.Helper()
			put(t, dir, "k.pub", public)
		}},
		secretsPlant{name: "untracked", path: "build/k", green: true, untracked: true},
		secretsPlant{name: "control", path: "notes.txt", green: true, write: func(t *testing.T, dir string, _, _ []byte) {
			t.Helper()
			put(t, dir, "notes.txt", []byte("a clean file\n"))
		}},
	)
}

func TestTheSecretScanIsRedOnAThrowawayKeyAtEveryPathInEveryPass(t *testing.T) {
	t.Parallel()
	base := secretsBase(t)
	for _, p := range secretsPlants() {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			private, public, body := throwawayKey(t)
			dir := filepath.Join(t.TempDir(), "clone")
			wideGit(t, filepath.Dir(dir), "clone", "-q", base, dir)
			if p.untracked {
				put(t, dir, ".gitignore", []byte("/build/\n"))
			}
			if p.merge {
				wideGit(t, dir, "switch", "-q", "-c", "side")
				put(t, dir, "side.txt", []byte("side\n"))
				wideGit(t, dir, "add", "side.txt")
				wideGit(t, dir, "commit", "-q", "-m", "side")
				wideGit(t, dir, "switch", "-q", "main")
				put(t, dir, "main.txt", []byte("main\n"))
				wideGit(t, dir, "add", "main.txt")
				wideGit(t, dir, "commit", "-q", "-m", "main")
				wideGit(t, dir, "merge", "-q", "--no-commit", "side")
			}
			if p.write != nil {
				p.write(t, dir, private, public)
			} else {
				put(t, dir, p.path, private)
			}
			wideGit(t, dir, "add", "-A")
			check := func(pass string, out string, ok bool) {
				t.Helper()
				for _, b := range body {
					if strings.Contains(out, b) {
						t.Errorf("%s: the report holds a line of the key's body", pass)
						break
					}
				}
				switch {
				case p.green && !ok:
					t.Errorf("%s: red, want green:\n%s", pass, scrub(out, body))
				case !p.green && ok:
					t.Errorf("%s: green, want red:\n%s", pass, scrub(out, body))
				case !p.green && !strings.Contains(out, p.want):
					t.Errorf("%s: red without naming %s:\n%s", pass, p.want, scrub(out, body))
				}
			}
			out, ok := runSecrets(t, dir, "secrets-staged")
			check("the stage", out, ok)
			wideGit(t, dir, "commit", "-q", "--allow-empty", "-m", "plant")
			out, ok = runSecrets(t, dir, "secrets")
			check("the commits past origin/main", out, ok)
			out, ok = scanPass(t, dir, "HEAD")
			check("the history", out, ok)
			out, ok = scanPass(t, dir, "tree")
			check("the tracked tree", out, ok)
		})
	}
}

// The tree pass reads a tracked file as it is in the folder, staged or not,
// and only the history pass reads a key that a later commit deleted.
func TestTheSecretScanReadsAnUnstagedEditAndADeletedKey(t *testing.T) {
	t.Parallel()
	base := secretsBase(t)
	private, _, body := throwawayKey(t)
	dir := filepath.Join(t.TempDir(), "clone")
	wideGit(t, filepath.Dir(dir), "clone", "-q", base, dir)
	put(t, dir, "README", private)
	out, ok := scanPass(t, dir, "tree")
	if ok || !strings.Contains(out, "private-key") {
		t.Errorf("an unstaged edit of a tracked file: ok %v, want red naming private-key:\n%s", ok, scrub(out, body))
	}
	wideGit(t, dir, "add", "README")
	wideGit(t, dir, "commit", "-q", "-m", "plant")
	put(t, dir, "README", []byte("a clean file\n"))
	wideGit(t, dir, "commit", "-q", "-a", "-m", "unplant")
	if out, ok := scanPass(t, dir, "tree"); !ok {
		t.Errorf("the tree after the key's deletion: red, want green:\n%s", scrub(out, body))
	}
	if out, ok := scanPass(t, dir, "HEAD"); ok || !strings.Contains(out, "private-key") {
		t.Errorf("the history after the key's deletion: ok %v, want red naming private-key:\n%s", ok, scrub(out, body))
	}
}

// A pass that cannot read the whole history is red, not an empty green: a
// shallow clone, a grafts file, and a range git cannot read, whose error
// gitleaks logs and then exits 0 on.
func TestTheSecretScanIsRedOnAHistoryItCannotReadWhole(t *testing.T) {
	t.Parallel()
	base := secretsBase(t)
	wideGit(t, base, "commit", "-q", "--allow-empty", "-m", "second")
	shallow := filepath.Join(t.TempDir(), "shallow")
	wideGit(t, filepath.Dir(shallow), "clone", "-q", "--depth", "1", "file://"+filepath.ToSlash(base), shallow)
	if out, ok := scanPass(t, shallow, "HEAD"); ok || !strings.Contains(out, "shallow clone") {
		t.Errorf("a shallow clone: ok %v, want red naming it:\n%s", ok, out)
	}
	grafts := filepath.Join(t.TempDir(), "grafts")
	wideGit(t, filepath.Dir(grafts), "clone", "-q", base, grafts)
	put(t, grafts, ".git/info/grafts", []byte{})
	if out, ok := scanPass(t, grafts, "HEAD"); ok || !strings.Contains(out, "grafts file") {
		t.Errorf("a grafts file: ok %v, want red naming it:\n%s", ok, out)
	}
	if out, ok := scanPass(t, base, "no-such-ref..HEAD"); ok || !strings.Contains(out, "cannot be trusted") {
		t.Errorf("a range git cannot read: ok %v, want red saying the pass cannot be trusted:\n%s", ok, out)
	}
}

// The count main's internal/dev/scan held: gitleaks must say it scanned as
// many commits as git's own log of the range holds with a hunk. A history of
// commits gitleaks does not count (an empty commit, an empty file, a mode
// change, a delete) stays green. A repository whose config sets
// format.pretty is red in every pass with no key in it: gitleaks then
// counts no commit, writes no error and exits 0, so only the count can say
// the pass did not scan what git holds.
// The full scan reads HEAD's history, not every ref of the clone: a key on a
// local branch and a tag that HEAD does not hold leaves it green, as a
// runner's unrelated branches must (gitleaks with no --log-opts reads git
// log --all). Two controls: the same clone read over --all is red, so the
// plant is one the scan finds, and once HEAD merges the branch, the full
// scan is red.
func TestTheFullSecretScanReadsHeadsHistoryAndNoOtherRef(t *testing.T) {
	t.Parallel()
	base := secretsBase(t)
	private, _, body := throwawayKey(t)
	dir := filepath.Join(t.TempDir(), "clone")
	wideGit(t, filepath.Dir(dir), "clone", "-q", base, dir)
	wideGit(t, dir, "switch", "-q", "-c", "elsewhere")
	put(t, dir, "plant.pem", private)
	wideGit(t, dir, "add", "plant.pem")
	wideGit(t, dir, "commit", "-q", "-m", "a key on another branch")
	wideGit(t, dir, "tag", "elsewhere-tag")
	wideGit(t, dir, "switch", "-q", "main")
	put(t, dir, "main.txt", []byte("main\n"))
	wideGit(t, dir, "add", "main.txt")
	wideGit(t, dir, "commit", "-q", "-m", "main")
	if out, ok := runSecrets(t, dir, "secrets-full"); !ok {
		t.Errorf("a key only on a branch and a tag HEAD does not hold: red, want green:\n%s", scrub(out, body))
	}
	if out, ok := scanPass(t, dir, "--all"); ok || !strings.Contains(out, "plant.pem") {
		t.Errorf("the same clone read over every ref, as gitleaks reads it with no --log-opts: ok %v, want red naming plant.pem:\n%s", ok, scrub(out, body))
	}
	wideGit(t, dir, "merge", "-q", "--no-edit", "elsewhere")
	out, ok := runSecrets(t, dir, "secrets-full")
	if ok || !strings.Contains(out, "plant.pem") {
		t.Errorf("the control, the branch merged into HEAD: ok %v, want red naming plant.pem:\n%s", ok, scrub(out, body))
	}
	for _, b := range body {
		if strings.Contains(out, b) {
			t.Error("the report holds a line of the key's body")
			break
		}
	}
}

func TestTheSecretScanIsRedWhenGitleaksScansFewerCommitsThanGitsLogHolds(t *testing.T) {
	t.Parallel()
	base := secretsBase(t)
	dir := filepath.Join(t.TempDir(), "clone")
	wideGit(t, filepath.Dir(dir), "clone", "-q", base, dir)
	wideGit(t, dir, "commit", "-q", "--allow-empty", "-m", "an empty commit")
	put(t, dir, "empty.txt", nil)
	wideGit(t, dir, "add", "empty.txt")
	wideGit(t, dir, "commit", "-q", "-m", "an empty file")
	wideGit(t, dir, "update-index", "--chmod=+x", "empty.txt")
	wideGit(t, dir, "commit", "-q", "-m", "a mode")
	wideGit(t, dir, "rm", "-q", "-f", "empty.txt")
	wideGit(t, dir, "commit", "-q", "-m", "a delete")
	for _, scan := range []string{"HEAD", "tree"} {
		if out, ok := scanPass(t, dir, scan); !ok {
			t.Errorf("SCAN=%s over commits that hold no hunk: red, want green:\n%s", scan, out)
		}
	}
	put(t, dir, "plant.txt", []byte("a clean line\n"))
	wideGit(t, dir, "add", "plant.txt")
	wideGit(t, dir, "commit", "-q", "-m", "a clean file")
	wideGit(t, dir, "config", "format.pretty", "oneline")
	for _, scan := range []string{"HEAD", "tree"} {
		out, ok := scanPass(t, dir, scan)
		if ok || !strings.Contains(out, "commits git's log holds") {
			t.Errorf("SCAN=%s under format.pretty: ok %v, want red on the count:\n%s", scan, ok, out)
		}
		if strings.Contains(out, "leaks found:") {
			t.Errorf("SCAN=%s under format.pretty holds a finding, so it is not red on the count alone:\n%s", scan, out)
		}
	}
}
