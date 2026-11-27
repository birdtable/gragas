package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"strconv"
	"strings"
	"testing"
)

// tarGzOfHeaderAlone is a .tar.gz holding one regular file whose header says size bytes
// and whose body is empty: the check of the header is all it reaches.
func tarGzOfHeaderAlone(t *testing.T, member string, size int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: member, Typeflag: tar.TypeReg, Size: size}); err != nil {
		t.Fatal(err)
	}
	// the tar writer is not closed, which would refuse a member short of its
	// size: the archive ends where the body should be
	if err := zw.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipOfHeaderAlone is a .zip holding one stored file whose header says size bytes
// and whose body is empty.
func zipOfHeaderAlone(t *testing.T, member string, size uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateRaw(&zip.FileHeader{Name: member, Method: zip.Store, UncompressedSize64: size})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(nil); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAProgramOfMaxProgramBytesIsReadAndOneByteMoreIsRefused (boundary: maxProgram):
// a .tar.gz or a .zip member whose header says maxProgram bytes passes the size
// check of unpackMember, which then reads to the end of an archive that holds
// none, and one whose header says maxProgram+1 is refused by that check,
// naming the cap, so the refusal is the cap's. The body is never made: the
// header is what the check reads, so the test allocates nothing near the cap.
// The cap of FetchHTTPS, the same constant on a https body, is not held here:
// its client takes no server of the test's, and the cap of the read of a zip
// member past its header (readZipFile) is not either, as it needs a body of
// the cap's size.
func TestAProgramOfMaxProgramBytesIsReadAndOneByteMoreIsRefused(t *testing.T) {
	t.Parallel()
	refusal := "is not a regular file of at most " + strconv.Itoa(maxProgram) + " bytes"
	for name, c := range map[string]struct {
		url  string
		with func(size int64) []byte
	}{
		"a .tar.gz": {"https://example.test/tool.tar.gz", func(n int64) []byte { return tarGzOfHeaderAlone(t, "tool", n) }},
		"a .zip":    {"https://example.test/tool.zip", func(n int64) []byte { return zipOfHeaderAlone(t, "tool", uint64(n)) }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := unpackMember(c.with(maxProgram), c.url, "tool"); err == nil || strings.Contains(err.Error(), refusal) {
				t.Errorf("a header of maxProgram bytes: %v, want the end of the archive and no refusal of the size", err)
			}
			if _, err := unpackMember(c.with(maxProgram+1), c.url, "tool"); err == nil || !strings.Contains(err.Error(), refusal) {
				t.Errorf("a header of maxProgram+1 bytes: %v, want it refused as %q", err, refusal)
			}
		})
	}
}
