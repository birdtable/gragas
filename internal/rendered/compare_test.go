package rendered_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/rendered"
)

func readFrom(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		s, ok := m[p]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(s), nil
	}
}

func TestCompareNamesTheFilesThatDifferAndAMissingOne(t *testing.T) {
	t.Parallel()
	files := []rendered.File{{Path: "a", Data: []byte("1")}, {Path: "b", Data: []byte("2")}, {Path: "c", Data: nil}, {Path: "d", Data: []byte("4")}}
	got, err := rendered.Compare(files, readFrom(map[string]string{"a": "1", "b": "x", "d": "4"}))
	if err != nil || !slices.Equal(got, []string{"b", "c"}) {
		t.Errorf("got %v, %v; want b and c, a missing empty file included", got, err)
	}
}

func TestCompareLineEndsDiffer(t *testing.T) {
	t.Parallel()
	got, err := rendered.Compare([]rendered.File{{Path: "a", Data: []byte("x\ny\n")}}, readFrom(map[string]string{"a": "x\r\ny\r\n"}))
	if err != nil || !slices.Equal(got, []string{"a"}) {
		t.Errorf("got %v, %v; a file that differs only in line ends differs", got, err)
	}
}

func TestCompareWithNoRenderedFileRefuses(t *testing.T) {
	t.Parallel()
	for _, files := range [][]rendered.File{nil, {}} {
		if _, err := rendered.Compare(files, readFrom(nil)); !errors.Is(err, rendered.ErrNoFile) {
			t.Errorf("err = %v, want ErrNoFile", err)
		}
	}
}

func TestCompareAnswersAReadErrorAndNotDiffers(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	got, err := rendered.Compare([]rendered.File{{Path: "a", Data: []byte("1")}}, func(string) ([]byte, error) { return nil, boom })
	if !errors.Is(err, boom) || got != nil {
		t.Errorf("got %v, %v; want the read error and no path", got, err)
	}
}
