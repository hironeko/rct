package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var production embed.FS

func Dist() fs.FS {
	dist, err := fs.Sub(production, "dist")
	if err != nil {
		panic(err)
	}
	return dist
}
