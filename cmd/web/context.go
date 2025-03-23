package main

import (
	"context"
	"net/http"
	"talent-rodeo/internal"
)

type contextKey string

const (
	authenticatedUserContextKey = contextKey("authenticatedUser")
)

func contextSetAuthenticatedUser(r *http.Request, user *internal.User) *http.Request {
	ctx := context.WithValue(r.Context(), authenticatedUserContextKey, user)
	return r.WithContext(ctx)
}

func contextGetAuthenticatedUser(r *http.Request) *internal.User {
	user, ok := r.Context().Value(authenticatedUserContextKey).(*internal.User)
	if !ok {
		return nil
	}

	return user
}
