package internal

import (
	"maps"
	"strings"
)

// gosecVersion is the gosec the pinned golangci-lint carries, its build
// info's github.com/securego/gosec/v2: the version gosecRules lists the rules
// of and gosecPlants proves. A golangci-lint that carries another fails
// TestEachGosecRuleThePresetTurnsOnFindsItsPlantAndSparesItsControl until both
// are written for it.
const gosecVersion = "v2.29.0"

// gosecRules are the rules of gosec at gosecVersion, each with what it
// finds: the set the preset turns on whole, since it lists no includes
// (supply.scans.3).
func gosecRules() []string {
	return []string{
		"G101", // Look for hardcoded credentials
		"G102", // Bind to all interfaces
		"G103", // Audit the use of unsafe block
		"G104", // Audit errors not checked
		"G106", // Audit the use of ssh.InsecureIgnoreHostKey function
		"G107", // Url provided to HTTP request as taint input
		"G108", // Profiling endpoint is automatically exposed
		"G109", // Converting strconv.Atoi result to int32/int16
		"G110", // Detect io.Copy instead of io.CopyN when decompression
		"G111", // Detect http.Dir('/') as a potential risk
		"G112", // Detect ReadHeaderTimeout not configured as a potential risk
		"G113", // HTTP request smuggling via conflicting headers or bare LF in body parsing
		"G114", // Use of net/http serve function that has no support for setting timeouts
		"G115", // Type conversion which leads to integer overflow
		"G116", // Detect Trojan Source attacks using bidirectional Unicode characters
		"G117", // Potential exposure of secrets via JSON/YAML/XML/TOML marshaling
		"G118", // Context propagation failure leading to goroutine/resource leaks
		"G119", // Unsafe redirect policy may propagate sensitive headers
		"G120", // Unbounded form parsing in HTTP handlers can cause memory exhaustion
		"G121", // Unsafe CrossOriginProtection bypass patterns
		"G122", // Filesystem TOCTOU race risk in filepath.Walk/WalkDir callbacks
		"G123", // TLS resumption may bypass VerifyPeerCertificate when VerifyConnection is unset
		"G124", // Insecure HTTP cookie configuration missing Secure, HttpOnly, or SameSite attributes
		"G201", // SQL query construction using format string
		"G202", // SQL query construction using string concatenation
		"G203", // Use of unescaped data in HTML templates
		"G204", // Audit use of command execution
		"G301", // Poor file permissions used when creating a directory
		"G302", // Poor file permissions used when creation file or using chmod
		"G303", // Creating tempfile using a predictable path
		"G304", // File path provided as taint input
		"G305", // File path traversal when extracting zip archive
		"G306", // Poor file permissions used when writing to a file
		"G307", // Poor file permissions used when creating a file with os.Create
		"G401", // Detect the usage of MD5 or SHA1
		"G402", // Look for bad TLS connection settings
		"G403", // Ensure minimum RSA key length of 2048 bits
		"G404", // Insecure random number source (rand)
		"G405", // Detect the usage of DES or RC4
		"G406", // Detect the usage of deprecated MD4 or RIPEMD160
		"G407", // Use of hardcoded IV/nonce for encryption
		"G408", // Stateful misuse of ssh.PublicKeyCallback leading to auth bypass
		"G501", // Import blocklist: crypto/md5
		"G502", // Import blocklist: crypto/des
		"G503", // Import blocklist: crypto/rc4
		"G504", // Import blocklist: net/http/cgi
		"G505", // Import blocklist: crypto/sha1
		"G506", // Import blocklist: golang.org/x/crypto/md4
		"G507", // Import blocklist: golang.org/x/crypto/ripemd160
		"G601", // Implicit memory aliasing in RangeStmt
		"G602", // Possible slice bounds out of range
		"G701", // SQL injection via taint analysis
		"G702", // Command injection via taint analysis
		"G703", // Path traversal via taint analysis
		"G704", // SSRF via taint analysis
		"G705", // XSS via taint analysis
		"G706", // Log injection via taint analysis
		"G707", // SMTP command/header injection via taint analysis
		"G708", // Server-side template injection via taint analysis
		"G709", // Unsafe deserialization of untrusted data via taint analysis
		"G710", // Open redirect via taint analysis
	}
}

