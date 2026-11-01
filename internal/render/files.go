package render

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/jamestelfer/dollop/internal/ignore"
)

// ListFiles returns the slash-separated paths of every file under fsys. A
// symlink is listed only when it resolves to a file that fsys can reach;
// symlinks to directories are omitted, and symlinks that fsys refuses to follow
// (an os.Root FS rejects targets outside the root) are skipped with a warning
// on stderr. Paths that rules ignore are left out, and the .gitignore files
// of the directories walked are loaded into rules; nil rules ignore nothing.
func ListFiles(fsys fs.FS, stderr io.Writer, rules *ignore.Rules) ([]string, error) {
	var paths []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." && rules.Ignored(p, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return rules.Load(fsys, p)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			info, serr := fs.Stat(fsys, p)
			if serr != nil {
				fmt.Fprintf(stderr, "warning: skipping %s: %v\n", p, serr) //nolint:errcheck
				return nil
			}
			if info.IsDir() {
				return nil
			}
		}
		paths = append(paths, p)
		return nil
	})
	return paths, err
}

// openSeekable opens p in fsys as an io.ReadSeekCloser, which the S3 SDK needs
// to determine Content-Length without buffering.
func openSeekable(fsys fs.FS, p string) (io.ReadSeekCloser, error) {
	f, err := fsys.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}
	rsc, ok := f.(io.ReadSeekCloser)
	if !ok {
		f.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("open %s: file is not seekable", p)
	}
	return rsc, nil
}
