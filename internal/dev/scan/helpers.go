package scan

// plantGit is git's arguments with the stand-in of gate/standins.txt as the
// committer, and no signing.
func plantGit(args ...string) []string {
	return append([]string{"-c", "user.name=alex", "-c", "user.email=alex@example.com", "-c", "commit.gpgsign=false"}, args...)
}

// firstLine is the first line of b, for a message.
func firstLine(b []byte) string {
	s := string(b)
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
