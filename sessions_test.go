package jeff_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abraithwaite/jeff"
	"github.com/abraithwaite/jeff/memcache"
	"github.com/abraithwaite/jeff/memory"
	"github.com/abraithwaite/jeff/redis"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type server struct {
	j *jeff.Jeff
	t *testing.T
}

var email = []byte("super@example.com")

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	err := s.j.Set(r.Context(), w, email)
	assert.NoError(s.t, err)
}

var redir = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
})

func (s *server) authed(w http.ResponseWriter, r *http.Request) {
	v := jeff.ActiveSession(r.Context())
	assert.Equal(s.t, email, v, "authed session should set the user on context")
}

func TestMemory(t *testing.T) {
	Suite(t, memory.New())
}

func TestMemcache(t *testing.T) {
	mcc := memcache.New("localhost:11211")
	str := memcache_store.New(mcc)
	Suite(t, str)
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
	r.Handle("/authenticated", endpoint)
	r.HandleFunc("/login", s.login)

	var (
		req    *http.Request
		w      *httptest.ResponseRecorder
		cookie *http.Cookie
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

	t.Run("clear session", func(t *testing.T) {
		err := j.Clear(context.Background(), email)
		assert.NoError(t, err)
		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "unauthenticated requests should redirect")
		assert.Equal(t, "/login", resp.Header.Get("Location"), "unauthenticated requests should redirect")
	})

	// Can't really do shorter because both redis and memcache are second
	// granularity.  1s rounds down expire time to 0 for redis impl.
	exp = 2 * time.Second
	j = jeff.New(store,
		jeff.Redirect(redir),
		jeff.Domain("example.com"),
		jeff.CookieName("session"),
		jeff.Path("/"),
		jeff.Expires(exp),
	)

	s = &server{
		j: j,
		t: t,
	}

	r = http.NewServeMux()
	endpoint = j.Wrap(http.HandlerFunc(s.authed))
	r.Handle("/authenticated", endpoint)
	r.HandleFunc("/login", s.login)

	jeff.SetTime(func() time.Time { return time.Now() })

	t.Run("token expires serverside", func(t *testing.T) {

		// Setup cookie
		req = httptest.NewRequest("GET", "http://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		cookie = cookies[0]

		// Wish I could do this without sleep, but we're interacting with
		// external systems.
		// Setting this to 2.5s causes memcache to randomly fail. Additionally,
		// when adding this test I noticed that memcache would fail even when
		// long timeouts were set i.e. It was returning the key after it should
		// have expired.  The container had been up for some time (>1day) and
		// my laptop had gone in and out of sleep, if that had anything to do
		// with it
		time.Sleep(3500 * time.Millisecond)

		req = httptest.NewRequest("GET", "http://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "session should expire serverside")
	})

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

func TestExpires(t *testing.T) {
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

// This doesn't work :-(
// func SetTimes(t time.Time) {
// 	jeff.SetTime(func() time.Time { return t })
// 	memory.SetTime(func() time.Time { return t })
// 	redis_store.SetTime(func() time.Time { return t })
// }