// gosecPlants holds a planted finding and a clean control for each rule of
// gosecRules: our own code, a package each. The preset's settings decide which
// of them run (TestEachGosecRuleThePresetTurnsOnFindsItsPlantAndSparesItsControl),
// and a rule the preset turns on with no row here fails that test, so a rule is
// never turned on unproven. The rows are kept in two files by family: the
// general rules (G1) here, and the rest in gosecFamilyPlants.
func gosecPlants() map[string]gosecPlant {
	plants := gosecGeneralPlants()
	maps.Copy(plants, gosecFamilyPlants())
	return plants
}

// gosecGeneralPlants are the plants of the general rules, G1.
func gosecGeneralPlants() map[string]gosecPlant {
	return map[string]gosecPlant{
		"G101": {
			planted: `const apiToken = "` + plantedCredential() + `"

// Token is the credential.
func Token() string { return apiToken }
`,
			clean: `import (
	"os"
)

// Token is the credential, read from the environment.
func Token() string { return os.Getenv("SERVICE_TOKEN") }
`,
		},
		"G102": {
			planted: `import (
	"net"
)

// Listen listens on every interface.
func Listen() (net.Listener, error) { return net.Listen("tcp", "0.0.0.0:2000") }
`,
			clean: `import (
	"net"
)

// Listen listens on the loopback interface.
func Listen() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:2000") }
`,
		},
		"G103": {
			planted: `import (
	"unsafe"
)

// Addr is p as a number.
func Addr(p *int) uintptr { return uintptr(unsafe.Pointer(p)) }
`,
			clean: `// Value is what p points at.
func Value(p *int) int { return *p }
`,
		},
		"G104": {
			planted: `import (
	"os"
)

// Remove removes p and drops the error.
func Remove(p string) {
	os.Remove(p)
}
`,
			clean: `import (
	"os"
)

// Remove removes p.
func Remove(p string) error { return os.Remove(p) }
`,
		},
		"G106": {
			planted: `import (
	"golang.org/x/crypto/ssh"
)

// Callback trusts every host key.
func Callback() ssh.HostKeyCallback { return ssh.InsecureIgnoreHostKey() }
`,
			clean: `import (
	"golang.org/x/crypto/ssh"
)

// Callback trusts the one host key k.
func Callback(k ssh.PublicKey) ssh.HostKeyCallback { return ssh.FixedHostKey(k) }
`,
		},
		"G107": {
			planted: `import (
	"net/http"
)

// Fetch gets url.
func Fetch(url string) (*http.Response, error) { return http.Get(url) }
`,
			clean: `import (
	"net/http"
)

// Fetch gets the one page.
func Fetch() (*http.Response, error) { return http.Get("https://example.com/") }
`,
		},
		"G108": {
			planted: `import (
	"net/http"
	_ "net/http/pprof"
)

// Mux is the server's mux.
func Mux() *http.ServeMux { return http.NewServeMux() }
`,
			clean: `import (
	"net/http"
)

// Mux is the server's mux.
func Mux() *http.ServeMux { return http.NewServeMux() }
`,
		},
		"G109": {
			planted: `import (
	"strconv"
)

// Small is s as an int32.
func Small(s string) (int32, error) {
	i, err := strconv.Atoi(s)
	return int32(i), err
}
`,
			clean: `import (
	"strconv"
)

// Small is s as an int32.
func Small(s string) (int32, error) {
	i, err := strconv.ParseInt(s, 10, 32)
	return int32(i), err
}
`,
		},
		"G110": {
			planted: `import (
	"compress/gzip"
	"io"
)

// Inflate copies src, unzipped, to dst.
func Inflate(dst io.Writer, src io.Reader) error {
	z, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, z)
	return err
}
`,
			clean: `import (
	"compress/gzip"
	"io"
)

// Inflate copies at most a MiB of src, unzipped, to dst.
func Inflate(dst io.Writer, src io.Reader) error {
	z, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	_, err = io.CopyN(dst, z, 1<<20)
	return err
}
`,
		},
		"G111": {
			planted: `import (
	"net/http"
)

// Files serves the whole disk.
func Files() http.Handler { return http.FileServer(http.Dir("/")) }
`,
			clean: `import (
	"net/http"
)

// Files serves the public folder.
func Files() http.Handler { return http.FileServer(http.Dir("public")) }
`,
		},
		"G112": {
			planted: `import (
	"net/http"
)

// Server has no header timeout.
func Server() *http.Server { return &http.Server{Addr: "127.0.0.1:8080"} }
`,
			clean: `import (
	"net/http"
	"time"
)

// Server has a header timeout.
func Server() *http.Server {
	return &http.Server{Addr: "127.0.0.1:8080", ReadHeaderTimeout: 5 * time.Second}
}
`,
		},
		"G113": {
			planted: `import (
	"net/http"
)

// Respond sets both framings.
func Respond(w http.ResponseWriter) error {
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Length", "2")
	_, err := w.Write([]byte("ok"))
	return err
}
`,
			clean: `import (
	"net/http"
)

// Respond sets one framing.
func Respond(w http.ResponseWriter) error {
	w.Header().Set("Content-Length", "2")
	_, err := w.Write([]byte("ok"))
	return err
}
`,
		},
		"G114": {
			planted: `import (
	"net/http"
)

// Serve serves h with no timeouts.
func Serve(h http.Handler) error { return http.ListenAndServe("127.0.0.1:8080", h) }
`,
			clean: `import (
	"net/http"
	"time"
)

// Serve serves h with a header timeout.
func Serve(h http.Handler) error {
	s := &http.Server{Addr: "127.0.0.1:8080", Handler: h, ReadHeaderTimeout: 5 * time.Second}
	return s.ListenAndServe()
}
`,
		},
		"G115": {
			planted: `// Narrow is v as an int32.
func Narrow(v int64) int32 { return int32(v) }
`,
			clean: `import (
	"math"
)

// Narrow is v as an int32, when it fits.
func Narrow(v int64) (int32, bool) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, false
	}
	return int32(v), true
}
`,
		},
		"G116": {
			planted: `// Admin is whether the user is one.
func Admin() bool {
	// the check: ` + "\u202e" + ` granted
	return false
}
`,
			clean: `// Admin is whether the user is one.
func Admin() bool {
	// the check: not granted
	return false
}
`,
		},
		"G117": {
			planted: `import (
	"encoding/json"
)

// Config holds a password.
type Config struct {
	Password string
}

// Encode is c as JSON.
func Encode(c Config) ([]byte, error) { return json.Marshal(c) }
`,
			clean: `import (
	"encoding/json"
)

// Config holds a name.
type Config struct {
	Name string
}

// Encode is c as JSON.
func Encode(c Config) ([]byte, error) { return json.Marshal(c) }
`,
		},
		"G118": {
			planted: `import (
	"context"
	"net/http"
	"time"
)

// Handle starts work that loses the request's context.
func Handle(_ http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	go func() {
		child, _ := context.WithTimeout(context.Background(), time.Second)
		_ = child
	}()
}
`,
			clean: `import (
	"context"
	"net/http"
	"time"
)

// Handle starts work that ends with the request.
func Handle(_ http.ResponseWriter, r *http.Request) {
	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
		case <-time.After(time.Millisecond):
		}
	}(r.Context())
}
`,
		},
		"G119": {
			planted: `import (
	"net/http"
)

// Client copies every header to a redirect.
func Client() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		req.Header = via[len(via)-1].Header.Clone()
		return nil
	}}
}
`,
			clean: `import (
	"net/http"
)

// Client follows no redirect.
func Client() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
`,
		},
		"G120": {
			planted: `import (
	"net/http"
)

// Parse reads the form with no bound on the body.
func Parse(r *http.Request) error { return r.ParseMultipartForm(32 << 20) }
`,
			clean: `import (
	"net/http"
)

// Parse reads the form, which ParseForm reads at most 10 MB of.
func Parse(r *http.Request) error { return r.ParseForm() }
`,
		},
		"G121": {
			planted: `import (
	"net/http"
)

// Protect lets every path past the check.
func Protect() *http.CrossOriginProtection {
	c := http.NewCrossOriginProtection()
	c.AddInsecureBypassPattern("/")
	return c
}
`,
			clean: `import (
	"net/http"
)

// Protect lets the health path past the check.
func Protect() *http.CrossOriginProtection {
	c := http.NewCrossOriginProtection()
	c.AddInsecureBypassPattern("/healthz")
	return c
}
`,
		},
		"G122": {
			planted: `import (
	"io/fs"
	"os"
	"path/filepath"
)

// Clear removes every file below dir as it walks.
func Clear(dir string) error {
	return filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Remove(p)
	})
}
`,
			clean: `import (
	"io/fs"
	"path/filepath"
)

// Names lists the names below dir.
func Names(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, d.Name())
		return nil
	})
	return out, err
}
`,
		},
		"G123": {
			planted: `import (
	"crypto/tls"
	"crypto/x509"
)

// Config checks the peer only on a full handshake.
func Config() *tls.Config {
	return &tls.Config{
		MinVersion:            tls.VersionTLS13,
		VerifyPeerCertificate: func([][]byte, [][]*x509.Certificate) error { return nil },
	}
}
`,
			clean: `import (
	"crypto/tls"
)

// Config is TLS 1.3 with the default checks.
func Config() *tls.Config { return &tls.Config{MinVersion: tls.VersionTLS13} }
`,
		},
		"G124": {
			planted: `import (
	"net/http"
)

// Set sets a cookie with no attributes.
func Set(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "v"})
}
`,
			clean: `import (
	"net/http"
)

// Set sets a cookie only the server reads.
func Set(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "v", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}
`,
		},
	}
}

