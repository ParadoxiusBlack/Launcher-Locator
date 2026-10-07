// Package assets embeds the web UI and seed data into the executable.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed static seed.json
var files embed.FS

// Static returns the web UI files rooted at the static directory.
func Static() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// Seed returns the bundled maps and indicator types as JSON.
func Seed() ([]byte, error) { return files.ReadFile("seed.json") }
