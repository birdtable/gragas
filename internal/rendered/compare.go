package rendered

import (
	"bytes"
	"errors"
	"io/fs"
)

// ErrNoFile is the answer of Compare when it is given no rendered file: a
// compare of nothing would end clean and say nothing.
var ErrNoFile = errors.New("no rendered file to compare")

// File is one rendered file: its path, slash-separated and relative to the
// root the read function reads, and its bytes.
type File struct {
	Path string
	Data []byte
}

// Compare answers the paths of the files whose bytes differ from what read
// answers for the path, in the order of files. A file that read answers an
// error matching fs.ErrNotExist for differs, since it is not there, even when
// its Data is empty. Any other error of read ends the compare at once and is
// the answer, with no path: an unreadable file is not a file that differs.
// The compare is on bytes, never on text, so a file that differs only in its
// line ends (\r\n for \n) differs. With no file it answers ErrNoFile.
//
// It holds no path of its own and names no command or folder, and it does not
// call Splice.
func Compare(files []File, read func(path string) ([]byte, error)) ([]string, error) {
	if len(files) == 0 {
		return nil, ErrNoFile
	}
	var differ []string
	for _, f := range files {
		old, err := read(f.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			differ = append(differ, f.Path)
		case err != nil:
			return nil, err
		case !bytes.Equal(old, f.Data):
			differ = append(differ, f.Path)
		}
	}
	return differ, nil
}