// plantedCredential is the G101 plant's credential, written in pieces so
// that this file holds nothing the secret scan takes for one.
func plantedCredential() string {
	return strings.Join([]string{"f62e5bcd", "a4fae4f8", "2370da0c", "6f20697b", "8f8447ef"}, "")
}

// gosecStandIn is the stand-in for golang.org/x/crypto that the plants of
// G106, G406, G408, G506 and G507 import, through a replace: the names gosec
// matches and nothing more, so the module is linted with nothing fetched.
func gosecStandIn() map[string]string {
	return map[string]string{
		"go.mod": `module golang.org/x/crypto

go 1.21
`,
		"ssh/ssh.go": `// Package ssh stands in for golang.org/x/crypto/ssh.
package ssh

import (
	"errors"
	"net"
)

// PublicKey is a key.
type PublicKey interface{ Marshal() []byte }

// ConnMetadata is a connection.
type ConnMetadata interface{ User() string }

// Permissions are what a key may do.
type Permissions struct{ Extensions map[string]string }

// ServerConfig is a server's.
type ServerConfig struct {
	PublicKeyCallback func(ConnMetadata, PublicKey) (*Permissions, error)
}

// HostKeyCallback checks a host's key.
type HostKeyCallback func(hostname string, remote net.Addr, key PublicKey) error

// ErrNoKey is a refusal.
var ErrNoKey = errors.New("no key")

// InsecureIgnoreHostKey trusts every key.
func InsecureIgnoreHostKey() HostKeyCallback {
	return func(string, net.Addr, PublicKey) error { return nil }
}

// FixedHostKey trusts k.
func FixedHostKey(k PublicKey) HostKeyCallback {
	return func(string, net.Addr, PublicKey) error { _ = k; return nil }
}
`,
		"md4/md4.go": `// Package md4 stands in for golang.org/x/crypto/md4.
package md4

import (
	"crypto/sha256"
	"hash"
)

// New is a hash.
func New() hash.Hash { return sha256.New() }
`,
		"ripemd160/ripemd160.go": `// Package ripemd160 stands in for golang.org/x/crypto/ripemd160.
package ripemd160

import (
	"crypto/sha256"
	"hash"
)

// New is a hash.
func New() hash.Hash { return sha256.New() }
`,
	}
}
