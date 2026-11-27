package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/birdtable/go-proc/proc"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/digest"
)

// The most a fetched archive or a program may weigh; more is refused.
const maxProgram = 256 << 20

// toolPin is what tools.yaml pins of one release tool on one platform: the
// archive it is fetched as, the file in it, and the sha256 of each.
type toolPin struct {
	URL     string
	Archive string
	Member  string
	SHA256  string
}

// runFetch is `fetch <id>`: the release tool id of tools.yaml, which upstream
// ships as a program and does not support building (golangci-lint), placed in
// .tools/bin. Its one pin is tools.yaml's: the archive's sha256 is held before
// the archive is opened, the program's before it is placed, and a program
// already there with the pinned sha256 is left as it is. With --check it
// fetches nothing and fails unless the program in .tools/bin is the pinned one.
func runFetch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("fetch", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the repository holding tools.yaml")
	check := fl.Bool("check", false, "fetch nothing; fail unless the program in .tools/bin is the pinned one")
	platform := fl.String("platform", "", "the system, goos/goarch, whose pin is read")
	rest, err := parseInterspersed(fl, args)
	if err != nil || len(rest) != 1 || *platform == "" {
		talk.Text(usage)
		return exitUsage
	}
	out := &say.Speaker{W: stdout}
	if err := fetchTool(ctx, fetchJob{dir: *dir, id: rest[0], platform: *platform, check: *check, get: FetchHTTPS}, out); err != nil {
		talk.Text("gragas-dev fetch: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// parseInterspersed parses fl over args, with its flags before or after the
// one word the command takes, and answers the words that are not flags.
func parseInterspersed(fl *flag.FlagSet, args []string) ([]string, error) {
	var words []string
	for {
		if err := fl.Parse(args); err != nil {
			return nil, err
		}
		if fl.NArg() == 0 {
			return words, nil
		}
		words = append(words, fl.Arg(0))
		args = fl.Args()[1:]
	}
}

// fetchJob is one fetch: the repository, the tool, the platform, and whether it
// only checks. get is the download, which a test serves.
type fetchJob struct {
	dir, id, platform string
	// pins is the file pinning the release tools and bin the folder they are placed in, each read from
	// dir when it is not absolute; empty is tools.yaml and .tools/bin.
	pins, bin string
	check     bool
	get       func(context.Context, string) ([]byte, error)
}

// fetchTool is runFetch over a fetchJob.
func fetchTool(ctx context.Context, j fetchJob, out *say.Speaker) error {
	pins, bin := j.pins, j.bin
	if pins == "" {
		pins = "tools.yaml"
	}
	if bin == "" {
		bin = filepath.Join(".tools", "bin")
	}
	data, err := os.ReadFile(under(j.dir, pins))
	if err != nil {
		return err
	}
	pin, err := readToolPin(data, j.id, j.platform)
	if err != nil {
		return err
	}
	dst := filepath.Join(under(j.dir, bin), programName(j.id, j.platform))
	if have, err := os.ReadFile(dst); err == nil && digest.Hex(have) == pin.SHA256 {
		out.Text("fetch: " + j.id + " in .tools/bin is the pinned program\n")
		return nil
	}
	if j.check {
		return fmt.Errorf("%s in .tools/bin is missing or is not the program tools.yaml pins for %s (sha256 %s): run task tools", j.id, j.platform, pin.SHA256)
	}
	archive, err := j.get(ctx, pin.URL)
	if err != nil {
		return fmt.Errorf("%s: %w", pin.URL, err)
	}
	if got := digest.Hex(archive); got != pin.Archive {
		return fmt.Errorf("refused %s: the archive has sha256 %s and tools.yaml pins %s", pin.URL, got, pin.Archive)
	}
	program, err := unpackMember(archive, pin.URL, pin.Member)
	if err != nil {
		return fmt.Errorf("refused %s: %w", pin.URL, err)
	}
	if got := digest.Hex(program); got != pin.SHA256 {
		return fmt.Errorf("refused %s: %s has sha256 %s and tools.yaml pins %s", pin.URL, pin.Member, got, pin.SHA256)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := writeProgram(dst, program); err != nil {
		return err
	}
	out.Text("fetch: " + j.id + " placed in .tools/bin, the program tools.yaml pins for " + j.platform + "\n")
	return nil
}

// programName is the name id is placed under in .tools/bin for platform, a
// goos/goarch pair: the program's file name on that system, proc.FileName's
// (gragas#1088).
func programName(id, platform string) string {
	goos, _, _ := strings.Cut(platform, "/")
	return proc.FileName(goos, id)
}

// readToolPin is the pin of the release tool id on platform in tools.yaml's
// text, read line by line: a tool is a "  - id:" item of tools:, a platform a
// six-space key under its platforms:, and its four fields eight-space keys.
func readToolPin(data []byte, id, platform string) (toolPin, error) {
	var in, release, plat bool
	var pin toolPin
	var found bool
loop:
	for line := range strings.SplitSeq(string(data), "\n") {
		trim := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case strings.HasPrefix(line, "  - id: "):
			if in {
				break loop
			}
			in, release, plat = strings.TrimSpace(strings.TrimPrefix(line, "  - id: ")) == id, false, false
		case !in || trim == "":
		case indent == 4 && trim == "from: release":
			release = true
		case indent == 6 && strings.HasSuffix(trim, ":"):
			plat = strings.TrimSuffix(trim, ":") == platform
		case indent == 8 && plat:
			k, v, _ := strings.Cut(trim, ": ")
			found = true
			switch k {
			case "url":
				pin.URL = v
			case "archive":
				pin.Archive = v
			case "member":
				pin.Member = v
			case "sha256":
				pin.SHA256 = v
			}
		}
	}
	if found && !release {
		return toolPin{}, fmt.Errorf("%s is not a release tool of tools.yaml", id)
	}
	if !found {
		return toolPin{}, fmt.Errorf("tools.yaml pins no program of %s for %s", id, platform)
	}
	if pin.URL == "" || pin.Archive == "" || pin.Member == "" || pin.SHA256 == "" {
		return toolPin{}, fmt.Errorf("tools.yaml pins no complete program of %s for %s", id, platform)
	}
	return pin, nil
}

// FetchHTTPS downloads url, refusing anything but https, on every redirect
// too, and anything heavier than maxProgram.
func FetchHTTPS(ctx context.Context, url string) (body []byte, err error) {
	client := &http.Client{Timeout: 10 * time.Minute, CheckRedirect: httpsRedirect}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("refused %s, which is not https", url)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxProgram+1))
	if err == nil && len(b) > maxProgram {
		return nil, fmt.Errorf("refused: more than %d bytes", maxProgram)
	}
	return b, err
}

// httpsRedirect is a fetch's redirect check: each hop is https, and there
// are at most five.
func httpsRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refused a redirect to %s, which is not https", req.URL.Redacted())
	}
	if len(via) >= 5 {
		return errors.New("more than five redirects")
	}
	return nil
}

