package git

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CommitMeta is the metadata the preview header shows for a commit.
type CommitMeta struct {
	Hash                          string
	AuthorName, AuthorEmail       string
	AuthorDate                    time.Time
	CommitterName, CommitterEmail string
	Subject                       string
	Body                          string   // the full raw message (unsanitized: never print it as is)
	CoAuthors                     []string // names from Co-authored-by trailers
}

// CommitterDiffers reports whether someone other than the author committed.
func (c CommitMeta) CommitterDiffers() bool {
	return c.CommitterName != c.AuthorName || !strings.EqualFold(c.CommitterEmail, c.AuthorEmail)
}

// commitMetaFormat is NUL-separated with the free-form message last. Names
// and emails go through .mailmap, and the subject is %s, as in the table.
const commitMetaFormat = "%H%x00%aN%x00%aE%x00%at%x00%cN%x00%cE%x00%s%x00%B"

// CommitMeta reads the author, committer, date and message of a commit with
// a single git call.
func (r *Repo) CommitMeta(hash string) (CommitMeta, error) {
	out, err := r.run("show", "-s", "--no-show-signature", "--format="+commitMetaFormat, hash, "--")
	if err != nil {
		return CommitMeta{}, err
	}
	f := strings.SplitN(out, "\x00", 8)
	if len(f) != 8 {
		return CommitMeta{}, fmt.Errorf("git show %s: unexpected output", hash)
	}
	secs, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return CommitMeta{}, fmt.Errorf("git show %s: bad date %q", hash, f[3])
	}
	body := strings.TrimRight(f[7], "\n")
	return CommitMeta{
		Hash: f[0], AuthorName: SanitizeLine(f[1]), AuthorEmail: SanitizeLine(f[2]), AuthorDate: time.Unix(secs, 0),
		CommitterName: SanitizeLine(f[4]), CommitterEmail: SanitizeLine(f[5]),
		Subject: SanitizeLine(f[6]), Body: body, CoAuthors: parseCoAuthors(body),
	}, nil
}

// parseCoAuthors returns the names of the Co-authored-by trailers of msg
// (the email when a trailer has no name), without duplicates or control
// characters.
func parseCoAuthors(msg string) []string {
	const key = "co-authored-by:"
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < len(key) || !strings.EqualFold(line[:len(key)], key) {
			continue
		}
		v := strings.TrimSpace(SanitizeLine(line[len(key):]))
		name, email, _ := strings.Cut(v, "<")
		name = strings.TrimSpace(name)
		if name == "" {
			name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(email), ">"))
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
