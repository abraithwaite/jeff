package jeff_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abraithwaite/jeff"
	memcache_store "github.com/abraithwaite/jeff/memcache"
	"github.com/abraithwaite/jeff/memory"
	redis_store "github.com/abraithwaite/jeff/redis"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/publicsuffix"
)

type server struct {
	j *jeff.Jeff
	t *testing.T

	authedPub bool
}

var email = []byte("super@exa::mple.com")

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	err := s.j.Set(r.Context(), w, email, []byte(r.UserAgent()))
	assert.NoError(s.t, err)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	s.j.Clear(r.Context(), w)
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
	r.HandleFunc("/logout", s.logout)

	var cookie *http.Cookie
	rec := time.Now().UTC().Truncate(time.Second)
	jeff.SetTime(func() time.Time { return rec })

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	require.NoError(t, err, "should not error")

	var cases = []struct {
		Name     string
		req      func() *http.Request
		validate func(*testing.T, *http.Response)
	}{
		{
			Name: "not authenticated",
			req: func() *http.Request {
				return httptest.NewRequest("GET", "https://example.com/authenticated", nil)
			},
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusFound, resp.StatusCode, "unauthenticated requests should redirect")
				assert.Equal(t, "/login", resp.Header.Get("Location"), "unauthenticated requests should redirect")
			},
		},
		{
			Name: "login",
			req: func() *http.Request {
				req := httptest.NewRequest("GET", "https://example.com/login", nil)
				req.Header.Set("User-Agent", "golang-user-agent")
				return req
			},
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
				cookies := resp.Cookies()
				require.Equal(t, 1, len(cookies), "login should set cookie")
				cookie = cookies[0]
				assert.Equal(t, true, cookie.Secure, "cookie should be Secure")
				assert.Equal(t, true, cookie.HttpOnly, "cookie should be HttpOnly")
				assert.Equal(t, "example.com", cookie.Domain, "cookie Domain should be set")
				assert.Equal(t, "session", cookie.Name, "cookie Name should be set")
				assert.Equal(t, rec.Add(exp), cookie.Expires, "cookie Name should be set")
			},
		},
		{
			Name: "authenticated",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/authenticated", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "authenticated should succeed")
			},
		},
		{
			Name: "new login",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/login", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
				cookies := resp.Cookies()
				require.Equal(t, 1, len(cookies), "login should set cookie")
				assert.NotEqual(t, cookie, cookies[0], "logging in again should assign new session")
			},
		},
		{
			Name: "authenticated new session",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/authenticated", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "new session should be valid")
			},
		},
		{
			Name: "older session still works",
			req: func() *http.Request {
				req := httptest.NewRequest("GET", "https://example.com/authenticated", nil)
				// recall old cookie. Override session cookie from previous test case
				jar.SetCookies(req.URL, []*http.Cookie{cookie})
				return req
			},
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "old session should be valid")
			},
		},
		{
			Name: "clear session",
			req: func() *http.Request {
				err := j.Delete(context.Background(), email)
				assert.NoError(t, err)
				return httptest.NewRequest("GET", "https://example.com/authenticated", nil)
			},
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusFound, resp.StatusCode, "unauthenticated requests should redirect")
				assert.Equal(t, "/login", resp.Header.Get("Location"), "unauthenticated requests should redirect")
			},
		},
		{
			Name: "not authenticated and public url",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/public", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "unauthenticated requests should not redirect")
			},
		},
		{
			Name: "login to test authed public route",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/login", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
			},
		},
		{
			Name: "authed public route",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/public", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "authenticated should succeed")
				assert.True(t, s.authedPub, "authenticated should set user")
			},
		},
		{
			Name: "logout",
			req:  func() *http.Request { return httptest.NewRequest("GET", "https://example.com/logout", nil) },
			validate: func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "logout should succeed")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			req := c.req()
			for _, cookie := range jar.Cookies(req.URL) {
				req.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			resp := w.Result()
			c.validate(t, resp)
			jar.SetCookies(req.URL, resp.Cookies())
		})
	}
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
		req := httptest.NewRequest("GET", "https://example.com/login", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
		cookies := resp.Cookies()
		cookie := cookies[0]

		time.Sleep(2 * time.Second)

		// Setup second session, expiring later
		req = httptest.NewRequest("GET", "https://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")

		time.Sleep(3 * time.Second)

		req = httptest.NewRequest("GET", "https://example.com/authenticated", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "session should expire serverside")

		// Setup second session, expiring later
		req = httptest.NewRequest("GET", "https://example.com/login", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		resp = w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")

		time.Sleep(1 * time.Second)

		req = httptest.NewRequest("GET", "https://example.com/authenticated", nil)
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
	req := httptest.NewRequest("GET", "https://example.com/login", nil)
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
	req := httptest.NewRequest("GET", "https://example.com/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "login should succeed")
	cookies := resp.Cookies()
	require.Equal(t, 1, len(cookies), "login should set cookie")
	cookie := cookies[0]
	assert.True(t, cookie.Expires.IsZero(), "cookie expiration not set (session cookie)")
}
