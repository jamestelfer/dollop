// Package ignore decides which files of an uploaded directory git would
// ignore, using the .gitignore files of the directory and of its enclosing
// repository.
package ignore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const (
	ignoreFile = ".gitignore"
	gitDir     = ".git"
)

// Rules holds the gitignore patterns that apply to paths under an uploaded
// directory. It starts with the patterns inherited from the enclosing
// repository (see ForDir) and grows as Load reads the .gitignore files inside
// the directory. A nil *Rules ignores nothing.
type Rules struct {
	// base is the uploaded directory's path from the repository root, so that
	// inherited and local patterns share one coordinate system.
	base     []string
	patterns []gitignore.Pattern
}

// ForDir returns the rules dir inherits from the .gitignore files between the
// root of its enclosing git repository and dir's parent. When those rules
// ignore dir itself they are dropped: naming an ignored directory, such as a
// build output, is a request to upload it.
func ForDir(dir string) (*Rules, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", dir, err)
	}
	repo, ok := repoRoot(abs)
	if !ok {
		return &Rules{}, nil
	}
	rel, err := filepath.Rel(repo, abs)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", dir, err)
	}

	r := &Rules{}
	if rel != "." {
		r.base = strings.Split(filepath.ToSlash(rel), "/")
	}
	repoFS := os.DirFS(repo)
	for i := range r.base {
		ancestor := r.base[:i]
		if err := r.read(repoFS, path.Join(append(slices.Clone(ancestor), ignoreFile)...), ancestor); err != nil {
			return nil, err
		}
	}
	if gitignore.NewMatcher(r.patterns).Match(r.base, true) {
		r.patterns = nil
	}
	return r, nil
}

// repoRoot returns the nearest directory at or above abs holding a .git
// entry: a directory in a normal checkout, a file in a worktree or submodule.
func repoRoot(abs string) (string, bool) {
	for d := abs; ; {
		if _, err := os.Lstat(filepath.Join(d, gitDir)); err == nil {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// Load adds the patterns of the .gitignore file in dir, a slash-separated
// path relative to the uploaded directory that fsys is rooted at. Directories
// must be loaded parent first, as fs.WalkDir visits them.
func (r *Rules) Load(fsys fs.FS, dir string) error {
	if r == nil {
		return nil
	}
	return r.read(fsys, path.Join(dir, ignoreFile), r.under(dir))
}

// Ignored reports whether p, a slash-separated path relative to the uploaded
// directory, is ignored. .git directories are always ignored.
func (r *Rules) Ignored(p string, isDir bool) bool {
	if r == nil {
		return false
	}
	if isDir && path.Base(p) == gitDir {
		return true
	}
	return gitignore.NewMatcher(r.patterns).Match(r.under(p), isDir)
}

// under returns the components of p, relative to the uploaded directory, as
// a path from the repository root.
func (r *Rules) under(p string) []string {
	components := slices.Clone(r.base)
	if p == "." {
		return components
	}
	return append(components, strings.Split(p, "/")...)
}

// read adds the patterns of the gitignore file name in fsys, scoped to the
// directory domain.
func (r *Rules) read(fsys fs.FS, name string, domain []string) error {
	data, err := fs.ReadFile(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r.patterns = append(r.patterns, gitignore.ParsePattern(line, domain))
	}
	return nil
}
