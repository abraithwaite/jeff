# jeff

[![Build](https://circleci.com/gh/abraithwaite/jeff.svg?style=shield)](https://circleci.com/gh/abraithwaite/jeff)
[![GoDoc](https://godoc.org/github.com/abraithwaite/jeff?status.svg)](https://godoc.org/github.com/abraithwaite/jeff)
[![Go Report Card](https://goreportcard.com/badge/github.com/abraithwaite/jeff)](https://goreportcard.com/report/github.com/abraithwaite/jeff)
[![License](https://img.shields.io/badge/license-Apache--2.0-5B74AD.svg)](https://github.com/abraithwaite/jeff/blob/master/LICENSE)


A tool for managing login sessions in Go.

## Motivation

I was looking for a simple session management wrapper for Go and from what I
could tell there exists no simple sesssion library.

This library is requires a stateful backend to enable easy session revocation
and simplify the security considerations.  See the section on security for more
details.

## Features

- Redirect to login
- Middleware wrapper
- Easy to clear sessions
- Small, idiomatic API
- Context aware
- Fast

## Usage

There are three primary methods:

Set starts the session, sets the cookie on the given response, and stores the
session token.

```go
func (s Server) Login(w http.ResponseWriter, r *http.Request) {
    user = Authenticate(r)
    if user != nil {
        // Key must be unique to one user among all users
        err := s.jeff.Set(r.Context(), w, user.Email)
        // handle error
    }
    // finish login
}
```

Wrap authenticates every http.Handler it wraps, or redirects if authentication
fails.  Wrap's signature works with [alice](https://github.com/justinas/alice).

```go
    mux.HandleFunc("/login", loginHandler)
    mux.HandleFunc("/products", j.Wrap(productHandler))
    mux.HandleFunc("/users", j.Wrap(usersHandler))
    http.ListenAndServe(":8080", mux)
```

Clear deletes the active session from the store for the given key.

```go
func (s Server) Revoke(w http.ResponseWriter, r *http.Request) {
    // stuff to get user: admin input form or perhaps even from current session
    err = s.jeff.Clear(r.Context(), user.Email)
    // handle err
}
```

The default redirect handler redirects to root.  Override this behavior to set
your own login route.

```go
    sessions := jeff.New(store, jeff.Redirect(
        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            http.Redirect(w, r, "/login", http.StatusFound)
        })))
```

This is also helpful to stop a redirect on an authenticated route: For example
if you want to display user info on the home page but not redirect if
unauthenticated, rendering the page normally but without user info.

```go
    // customHandler gets called when authentication fails
    sessions := jeff.New(store, jeff.Redirect(customHandler))
```

## Design

Session tokens are securely generated on `Set` (called after successful login).
This library is unique in that the user gets to decide the session key. This is
to make it easier for operators to manage sessions by not having to track/store
session tokens after creating a session. Session keys don't have to be
cryptographically secure, just unique per login.  A good key that works for
most people is the user's email.  If you want to allow multiple logins from
different user-agents simultaneously, then you might want to include some kind
of device ID in the key.

The cookie format is as follows:

    CookieName=SessionKey::SessionToken

The SessionKey is used to find the given session in the backend. If found, the
client SessionToken is then constant-time compared with the stored token.

## Security

Most of the existing solutions use encrypted cookies for authentication. This
enables you to have stateless sessions.  However, this strategy has two major
drawbacks:

- Single ultra-secret key.
- Hard to revoke sessions.

It's possible to alleviate these concerns, but in the process one will end up
making a stateful framework for revocation, and a complicated key management
strategy for de-risking the single point of failure key.

Why aren't we encrypting the cookie?

Encrypting the cookie implies the single secret key used to encrypt said
cookie.  Programs like [chamber](https://github.com/segmentio/chamber) can aid
in handling these secrets, but any developer can tell you that accidentally
logging environment variables is commonplace.  I'd rather reduce the secrets
required for my service to a minimum.

## Limitations

At this time, this library does not implement arbitrary key/value storage.

Also excluded from this library are flash sessions.  While useful, this is not a
concern for this library.

For either of these features, please see one of the libraries below.

## Alternatives

The most popular session management tool is in the gorilla toolkit. It uses
encrypted cookies by default.  Has a very large API.

https://github.com/gorilla/sessions

A comprehensive session management tool.  Also a very large API.  Heavy use of
naked interfaces.

https://github.com/kataras/go-sessions

Encrypted cookie manager by default.  Has middleware feature.  Big API. No easy
way to clear session without storing session token elsewhere.

https://github.com/alexedwards/scs

Lightweight, server-only API.  Uncertain about what the purpose of the Manager
interface is.  Heavy use of naked interface.

https://github.com/icza/session

Lightweight, server-only API.  Includes concept of Users in library. No
wrapping or middleware.

https://github.com/rivo/sessions
