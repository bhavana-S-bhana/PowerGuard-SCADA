package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFS embed.FS

func getStaticFileSystem() http.FileSystem {
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
