package internal

// gosecFamilyPlants are the plants of the rule families past the general one:
// injection (G2), file and path (G3), cryptography (G4), blocked imports (G5),
// memory (G6) and taint (G7).
func gosecFamilyPlants() map[string]gosecPlant {
	return map[string]gosecPlant{
		"G201": {
			planted: `import (
	"database/sql"
	"fmt"
	"os"
)

// Find builds its query from the first argument.
func Find() error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}
	q := fmt.Sprintf("SELECT id FROM t WHERE name = '%s'", os.Args[1])
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	return rows.Close()
}
`,
			clean: `import (
	"database/sql"
	"os"
)

// Find passes the first argument as a parameter.
func Find() error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}
	q := "SELECT id FROM t WHERE name = ?"
	rows, err := db.Query(q, os.Args[1])
	if err != nil {
		return err
	}
	return rows.Close()
}
`,
		},
		"G202": {
			planted: `import (
	"database/sql"
	"os"
)

// Find joins the first argument into its query.
func Find() error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}
	q := "SELECT id FROM t WHERE name = '" + os.Args[1] + "'"
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	return rows.Close()
}
`,
			clean: `import (
	"database/sql"
	"os"
)

// Find passes the first argument as a parameter.
func Find() error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}
	q := "SELECT id FROM t WHERE name = ?"
	rows, err := db.Query(q, os.Args[1])
	if err != nil {
		return err
	}
	return rows.Close()
}
`,
		},
		"G203": {
			planted: `import (
	"html/template"
)

// Raw is s as markup, unescaped.
func Raw(s string) template.HTML { return template.HTML(s) }
`,
			clean: `import (
	"html/template"
)

// Escaped is s escaped for markup.
func Escaped(s string) string { return template.HTMLEscapeString(s) }
`,
		},
		"G204": {
			planted: `import (
	"context"
	"os/exec"
)

// Go runs go with args.
func Go(ctx context.Context, args ...string) error {
	return exec.CommandContext(ctx, "go", args...).Run()
}
`,
			clean: `import (
	"context"
	"os/exec"
)

// Go runs go version.
func Go(ctx context.Context) error {
	return exec.CommandContext(ctx, "go", "version").Run()
}
`,
		},
		"G301": {
			planted: `import (
	"os"
)

// Make makes dir, open to all.
func Make(dir string) error { return os.MkdirAll(dir, 0o777) }
`,
			clean: `import (
	"os"
)

// Make makes dir, for its owner and group.
func Make(dir string) error { return os.MkdirAll(dir, 0o750) }
`,
		},
		"G302": {
			planted: `import (
	"os"
)

// Open opens p to all.
func Open(p string) error { return os.Chmod(p, 0o777) }
`,
			clean: `import (
	"os"
)

// Close closes p to all but its owner.
func Close(p string) error { return os.Chmod(p, 0o600) }
`,
		},
		"G303": {
			planted: `import (
	"os"
)

// Report makes the report at a fixed path.
func Report() (*os.File, error) { return os.Create("/t` + `mp/report") }
`,
			clean: `import (
	"os"
)

// Report makes the report at a new temporary path.
func Report() (*os.File, error) { return os.CreateTemp("", "report-*") }
`,
		},
		"G304": {
			planted: `import (
	"os"
)

// Read reads p.
func Read(p string) ([]byte, error) { return os.ReadFile(p) }
`,
			clean: `import (
	"os"
)

// Read reads the settings.
func Read() ([]byte, error) { return os.ReadFile("settings.yaml") }
`,
		},
		"G305": {
			planted: `import (
	"archive/zip"
	"path/filepath"
)

// Paths is where each file of r would be written below dir.
func Paths(r *zip.Reader, dir string) []string {
	var out []string
	for _, f := range r.File {
		out = append(out, filepath.Join(dir, f.Name))
	}
	return out
}
`,
			clean: `import (
	"archive/zip"
)

// Names is the name of each file of r.
func Names(r *zip.Reader) []string {
	var out []string
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	return out
}
`,
		},
		"G306": {
			planted: `import (
	"os"
)

// Write writes b to p, readable by all.
func Write(p string, b []byte) error { return os.WriteFile(p, b, 0o644) }
`,
			clean: `import (
	"os"
)

// Write writes b to p, for its owner.
func Write(p string, b []byte) error { return os.WriteFile(p, b, 0o600) }
`,
		},
		"G307": {
			planted: `import (
	"os"
)

// Create makes the report with os.Create's mode.
func Create() (*os.File, error) { return os.Create("report.txt") }
`,
			clean: `import (
	"os"
)

// Create makes the report for its owner.
func Create() (*os.File, error) {
	return os.OpenFile("report.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
}
`,
			silent: "os.Create makes a file at 0o666, which is the mode the rule allows by default, and the preset sets none",
		},
		"G401": {
			planted: `import (
	"crypto/md5"
)

// Sum is b's MD5.
func Sum(b []byte) [16]byte { return md5.Sum(b) }
`,
			clean: `import (
	"crypto/sha256"
)

// Sum is b's SHA-256.
func Sum(b []byte) [32]byte { return sha256.Sum256(b) }
`,
		},
		"G402": {
			planted: `import (
	"crypto/tls"
)

// Config skips the peer's check.
func Config() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}
}
`,
			clean: `import (
	"crypto/tls"
)

// Config is TLS 1.3 with the default checks.
func Config() *tls.Config { return &tls.Config{MinVersion: tls.VersionTLS13} }
`,
		},
		"G403": {
			planted: `import (
	"crypto/rand"
	"crypto/rsa"
)

// Key is a 1024-bit key.
func Key() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 1024) }
`,
			clean: `import (
	"crypto/rand"
	"crypto/rsa"
)

// Key is a 3072-bit key.
func Key() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 3072) }
`,
		},
		"G404": {
			planted: `import (
	"math/rand"
)

// Pick is a number below n.
func Pick(n int) int { return rand.Intn(n) }
`,
			clean: `import (
	"crypto/rand"
	"math/big"
)

// Pick is a number below n.
func Pick(n int64) (*big.Int, error) { return rand.Int(rand.Reader, big.NewInt(n)) }
`,
		},
		"G405": {
			planted: `import (
	"crypto/cipher"
	"crypto/des"
)

// Block is a DES block.
func Block(key []byte) (cipher.Block, error) { return des.NewCipher(key) }
`,
			clean: `import (
	"crypto/aes"
	"crypto/cipher"
)

// Block is an AES block.
func Block(key []byte) (cipher.Block, error) { return aes.NewCipher(key) }
`,
		},
		"G406": {
			planted: `import (
	"golang.org/x/crypto/md4"
	"hash"
)

// New is an MD4.
func New() hash.Hash { return md4.New() }
`,
			clean: `import (
	"crypto/sha256"
	"hash"
)

// New is a SHA-256.
func New() hash.Hash { return sha256.New() }
`,
		},
		"G407": {
			planted: `import (
	"crypto/aes"
	"crypto/cipher"
)

// Seal seals msg under a fixed nonce.
func Seal(key, msg []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, []byte("012345678901"), msg, nil), nil
}
`,
			clean: `import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

// Seal seals msg under a random nonce, which leads the result.
func Seal(key, msg []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, msg, nil), nil
}
`,
		},
		"G408": {
			planted: `import (
	"golang.org/x/crypto/ssh"
)

var last ssh.PublicKey

// Config keeps the last key it was shown.
func Config() *ssh.ServerConfig {
	c := &ssh.ServerConfig{}
	c.PublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		last = key
		return &ssh.Permissions{}, nil
	}
	return c
}
`,
			clean: `import (
	"golang.org/x/crypto/ssh"
)

// Config refuses every key.
func Config() *ssh.ServerConfig {
	c := &ssh.ServerConfig{}
	c.PublicKeyCallback = func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
		return nil, ssh.ErrNoKey
	}
	return c
}
`,
		},
		"G501": {
			planted: `import (
	"crypto/md5"
	"hash"
)

// New is an MD5.
func New() hash.Hash { return md5.New() }
`,
			clean: `import (
	"crypto/sha256"
	"hash"
)

// New is a SHA-256.
func New() hash.Hash { return sha256.New() }
`,
		},
		"G502": {
			planted: `import (
	"crypto/cipher"
	"crypto/des"
)

// Block is a DES block.
func Block(key []byte) (cipher.Block, error) { return des.NewCipher(key) }
`,
			clean: `import (
	"crypto/aes"
	"crypto/cipher"
)

// Block is an AES block.
func Block(key []byte) (cipher.Block, error) { return aes.NewCipher(key) }
`,
		},
		"G503": {
			planted: `import (
	"crypto/rc4"
)

// Stream is an RC4 stream.
func Stream(key []byte) (*rc4.Cipher, error) { return rc4.NewCipher(key) }
`,
			clean: `import (
	"crypto/aes"
	"crypto/cipher"
)

// Stream is an AES counter stream.
func Stream(key, iv []byte) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewCTR(block, iv), nil
}
`,
		},
		"G504": {
			planted: `import (
	"net/http"
	"net/http/cgi"
)

// Handler runs a CGI program.
func Handler() http.Handler { return &cgi.Handler{Path: "/work/cgi-bin/app"} }
`,
			clean: `import (
	"net/http"
)

// Handler serves nothing.
func Handler() http.Handler { return http.NotFoundHandler() }
`,
		},
		"G505": {
			planted: `import (
	"crypto/sha1"
	"hash"
)

// New is a SHA-1.
func New() hash.Hash { return sha1.New() }
`,
			clean: `import (
	"crypto/sha256"
	"hash"
)

// New is a SHA-256.
func New() hash.Hash { return sha256.New() }
`,
		},
		"G506": {
			planted: `import (
	"golang.org/x/crypto/md4"
	"hash"
)

// New is an MD4.
func New() hash.Hash { return md4.New() }
`,
			clean: `import (
	"crypto/sha256"
	"hash"
)

// New is a SHA-256.
func New() hash.Hash { return sha256.New() }
`,
		},
		"G507": {
			planted: `import (
	"golang.org/x/crypto/ripemd160"
	"hash"
)

// New is a RIPEMD-160.
func New() hash.Hash { return ripemd160.New() }
`,
			clean: `import (
	"crypto/sha256"
	"hash"
)

// New is a SHA-256.
func New() hash.Hash { return sha256.New() }
`,
		},
		"G601": {
			planted: `// Pointers points at each element, through the loop's one variable.
func Pointers(xs []int) []*int {
	var out []*int
	for _, x := range xs {
		out = append(out, &x)
	}
	return out
}
`,
			clean: `// Pointers points at each element.
func Pointers(xs []int) []*int {
	var out []*int
	for i := range xs {
		out = append(out, &xs[i])
	}
	return out
}
`,
			goVersion: "1.21.0",
		},
		"G602": {
			planted: `// Head is the first three bytes of an empty slice.
func Head() []byte {
	s := make([]byte, 0)
	return s[:3]
}
`,
			clean: `// Head is the first three bytes of a slice of three.
func Head() []byte {
	s := make([]byte, 3)
	return s[:3]
}
`,
		},
		"G701": {
			planted: `import (
	"database/sql"
	"net/http"
)

// Find joins the query string into its query.
func Find(db *sql.DB, r *http.Request) (*sql.Rows, error) {
	name := r.URL.Query().Get("name")
	q := "SELECT id FROM users WHERE name = '" + name + "'"
	return db.Query(q)
}
`,
			clean: `import (
	"database/sql"
	"net/http"
)

// Find passes the query string as a parameter.
func Find(db *sql.DB, r *http.Request) (*sql.Rows, error) {
	return db.Query("SELECT id FROM users WHERE name = ?", r.URL.Query().Get("name"))
}
`,
		},
		"G702": {
			planted: `import (
	"net/http"
	"os/exec"
)

// Show runs cat on the file the request names.
func Show(r *http.Request) error {
	name := r.URL.Query().Get("file")
	return exec.CommandContext(r.Context(), "cat", name).Run()
}
`,
			clean: `import (
	"context"
	"os/exec"
)

// Show runs date.
func Show(ctx context.Context) error { return exec.CommandContext(ctx, "date").Run() }
`,
		},
		"G703": {
			planted: `import (
	"net/http"
	"os"
)

// Open opens the file the request names.
func Open(r *http.Request) (*os.File, error) { return os.Open(r.URL.Query().Get("file")) }
`,
			clean: `import (
	"net/http"
	"os"
)

// Open opens the readme.
func Open(_ *http.Request) (*os.File, error) { return os.Open("README.md") }
`,
		},
		"G704": {
			planted: `import (
	"net/http"
)

// Fetch gets the page the request names.
func Fetch(r *http.Request) (*http.Response, error) { return http.Get(r.URL.Query().Get("url")) }
`,
			clean: `import (
	"net/http"
)

// Fetch gets the one page.
func Fetch(_ *http.Request) (*http.Response, error) { return http.Get("https://example.com/") }
`,
		},
		"G705": {
			planted: `import (
	"fmt"
	"net/http"
)

// Greet writes the name the request gives into markup.
func Greet(w http.ResponseWriter, r *http.Request) error {
	_, err := fmt.Fprintf(w, "<h1>%s</h1>", r.URL.Query().Get("name"))
	return err
}
`,
			clean: `import (
	"fmt"
	"html"
	"net/http"
)

// Greet writes the name the request gives, escaped.
func Greet(w http.ResponseWriter, r *http.Request) error {
	_, err := fmt.Fprintf(w, "<h1>%s</h1>", html.EscapeString(r.URL.Query().Get("name")))
	return err
}
`,
		},
		"G706": {
			planted: `import (
	"log/slog"
	"net/http"
)

// Note logs the message the request gives.
func Note(r *http.Request) { slog.Warn(r.URL.Query().Get("msg")) }
`,
			clean: `import (
	"log/slog"
	"net/http"
)

// Note logs how long the message the request gives is.
func Note(r *http.Request) { slog.Warn("a message", "length", len(r.URL.Query().Get("msg"))) }
`,
		},
		"G707": {
			planted: `import (
	"net/http"
	"net/smtp"
)

// Send mails from the sender the request names.
func Send(r *http.Request) error {
	return smtp.SendMail("127.0.0.1:25", nil, r.FormValue("from"), []string{"alex@example.com"}, []byte("Subject: hi\r\n\r\nbody"))
}
`,
			clean: `import (
	"net/http"
	"net/smtp"
)

// Send mails from the one sender.
func Send(_ *http.Request) error {
	return smtp.SendMail("127.0.0.1:25", nil, "alex@example.com", []string{"alex@example.org"}, []byte("Subject: hi\r\n\r\nbody"))
}
`,
		},
		"G708": {
			planted: `import (
	"net/http"
	"text/template"
)

// Page parses the template the request gives.
func Page(w http.ResponseWriter, r *http.Request) error {
	t, err := template.New("page").Parse(r.URL.Query().Get("tmpl"))
	if err != nil {
		return err
	}
	return t.Execute(w, nil)
}
`,
			clean: `import (
	"html/template"
	"net/http"
)

// Page fills the one template with the name the request gives.
func Page(w http.ResponseWriter, r *http.Request) error {
	t, err := template.New("page").Parse("<p>{{.}}</p>")
	if err != nil {
		return err
	}
	return t.Execute(w, r.URL.Query().Get("name"))
}
`,
		},
		"G709": {
			planted: `import (
	"encoding/gob"
	"net/http"
	"strings"
)

// User is one account.
type User struct{ Name string }

// Decode decodes the user the request sends.
func Decode(r *http.Request) (User, error) {
	var u User
	err := gob.NewDecoder(strings.NewReader(r.FormValue("data"))).Decode(&u)
	return u, err
}
`,
			clean: `import (
	"net/http"
)

// User is one account.
type User struct{ Name string }

// Decode takes the name the request sends.
func Decode(r *http.Request) User { return User{Name: r.FormValue("name")} }
`,
		},
		"G710": {
			planted: `import (
	"net/http"
)

// Next redirects to the place the request names.
func Next(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, r.URL.Query().Get("next"), http.StatusFound)
}
`,
			clean: `import (
	"net/http"
)

// Next redirects home.
func Next(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/start", http.StatusFound) }
`,
		},
	}
}
