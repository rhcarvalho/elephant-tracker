package main

import (
	"net/http"
)

type Context struct {
	Store Storage
}

type contextualHandlerFunc func(http.ResponseWriter, *http.Request, *Context)

func (h contextualHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ms := mgoSession.Clone()
	defer ms.Close()
	db := ms.DB(mgoDatabase)
	h(w, r, &Context{&MongoStore{db}})
}