// unpackMember is the one regular file named member in the archive fetched
// from url: a .tar.gz, or a .zip, the form every windows release and ast-grep
// ship in (gragas#1088). member may sit in a folder, as node's and
// golangci-lint's do; an archive of any other form is refused.
func unpackMember(archive []byte, url, member string) ([]byte, error) {
	switch {
	case strings.HasSuffix(url, ".tar.gz"):
		return unpackTarGz(archive, member)
	case strings.HasSuffix(url, ".zip"):
		return unpackZip(archive, member)
	}
	return nil, errors.New("the archive is neither a .tar.gz nor a .zip")
}

// unpackTarGz is the one regular file named member in a .tar.gz archive.
func unpackTarGz(archive []byte, member string) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("no such file in the archive")
		}
		if err != nil {
			return nil, err
		}
		if h.Name != member && h.Name != "./"+member {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size > maxProgram {
			return nil, fmt.Errorf("%s is not a regular file of at most %d bytes", h.Name, maxProgram)
		}
		return io.ReadAll(io.LimitReader(tr, maxProgram))
	}
}

// unpackZip is the first file named member in a .zip archive, refused unless
// it is a regular file of at most maxProgram bytes, whatever size its header
// claims. The program's sha256 is held after, so a second file of the same
// name changes nothing that is placed.
func unpackZip(archive []byte, member string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != member && f.Name != "./"+member {
			continue
		}
		if !f.Mode().IsRegular() || f.UncompressedSize64 > maxProgram {
			return nil, fmt.Errorf("%s is not a regular file of at most %d bytes", f.Name, maxProgram)
		}
		return readZipFile(f)
	}
	return nil, errors.New("no such file in the archive")
}

// readZipFile is f's bytes, refused past maxProgram; reading it to its end
// holds its CRC-32.
func readZipFile(f *zip.File) (body []byte, err error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rc.Close()) }()
	b, err := io.ReadAll(io.LimitReader(rc, maxProgram+1))
	if err == nil && len(b) > maxProgram {
		return nil, fmt.Errorf("%s is more than %d bytes", f.Name, maxProgram)
	}
	return b, err
}
