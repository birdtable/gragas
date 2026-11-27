package main

import (
	"io"
	"os"
)

// main wraps Serve: one request on stdin, one response on stdout, exit 0.
// Stderr is a log and this writes none. A stdin that cannot be
// read is answered as no request at all, which Serve answers with the plugin's
// own error, so the program never exits on its own: it is a program for the wasm
// target, whose descriptors 0 and 1 are stdin and stdout.
func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		in = nil
	}
	if _, err := os.NewFile(1, "stdout").Write(Serve(in)); err != nil {
		return
	}
}
