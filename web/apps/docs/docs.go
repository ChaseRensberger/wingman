// Package docs embeds Wingman's official Markdown documentation.
package docs

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

const contentRoot = "src/content/docs"

//go:embed all:src/content/docs
var content embed.FS

// File is one bundled documentation page.
type File struct {
	Path    string
	Content string
	SHA256  string
}

// Files returns the bundled documentation pages in path order.
func Files() ([]File, error) {
	var files []File
	err := fs.WalkDir(content, contentRoot, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path.Ext(name) != ".md" {
			return nil
		}
		file, err := read(name)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Read returns one bundled documentation page by its path relative to docs.
func Read(name string) (File, error) {
	clean := path.Clean(name)
	if name == "" || path.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.Ext(clean) != ".md" {
		return File{}, fmt.Errorf("documentation path must name a relative Markdown file")
	}
	return read(path.Join(contentRoot, clean))
}

func read(name string) (File, error) {
	data, err := content.ReadFile(name)
	if err != nil {
		return File{}, err
	}
	relative := strings.TrimPrefix(name, contentRoot+"/")
	sum := sha256.Sum256(data)
	return File{Path: relative, Content: string(data), SHA256: fmt.Sprintf("%x", sum)}, nil
}
