package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// loaded returns the rules for dir with the .gitignore files at the given
// slash-separated directories inside it loaded, parent first.
func loaded(t *testing.T, dir string, dirs ...string) *ignore.Rules {
	t.Helper()
	rules, err := ignore.ForDir(dir)
	require.NoError(t, err)
	fsys := os.DirFS(dir)
	for _, d := range append([]string{"."}, dirs...) {
		require.NoError(t, rules.Load(fsys, d))
	}
	return rules
}

func TestRules_NilIgnoresNothing(t *testing.T) {
	var rules *ignore.Rules
	assert.False(t, rules.Ignored(".git", true))
	assert.NoError(t, rules.Load(os.DirFS(t.TempDir()), "."))
}

func TestRules_LocalPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "# comment\n*.log\r\nbuild/\n!keep.log\n\n")
	writeFile(t, filepath.Join(dir, "sub", ".gitignore"), "local.txt\n")

	rules := loaded(t, dir, "sub")

	assert.True(t, rules.Ignored("debug.log", false))
	assert.True(t, rules.Ignored("sub/debug.log", false))
	assert.False(t, rules.Ignored("keep.log", false), "negation re-includes")
	assert.True(t, rules.Ignored("build", true))
	assert.False(t, rules.Ignored("build", false), "dir-only pattern does not match a file")
	assert.True(t, rules.Ignored("sub/local.txt", false))
	assert.False(t, rules.Ignored("local.txt", false), "nested .gitignore is scoped to its directory")
	assert.False(t, rules.Ignored("notes.md", false))
}

func TestRules_GitDirAlwaysIgnored(t *testing.T) {
	rules := loaded(t, t.TempDir())
	assert.True(t, rules.Ignored(".git", true))
	assert.True(t, rules.Ignored("vendor/lib/.git", true))
	assert.False(t, rules.Ignored(".git", false), "a .git file is not a repository directory")
}

func TestForDir_InheritsRepositoryPatterns(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	writeFile(t, filepath.Join(repo, ".gitignore"), "*.log\n/docs/drafts/\n")
	writeFile(t, filepath.Join(repo, "docs", ".gitignore"), "*.tmp\n")
	writeFile(t, filepath.Join(repo, "other", ".gitignore"), "*.md\n")
	dir := filepath.Join(repo, "docs")

	rules := loaded(t, dir)

	assert.True(t, rules.Ignored("app.log", false))
	assert.True(t, rules.Ignored("drafts", true), "anchored pattern resolves from the repository root")
	assert.True(t, rules.Ignored("scratch.tmp", false))
	assert.False(t, rules.Ignored("guide.md", false), "a sibling directory's .gitignore does not apply")
}

func TestForDir_RepositoryInWorktree(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".git"), "gitdir: /elsewhere\n")
	writeFile(t, filepath.Join(repo, ".gitignore"), "*.log\n")
	dir := filepath.Join(repo, "site")
	require.NoError(t, os.Mkdir(dir, 0o700))

	assert.True(t, loaded(t, dir).Ignored("app.log", false))
}

func TestForDir_IgnoredDirectoryKeepsOtherInheritedPatterns(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	writeFile(t, filepath.Join(repo, ".gitignore"), "dist/\n*.log\nnode_modules/\n")
	writeFile(t, filepath.Join(repo, "dist", ".gitignore"), "*.map\n")
	dir := filepath.Join(repo, "dist")

	rules := loaded(t, dir)

	assert.False(t, rules.Ignored("index.html", false))
	assert.False(t, rules.Ignored("assets/site.css", false), "the pattern ignoring dist does not reach beneath it")
	assert.True(t, rules.Ignored("app.log", false))
	assert.True(t, rules.Ignored("node_modules", true))
	assert.True(t, rules.Ignored("app.js.map", false), "the directory's own .gitignore applies")
}

func TestForDir_IgnoredAncestorKeepsOtherInheritedPatterns(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	writeFile(t, filepath.Join(repo, ".gitignore"), "/build/\n*.log\n")
	dir := filepath.Join(repo, "build", "site")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	rules := loaded(t, dir)

	assert.False(t, rules.Ignored("index.html", false))
	assert.True(t, rules.Ignored("app.log", false))
}

func TestRules_MatchReportsReason(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	writeFile(t, filepath.Join(repo, ".gitignore"), "*.log\nsecrets.env\n")

	rules := loaded(t, repo)

	assert.Equal(t, ignore.NotIgnored, rules.Match("index.html", false))
	assert.Equal(t, ignore.Gitignored, rules.Match("app.log", false))
	assert.Equal(t, ignore.Sensitive, rules.Match("server.pem", false))
	assert.Equal(t, ignore.Gitignored, rules.Match("secrets.env", false), "a .gitignore match takes precedence over the sensitive list")
	assert.Equal(t, ignore.GitDir, rules.Match(".git", true))
}

func TestForDir_OutsideRepository(t *testing.T) {
	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, ".gitignore"), "*.log\n")
	dir := filepath.Join(parent, "upload")
	require.NoError(t, os.Mkdir(dir, 0o700))

	assert.False(t, loaded(t, dir).Ignored("app.log", false), "a .gitignore outside any repository is not inherited")
}

func TestForDir_SensitiveFilesIgnored(t *testing.T) {
	rules := loaded(t, t.TempDir())

	for _, p := range []string{".env", "config/.env.production", ".envrc", "app/.envrc.local", "tls/server.pem", "deploy.key", "id_ed25519", "id_rsa.pub", "credentials", "infra/terraform.tfstate", ".npmrc"} {
		assert.True(t, rules.Ignored(p, false), p)
	}
	for _, p := range []string{".aws", "home/.ssh"} {
		assert.True(t, rules.Ignored(p, true), p)
	}
	for _, p := range []string{".env.example", ".envrc.example", ".envrc.sample", "README.md", "keys.md", "credentials.md"} {
		assert.False(t, rules.Ignored(p, false), p)
	}
}

func TestForDir_SensitiveFilesIgnoredInIgnoredDirectory(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	writeFile(t, filepath.Join(repo, ".gitignore"), "dist/\n")
	dir := filepath.Join(repo, "dist")
	require.NoError(t, os.Mkdir(dir, 0o700))

	rules := loaded(t, dir)

	assert.False(t, rules.Ignored("index.html", false))
	assert.True(t, rules.Ignored(".env", false))
}

func TestForDir_GitignoreNegationReincludesSensitiveFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "!public.pem\n")

	rules := loaded(t, dir)

	assert.False(t, rules.Ignored("public.pem", false))
	assert.True(t, rules.Ignored("private.pem", false))
}

func TestForDir_SensitivePatternsScopedToDirectory(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	dir := filepath.Join(repo, ".aws", "site")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	assert.False(t, loaded(t, dir).Ignored("index.html", false), "an ancestor's name does not trigger a sensitive pattern")
}
