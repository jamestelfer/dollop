package render

import (
	"fmt"
	"io"
	"io/fs"
	"slices"

	"github.com/jamestelfer/dollop/internal/ignore"
)

// ListFiles returns the slash-separated paths of every file under fsys. A
// symlink is listed only when it resolves to a file that fsys can reach;
// symlinks to directories are omitted, and symlinks that fsys refuses to follow
// (an os.Root FS rejects targets outside the root) are skipped with a warning
// on stderr. Paths that rules ignore are left out, each reported on stderr
// and counted in skipped, and the .gitignore files of the directories walked
// are loaded into rules; nil rules ignore nothing. A symlink to an ignored
// file is left out too.
func ListFiles(fsys fs.FS, stderr io.Writer, rules *ignore.Rules) (paths []string, skipped int, err error) {
	links := map[string]bool{}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return rules.Load(p)
		}
		if reason := rules.Match(p, d.IsDir()); reason != ignore.NotIgnored {
			skipped++
			if d.IsDir() {
				fmt.Fprintf(stderr, "skipping [%s/] (%s)\n", p, reason) //nolint:errcheck
				return fs.SkipDir
			}
			fmt.Fprintf(stderr, "skipping [%s] (%s)\n", p, reason) //nolint:errcheck
			return nil
		}
		if d.IsDir() {
			return rules.Load(p)
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
			links[p] = true
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil || len(links) == 0 {
		return paths, skipped, err
	}
	// Link targets are checked once every .gitignore has loaded, as a target
	// may sit in a directory the walk reaches after the link.
	paths = slices.DeleteFunc(paths, func(p string) bool {
		if !links[p] {
			return false
		}
		reason, terr := rules.MatchTarget(p)
		if terr != nil {
			err = terr
		}
		if reason == ignore.NotIgnored {
			return false
		}
		skipped++
		fmt.Fprintf(stderr, "skipping [%s] (link target: %s)\n", p, reason) //nolint:errcheck
		return true
	})
	return paths, skipped, err
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
