package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html assets/*
var content embed.FS

// Handler serves the embedded UI file tree.
func Handler() http.Handler {
	return http.FileServer(http.FS(content))
}

// Assets returns only the assets subdirectory.
func Assets() http.Handler {
	sub, err := fs.Sub(content, "assets")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

// IndexHTML returns the main document bytes.
func IndexHTML() ([]byte, error) {
	return content.ReadFile("index.html")
}
