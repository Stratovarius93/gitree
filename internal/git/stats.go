package git

import (
	"strconv"
	"strings"
)

// Stat is the number of lines a commit adds and deletes, over all files.
type Stat struct {
	Added, Deleted int
}

// Stats returns the line totals of each commit in hashes, from a single git
// call. Every commit git reports gets an entry; merges (no diff by default),
// empty commits and binary-only changes are the zero Stat.
func (r *Repo) Stats(hashes []string) (map[string]Stat, error) {
	res := map[string]Stat{}
	if len(hashes) == 0 {
		return res, nil
	}
	args := append([]string{"log", "--no-walk=unsorted", "--format=%x1e%H", "--numstat"}, hashes...)
	out, err := r.run(append(args, "--")...)
	if err != nil {
		return nil, err
	}
	for _, rec := range strings.Split(out, "\x1e") {
		lines := strings.Split(strings.TrimSpace(rec), "\n")
		hash := strings.TrimSpace(lines[0])
		if hash == "" {
			continue
		}
		var s Stat
		for _, l := range lines[1:] {
			f := strings.SplitN(l, "\t", 3)
			if len(f) < 3 {
				continue
			}
			a, _ := strconv.Atoi(f[0]) // binary files: "-" counts as 0
			d, _ := strconv.Atoi(f[1])
			s.Added += a
			s.Deleted += d
		}
		res[hash] = s
	}
	return res, nil
}
