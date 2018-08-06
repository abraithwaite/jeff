package jeff

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"
)

// Cookie Format:
// CookieName=SessionKey::SessionToken
const separator = "::"

var defaultRedirect = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
})

type contextKey struct{ name string }

var sessionKey = contextKey{name: "session"}

var now = func() time.Time {
	return time.Now()
}

// Jeff holds the metadata needed to handle session management.
type Jeff struct {
	s          Storage
	redir      http.Handler
	cookieName string
	domain     string
	path       string
	expires    time.Duration
	insecure   bool
}

// Storage provides the base level abstraction for implementing session
// storage.  Typically this would be memcache, redis or a database.
type Storage interface {
	Store(ctx context.Context, key, value []byte, exp time.Time) error
	Fetch(ctx context.Context, key []byte) ([]byte, error)
	Delete(ctx context.Context, key []byte) error
}

// Domain sets the domain the cookie belongs to.  If unset, cookie becomes a
// host-only domain, meaning subdomains won't receive the cookie.
func Domain(d string) func(*Jeff) {
	return func(j *Jeff) {
		j.domain = d
	}
}

// CookieName sets the name of the cookie in the browser.  If you want to avoid
// fingerprinting, override it here. defaults to "_gosession"
func CookieName(n string) func(*Jeff) {
	return func(j *Jeff) {
		j.cookieName = n
	}
}

// Redirect sets the handler which gets called when authentication fails.  By
// default, this redirects to '/'. It's recommended that you replace this with
// your own.
//
//     sessions := jeff.New(store, jeff.Redirect(
//         http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//             http.Redirect(w, r, "/login", http.StatusFound)
//         })))
//
// Setting this is particularly useful if you want to stop a redirect on an
// authenticated route to render a page despite the user not being
// authenticated.  For example, say you want to display user information on the
// home page if they're logged in, but otherwise want to ignore the redirect
// and render the page for an anonymous user.  You'd define that behavior using
// a custom handler here.
func Redirect(h http.Handler) func(*Jeff) {
	return func(j *Jeff) {
		j.redir = h
	}
}

// Path sets the path attribute of the cookie.  Defaults to '/'.  You probably
// don't need to change this. See the RFC for details:
// https://tools.ietf.org/html/rfc6265
func Path(p string) func(*Jeff) {
	return func(j *Jeff) {
		j.path = p
	}
}

// Expires sets the cookie lifetime.  After logging in, the session will last
// as long as defined here and then expire.  If set to 0, then Expiration is
// not set and the cookie will expire when the client closes their user agent.
// Defaults to 30 days.
func Expires(dur time.Duration) func(*Jeff) {
	return func(j *Jeff) {
		j.expires = dur
	}
}

// Insecure unsets the Secure flag for the cookie.  This is for development
// only.  Doing this in production is an error.
func Insecure(j *Jeff) {
	log.Println("ERROR: sessions configured insecurely. for development only")
	j.insecure = true
}

// New instantiates a Jeff, applying the options provided.
func New(s Storage, opts ...func(*Jeff)) *Jeff {
	j := &Jeff{
		s:       s,
		expires: 30 * 24 * time.Hour,
	}
	for _, o := range opts {
		o(j)
	}
	j.defaults()
	return j
}

// Wrap wraps the given handler, authenticating this route and calling the
// redirect handler if session is invalid.
func (j *Jeff) Wrap(wrap http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(j.cookieName)
		if err != nil {
			j.redir.ServeHTTP(w, r)
			return
		}
		vals := strings.SplitN(c.Value, separator, 2)
		if len(vals) != 2 {
			j.redir.ServeHTTP(w, r)
			return
		}
		decoded, err := decode(vals[0])
		if err != nil {
			j.redir.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		stored, err := j.s.Fetch(ctx, decoded)
		if err != nil {
			j.redir.ServeHTTP(w, r)
			return
		}
		valid := subtle.ConstantTimeCompare(stored, []byte(vals[1])) == 1
		if valid {
			r = r.WithContext(context.WithValue(ctx, sessionKey, decoded))
			wrap.ServeHTTP(w, r)
		} else {
			j.redir.ServeHTTP(w, r)
		}
	})
}

// Set the session cookie on the response.  Call after successful
// authentication / login.
func (j *Jeff) Set(ctx context.Context, w http.ResponseWriter, key []byte) error {
	secure, err := genRandomString(24) // 192 bits
	if err != nil {
		// TODO?
		panic(err)
	}
	c := &http.Cookie{
		Secure:   !j.insecure,
		HttpOnly: true,
		Name:     j.cookieName,
		Value:    strings.Join([]string{encode(key), secure}, separator),
		Path:     j.path,
		Domain:   j.domain,
	}
	var exp time.Time
	if j.expires != 0 {
		exp = now().Add(j.expires)
		c.Expires = exp
	} else {
		// For session cookies which expire "when the browser closes"
		exp = now().Add(30 * 24 * time.Hour)
	}
	http.SetCookie(w, c)
	// TODO: encode key as store key?
	return j.s.Store(ctx, key, []byte(secure), exp)
}

// Clear the session for the given key.
func (j *Jeff) Clear(ctx context.Context, key []byte) error {
	return j.s.Delete(ctx, key)
}

func ActiveSession(ctx context.Context) []byte {
	if v, ok := ctx.Value(sessionKey).([]byte); ok {
		return v
	}
	return nil
}

func (j *Jeff) defaults() {
	if j.redir == nil {
		j.redir = defaultRedirect
	}
	if j.cookieName == "" {
		j.cookieName = "_gosession"
	}
	if j.path == "" {
		j.path = "/"
	}
}

// From: https://blog.questionable.services/article/generating-secure-random-numbers-crypto-rand/

// genRandomString returns a URL-safe, base64 encoded securely generated random
// string.  It will return an error if the system's secure random number
// generator fails to function correctly, in which case the caller should not
// continue.
func genRandomString(n int) (string, error) {
	b, err := genRandomBytes(n)
	return encode(b), err
}

// genRandomBytes returns securely generated random bytes.  It will return an
// error if the system's secure random number generator fails to function
// correctly, in which case the caller should not continue.
func genRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err != nil when we fail to read len(b) bytes.
	if err != nil {
		return nil, err
	}
	return b, nil
}

func encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
