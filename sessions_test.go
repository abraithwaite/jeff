package jeff_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abraithwaite/jeff"
	memcache_store "github.com/abraithwaite/jeff/memcache"
	"github.com/abraithwaite/jeff/memory"
	redis_store "github.com/abraithwaite/jeff/redis"
	"github.com/abraithwaite/jeff/sqlite"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gomodule/redigo/redis"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type server struct {
	j *jeff.Jeff
	t *testing.T

	authedPub bool
}

// email is intentionally invalid
var email = []byte("super@exa::mple.com")

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	err := s.j.Set(r.Context(), w, email, []byte(r.UserAgent()))
	assert.NoError(s.t, err)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	err := s.j.Clear(r.Context(), w)
	assert.NoError(s.t, err)
}

var redir = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
})

func (s *server) authed(w http.ResponseWriter, r *http.Request) {
	v := jeff.ActiveSession(r.Context())
	assert.Equal(s.t, email, v.Key, "authed session should set the user on context")
}

func (s *server) public(w http.ResponseWriter, r *http.Request) {
	v := jeff.ActiveSession(r.Context())
	if string(v.Key) != "" {
		s.authedPub = true
		assert.Equal(s.t, email, v.Key, "authed session should set the user on context")
	}
	w.Write([]byte("okay"))
}

func TestMemory(t *testing.T) {
	Suite(t, memory.New())
}

func TestMemoryExpires(t *testing.T) {
	SuiteExpires(t, memory.New())
}

func TestMemcache(t *testing.T) {
	mcc := memcache.New("localhost:11211")
	str := memcache_store.New(mcc)
	Suite(t, str)
}

func TestMemcacheExpires(t *testing.T) {
	mcc := memcache.New("localhost:11211")
	str := memcache_store.New(mcc)
	SuiteExpires(t, str)
}

func TestRedis(t *testing.T) {
	p := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", "localhost:6379") },
	}
	str := redis_store.New(p)
	Suite(t, str)
}

func TestRedisExpires(t *testing.T) {
	p := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", "localhost:6379") },
	}
	str := redis_store.New(p)
	SuiteExpires(t, str)
}

// sqliteDbPath creates a path to an ephemeral, in-memory SQLite database path,
// choosing a random name to ensure that we don't re-use the same database path
// during multiple tests.
func sqliteDbPath() string {
	charset := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	runes := make([]rune, 10)
	for i := range runes {
		runes[i] = charset[rand.Intn(len(charset))]
	}
	name := string(runes)
	return fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
}

func TestSqlite(t *testing.T) {
	db, err := sql.Open("sqlite3", sqliteDbPath())
	assert.NoError(t, err)
	str, err := sqlite.New(db)
	assert.NoError(t, err)
	Suite(t, str)
}

func TestSqliteExpires(t *testing.T) {
	db, err := sql.Open("sqlite3", sqliteDbPath())
	assert.NoError(t, err)
	str, err := sqlite.New(db, sqlite.CleanupInterval(1*time.Second))
	assert.NoError(t, err)
	SuiteExpires(t, str)
}

