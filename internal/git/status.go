package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FileChange is one changed file. Status is one of 'M','A','D','R','?'.
type FileChange struct {
	Status  byte
	Path    string
	Staged  bool // some of the change is in the index (TrackedChanges only)
	OldPath string
	Added   int
	Deleted int
}

// WorkingFiles lists tracked changes (staged+unstaged vs HEAD) plus untracked
// files as '?'.
func (r *Repo) WorkingFiles() ([]FileChange, error) {
	out, err := r.run("status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	files := parseStatus(out)
	nums, err := r.numstat("diff", "HEAD", "--numstat", "-z", "-M")
	if err != nil {
		return nil, err
	}
	for i := range files {
		f := &files[i]
		if f.Status == '?' {
			f.Added = countLines(filepath.Join(r.root, f.Path))
			continue
		}
		if n, ok := nums[f.Path]; ok {
			f.Added, f.Deleted = n[0], n[1]
		}
	}
	return files, nil
}

// TrackedChanges lists the tracked files that differ from HEAD (staged or
// not), without line counts: what ErrDirty is about.
func (r *Repo) TrackedChanges() ([]FileChange, error) {
	out, err := r.run("status", "--porcelain=v2", "-z", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// parseStatus parses `git status --porcelain=v2 -z` output.
func parseStatus(out string) []FileChange {
	var files []FileChange
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '?':
			files = append(files, FileChange{Status: '?', Path: rec[2:]})
		case '1', 'u':
			n := 9
			if rec[0] == 'u' {
				n = 11
			}
			f := strings.SplitN(rec, " ", n)
			if len(f) < n {
				continue
			}
			files = append(files, FileChange{Status: statusFromXY(f[1]), Path: f[n-1], Staged: f[1][0] != '.'})
		case '2':
			f := strings.SplitN(rec, " ", 10)
			if len(f) < 10 {
				continue
			}
			fc := FileChange{Status: 'R', Path: f[9], Staged: f[1][0] != '.'}
			if i+1 < len(recs) {
				i++
				fc.OldPath = recs[i]
			}
			files = append(files, fc)
		}
	}
	return files
}

// HasTrackedChanges reports whether any tracked file differs from HEAD.
func (r *Repo) HasTrackedChanges() (bool, error) {
	out, err := r.run("status", "--porcelain=v2", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// CommitFiles lists files changed by a commit versus its first parent (or the
// empty tree for a root commit).
func (r *Repo) CommitFiles(hash string) ([]FileChange, error) {
	base := []string{"diff-tree", "-r", "-M", "--no-commit-id", "-z"}
	var rng []string
	if _, err := r.run("rev-parse", "--verify", "-q", hash+"^"); err == nil {
		rng = []string{hash + "^", hash}
	} else {
		base = append(base, "--root")
		rng = []string{hash}
	}
	ns, err := r.run(append(append(append([]string{}, base...), "--name-status"), rng...)...)
	if err != nil {
		return nil, err
	}
	nums, err := r.numstat(append(append(append([]string{}, base...), "--numstat"), rng...)...)
	if err != nil {
		return nil, err
	}
	var files []FileChange
	recs := strings.Split(ns, "\x00")
	for i := 0; i < len(recs); i++ {
		code := recs[i]
		if code == "" {
			continue
		}
		fc := FileChange{Status: statusFromXY(code[:1])}
		if i+1 >= len(recs) {
			break
		}
		i++
		fc.Path = recs[i]
		if code[0] == 'R' || code[0] == 'C' {
			if i+1 >= len(recs) {
				break
			}
			i++
			fc.OldPath, fc.Path = fc.Path, recs[i]
		}
		if n, ok := nums[fc.Path]; ok {
			fc.Added, fc.Deleted = n[0], n[1]
		}
		files = append(files, fc)
	}
	return files, nil
}

// statusFromXY maps a porcelain XY code (or a single letter) to a Status.
func statusFromXY(xy string) byte {
	for i := 0; i < len(xy); i++ {
		switch xy[i] {
		case 'M', 'T', 'U':
			return 'M'
		case 'A', 'C':
			return 'A'
		case 'D':
			return 'D'
		case 'R':
			return 'R'
		}
	}
	return 'M'
}

// numstat runs a `--numstat -z` git command and returns [added, deleted] by
// (new) path. Binary files count as 0/0.
func (r *Repo) numstat(args ...string) (map[string][2]int, error) {
	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}
	res := map[string][2]int{}
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		f := strings.SplitN(recs[i], "\t", 3)
		if len(f) < 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		d, _ := strconv.Atoi(f[1])
		path := f[2]
		if path == "" { // rename: old and new follow as separate records
			if i+2 >= len(recs) {
				return nil, fmt.Errorf("malformed numstat output")
			}
			path = recs[i+2]
			i += 2
		}
		res[path] = [2]int{a, d}
	}
	return res, nil
}

// countLines counts lines of a small text file (0 for binary/unreadable).
func countLines(path string) int {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return 0
	}
	b, err := os.ReadFile(path)
	if err != nil || strings.IndexByte(string(b), 0) >= 0 {
		return 0
	}
	n := strings.Count(string(b), "\n")
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}
