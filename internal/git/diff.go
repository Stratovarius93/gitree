package git

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// DiffTarget selects what to diff. Hash == "" means working tree vs HEAD.
// OldPath is the pre-rename path of the file being diffed (optional); with it
// both paths are passed so git can pair them and show a rename, not an add.
type DiffTarget struct{ Hash, OldPath string }

// LookPath locates executables; overridable in tests.
var LookPath = exec.LookPath

// DeltaArgs are the delta flags (from the lazygit config), one argv element each.
var DeltaArgs = []string{
	"--dark", "--paging=never", "--line-numbers", "--syntax-theme=gruvbox-dark",
	`--plus-style=syntax "#34381b"`, `--plus-emph-style=syntax "#4b5a26"`,
	`--minus-style=syntax "#402120"`, `--minus-emph-style=syntax "#6a2e2b"`,
	"--line-numbers-plus-style=#a9b665", "--line-numbers-minus-style=#ea6962",
	"--line-numbers-zero-style=#7c6f64",
	"--hunk-header-style=file line-number syntax", "--hunk-header-decoration-style=#665c54 box",
	"--file-style=#d8a657 bold", "--file-decoration-style=#665c54 ul",
}

// Diff returns the colored diff for a commit or the working tree, piped
// through delta when available. path "" means the whole commit/working tree.
func (r *Repo) Diff(t DiffTarget, path string, width int) (string, error) {
	var raw string
	var err error
	switch {
	case t.Hash != "":
		args := []string{"--color=always", "-M"}
		if _, perr := r.run("rev-parse", "--verify", "-q", t.Hash+"^"); perr == nil {
			args = append([]string{"diff"}, append(args, t.Hash+"^", t.Hash)...)
		} else {
			args = append([]string{"show", "--format=", "--root"}, append(args, t.Hash)...)
		}
		args = appendPaths(args, path, t.OldPath)
		raw, err = r.run(args...)
	case path != "" && r.isUntracked(path):
		// exit code 1 means "differences found"
		raw, err = r.runAllow([]int{1}, "diff", "--no-index", "--color=always", "/dev/null", path)
	default:
		args := []string{"diff", "HEAD", "--color=always", "-M"}
		args = appendPaths(args, path, t.OldPath)
		raw, err = r.run(args...)
	}
	if err != nil {
		return "", err
	}
	if out, ok := runDelta(raw, width); ok {
		return out, nil
	}
	return raw, nil
}

func appendPaths(args []string, path, oldPath string) []string {
	if path == "" {
		return args
	}
	args = append(args, "--", path)
	if oldPath != "" && oldPath != path {
		args = append(args, oldPath)
	}
	return args
}

func (r *Repo) isUntracked(path string) bool {
	out, err := r.run("ls-files", "--others", "--exclude-standard", "--", path)
	return err == nil && strings.TrimSpace(out) != ""
}

// runDelta pipes raw through delta; ok is false when delta is unavailable or fails.
func runDelta(raw string, width int) (string, bool) {
	bin, err := LookPath("delta")
	if err != nil {
		return "", false
	}
	args := append([]string{}, DeltaArgs...)
	args = append(args, "--width="+strconv.Itoa(width))
	if width > 160 {
		args = append(args, "--side-by-side")
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(raw)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return out.String(), true
}

// ShowHeader returns the full hash, author, date and full message of a commit.
func (r *Repo) ShowHeader(hash string) (string, error) {
	out, err := r.run("show", "-s", "--format=commit %H%nAuthor: %an <%ae>%nDate:   %aD%n%n%B", hash)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}