func Suite(t *testing.T, store jeff.Storage) {
	exp := 10 * 24 * time.Hour
	j := jeff.New(store,
		jeff.Redirect(redir),
		jeff.Domain("example.com"),
		jeff.CookieName("session"),
		jeff.Path("/"),
		jeff.Expires(exp),
	)

	s := &server{
		j: j,
		t: t,
	}

	r := http.NewServeMux()
	endpoint := j.Wrap(http.HandlerFunc(s.authed))
	public := j.Public(http.HandlerFunc(s.public))
	r.Handle("/authenticated", endpoint)
	r.Handle("/public", public)
	r.HandleFunc("/login", s.login)
	r.Handle("/logout", j.Wrap(http.HandlerFunc(s.logout)))

	var (
		req             *http.Request
		w               *httptest.ResponseRecorder
		cookie, cookie2 *http.Cookie
	)

	rec := time.Now().UTC().Truncate(time.Second)
	jeff.SetTime(func() time.Time { return rec })

	t.Run("not authenticated", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "unauthenticated requests should redirect")
		assert.Equal(t, "/login", resp.Header.Get("Location"), "unauthenticated requests should redirect")
	})

	t.Run("login", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		req.Header.Set("User-Agent", "golang-user-agent")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		require.Equal(t, 1, len(cookies), "login should set cookie")
		cookie = cookies[0]
		assert.Equal(t, true, cookie.Secure, "cookie should be Secure")
		assert.Equal(t, true, cookie.HttpOnly, "cookie should be HttpOnly")
		assert.Equal(t, "example.com", cookie.Domain, "cookie Domain should be set")
		assert.Equal(t, "session", cookie.Name, "cookie Name should be set")
		assert.Equal(t, rec.Add(exp), cookie.Expires, "cookie Name should be set")
	})

	t.Run("authenticated", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "authenticated should succeed")
	})

	t.Run("new login", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		require.Equal(t, 1, len(cookies), "login should set cookie")
		cookie2 = cookies[0]
		assert.NotEqual(t, cookie, cookie2, "logging in again should assign new session")
	})

	t.Run("authenticated new session", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie2)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "new session should be valid")
	})

	t.Run("authenticated old session", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "old session should be valid")
	})

	t.Run("third login", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		require.Equal(t, 1, len(cookies), "login should set cookie")
		cookie2 = cookies[0]
	})

	t.Run("get all sessions", func(t *testing.T) {
		sessions, err := j.SessionsForKey(context.Background(), email)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(sessions), "two active sessions should be returned")
	})

	t.Run("clear one session", func(t *testing.T) {
		vals := strings.SplitN(cookie2.Value, "::", 2)
		assert.Equal(t, 2, len(vals), "invalid cookie value")
		err := j.Delete(context.Background(), email, []byte(vals[1]))
		assert.NoError(t, err)
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie2)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "unauthenticated requests should redirect")
		assert.Equal(t, "/login", resp.Header.Get("Location"), "unauthenticated requests should redirect")

		sessions, err := j.SessionsForKey(context.Background(), email)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(sessions), "two active sessions should be returned")
	})

	t.Run("clear all sessions", func(t *testing.T) {
		err := j.Delete(context.Background(), email)
		assert.NoError(t, err)
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "unauthenticated requests should redirect")
		assert.Equal(t, "/login", resp.Header.Get("Location"), "unauthenticated requests should redirect")
	})

	t.Run("get all sessions", func(t *testing.T) {
		sessions, err := j.SessionsForKey(context.Background(), email)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(sessions), "two active sessions should be returned")
	})

	t.Run("not authenticated public", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/public", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "unauthenticated requests should not redirect")
	})

	t.Run("login", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		req.Header.Set("User-Agent", "golang-user-agent")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		require.Equal(t, 1, len(cookies), "login should set cookie")
		cookie = cookies[0]
	})

	t.Run("authenticated public", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/public", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "authenticated should succeed")
		assert.True(t, s.authedPub, "authenticated should set user")
	})

	t.Run("logout", func(t *testing.T) {
		req = httptest.NewRequest("GET", "http://example.com/logout", nil)
		req.Header.Set("User-Agent", "golang-user-agent")
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "logout should succeed")
		cookies := resp.Cookies()
		require.Equal(t, 1, len(cookies), "logout should set cookie")
	})

	t.Run("get all sessions", func(t *testing.T) {
		sessions, err := j.SessionsForKey(context.Background(), email)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(sessions), "two active sessions should be returned")
	})

}

func SuiteExpires(t *testing.T, store jeff.Storage) {
	// Can't really do shorter because both redis and memcache are second
	// granularity.  1s rounds down expire time to 0 for redis impl.
	exp := 5 * time.Second
	j := jeff.New(store,
		jeff.Redirect(redir),
		jeff.Domain("example.com"),
		jeff.CookieName("session"),
		jeff.Path("/"),
		jeff.Expires(exp),
	)

	s := &server{
		j: j,
		t: t,
	}

	r := http.NewServeMux()
	endpoint := j.Wrap(http.HandlerFunc(s.authed))
	r.Handle("/authenticated", endpoint)
	r.HandleFunc("/login", s.login)

	jeff.SetTime(func() time.Time { return time.Now() })

	t.Run("token expires serverside", func(t *testing.T) {

		// Setup cookie
		req := httptest.NewRequest("GET", "http://example.com/login", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		cookie := cookies[0]

		time.Sleep(2 * time.Second)

		// Setup second session, expiring later
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")

		time.Sleep(3 * time.Second)

		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "session should expire serverside")

		// Setup second session, expiring later
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")

		time.Sleep(1 * time.Second)

		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "session should expire serverside")
	})

	err := j.Delete(context.Background(), email)
	assert.NoError(t, err)
}

func TestInsecure(t *testing.T) {
	j := jeff.New(memory.New(), jeff.Insecure)
	s := &server{j: j, t: t}
	r := http.NewServeMux()
	r.HandleFunc("/login", s.login)
	req := httptest.NewRequest("GET", "http://example.com/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
	cookies := resp.Cookies()
	require.Equal(t, 1, len(cookies), "login should set cookie")
	cookie := cookies[0]
	assert.Equal(t, false, cookie.Secure, "cookie is not secure")
}

func TestSessCookie(t *testing.T) {
	j := jeff.New(memory.New(), jeff.Expires(0))
	s := &server{j: j, t: t}
	r := http.NewServeMux()
	r.HandleFunc("/login", s.login)
	req := httptest.NewRequest("GET", "http://example.com/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
	cookies := resp.Cookies()
	require.Equal(t, 1, len(cookies), "login should set cookie")
	cookie := cookies[0]
	assert.True(t, cookie.Expires.IsZero(), "cookie expiration not set (session cookie)")
}
