package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/abraithwaite/jeff"
	redis_store "github.com/abraithwaite/jeff/redis"
	"github.com/gomodule/redigo/redis"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

var router = mux.NewRouter()

// User is an example user in your application
type User struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

var userMap = map[string]User{}

type server struct {
	jeff *jeff.Jeff
}

func main() {
	p := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", "localhost:6379") },
	}
	str := redis_store.New(p)
	s := &server{
		jeff: jeff.New(str, jeff.Insecure),
	}
	router.HandleFunc("/", s.indexPageHandler)
	router.Handle("/internal", s.jeff.WrapFunc(s.internalPageHandler))
	router.Handle("/public", s.jeff.Public(http.HandlerFunc(s.publicPageHandler)))
	router.HandleFunc("/register", s.registerFormHandler).Methods("GET")
	router.HandleFunc("/register", s.registerHandler).Methods("POST")
	router.HandleFunc("/login", s.loginHandler).Methods("POST")
	router.Handle("/logout", s.jeff.Wrap(http.HandlerFunc(s.logoutHandler))).Methods("POST")
	sv := handlers.LoggingHandler(os.Stderr, router)
	http.Handle("/", sv)
	http.ListenAndServe(":8000", nil)
}

func (s *server) indexPageHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, indexPage)
}

func (s *server) registerFormHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, registerForm)
}

func (s *server) internalPageHandler(w http.ResponseWriter, r *http.Request) {
	sess := jeff.ActiveSession(r.Context())
	fmt.Fprintf(w, internalPage, sess.Key)
}

func (s *server) publicPageHandler(w http.ResponseWriter, r *http.Request) {
	sess := jeff.ActiveSession(r.Context())
	fmt.Fprintf(w, publicPage, sess.Key)
}

func (s *server) loginHandler(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	pass := r.FormValue("password")
	redir := "/"
	if name == "" || pass == "" {
		w.WriteHeader(400)
		return
	}
	u, ok := userMap[name]
	if ok {
		if u.Password != pass {
			w.WriteHeader(400)
			return
		}
		err := s.jeff.Set(r.Context(), w, []byte(name))
		if err != nil {
			w.WriteHeader(500)
			panic(err)
		}
		redir = "/internal"
	}
	http.Redirect(w, r, redir, 302)
}

func (s *server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	s.jeff.Clear(r.Context(), w)
	http.Redirect(w, r, "/", 302)
}

func (s *server) registerHandler(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	pass := r.FormValue("password")
	if name == "" || pass == "" {
		w.WriteHeader(400)
		return
	}
	userMap[name] = User{
		Name:     name,
		Password: pass,
	}
	http.Redirect(w, r, "/", 302)
}

const indexPage = `
<h1>Login</h1>
<form method="post" action="/login">
    <label for="name">User name</label>
    <input type="text" id="name" name="name">
    <label for="password">Password</label>
    <input type="password" id="password" name="password">
    <button type="submit">Login</button>
</form>
`

const registerForm = `
<h1>Register</h1>
<form method="post" action="/register">
    <label for="name">User name</label>
    <input type="text" id="name" name="name">
    <label for="password">Password</label>
    <input type="password" id="password" name="password">
    <button type="submit">Register</button>
</form>
`

const internalPage = `
<h1>Internal</h1>
<hr>
<p>User: %s</p>
<form method="post" action="/logout">
	<button type="submit">Logout</button>
</form>
`

const publicPage = `
<h1>Public</h1>
<hr>
<p>User: %s</p>
`
