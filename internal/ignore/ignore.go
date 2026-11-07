// Package ignore decides which files of an uploaded directory to leave out:
// those git would ignore, using the .gitignore files of the directory and of
// its enclosing repository, and those named like credentials or keys.
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

// sensitive matches files that commonly hold credentials or private keys. It
// applies whatever the .gitignore files say, as their lowest-priority
// patterns, so a negation in one can re-include a match.
var sensitive = []string{
	".env",
	".env.*",
	"!.env.example",
	"!.env.sample",
	"!.env.template",
	".envrc",
	".envrc.*",
	"!.envrc.example",
	"!.envrc.sample",
	"!.envrc.template",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"*.jks",
	"*.keystore",
	"*.kdbx",
	"id_rsa*",
	"id_dsa*",
	"id_ecdsa*",
	"id_ed25519*",
	".ssh/",
	".gnupg/",
	".aws/",
	"credentials",
	"credentials.json",
	".netrc",
	".npmrc",
	".pypirc",
	".git-credentials",
	".htpasswd",
	"*.tfstate",
	"*.tfstate.backup",
}

// Reason says why a path is left out of an upload; NotIgnored means it is not.
type Reason string

const (
	NotIgnored Reason = ""
	GitDir     Reason = "git directory"
	Gitignored Reason = "gitignored"
	Sensitive  Reason = "sensitive"
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
	// sensitive counts the leading patterns that come from the sensitive list.
	sensitive int
}

// ForDir returns the rules dir inherits from the .gitignore files between the
// root of its enclosing git repository and dir's parent. A pattern that
// ignores dir or one of its ancestors is dropped, and the rest apply: naming
// an ignored directory, such as a build output, is a request to upload it.
// The sensitive patterns precede them, scoped to dir, and are never dropped.
func ForDir(dir string) (*Rules, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", dir, err)
	}
	r := &Rules{}
	if repo, ok := repoRoot(abs); ok {
		if err := r.inherit(repo, abs); err != nil {
			return nil, err
		}
	}
	defaults := make([]gitignore.Pattern, 0, len(sensitive)+len(r.patterns))
	for _, p := range sensitive {
		defaults = append(defaults, gitignore.ParsePattern(p, r.base))
	}
	r.patterns = append(defaults, r.patterns...)
	r.sensitive = len(sensitive)
	return r, nil
}

// inherit sets base to abs's path within repo and reads the .gitignore files
// of abs's ancestors in repo, keeping only the patterns that cannot ignore abs.
func (r *Rules) inherit(repo, abs string) error {
	rel, err := filepath.Rel(repo, abs)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", abs, err)
	}
	if rel != "." {
		r.base = strings.Split(filepath.ToSlash(rel), "/")
	}
	repoFS := os.DirFS(repo)
	for i := range r.base {
		ancestor := r.base[:i]
		if err := r.read(repoFS, path.Join(append(slices.Clone(ancestor), ignoreFile)...), ancestor); err != nil {
			return err
		}
	}
	r.patterns = slices.DeleteFunc(r.patterns, r.excludesBase)
	return nil
}

// excludesBase reports whether p excludes the uploaded directory or one of
// its ancestors. A pattern matching a directory matches everything beneath
// it, so only such a pattern can ignore the whole upload.
func (r *Rules) excludesBase(p gitignore.Pattern) bool {
	for i := 1; i <= len(r.base); i++ {
		if p.Match(r.base[:i], true) == gitignore.Exclude {
			return true
		}
	}
	return false
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
// directory, is left out of the upload.
func (r *Rules) Ignored(p string, isDir bool) bool {
	return r.Match(p, isDir) != NotIgnored
}

// Match reports why p, a slash-separated path relative to the uploaded
// directory, is left out of the upload. .git directories always are.
func (r *Rules) Match(p string, isDir bool) Reason {
	if r == nil {
		return NotIgnored
	}
	if isDir && path.Base(p) == gitDir {
		return GitDir
	}
	components := r.under(p)
	if !gitignore.NewMatcher(r.patterns).Match(components, isDir) {
		return NotIgnored
	}
	// The sensitive patterns have the lowest priority, so the .gitignore
	// patterns alone decide whenever they match.
	if gitignore.NewMatcher(r.patterns[r.sensitive:]).Match(components, isDir) {
		return Gitignored
	}
	return Sensitive
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
