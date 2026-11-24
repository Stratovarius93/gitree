package git

import (
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeLine(t *testing.T) {
	cases := map[string]string{
		"plain subject":                     "plain subject",
		"ünïcode — ok":                      "ünïcode — ok",
		"red \x1b[31mtext\x1b[0m":           "red text",
		"title \x1b]0;pwned\x07 after":      "title  after",
		"osc st \x1b]8;;http://x\x1b\\link": "osc st link",
		"bell\x07 bs\x08 cr\r nul\x00":      "bell bs cr nul",
		"c1 \u009b31mcsi":                   "c1 csi",
		"tab\there":                         "tab here",
		"reset \x1bc done":                  "reset  done",
		"trailing esc \x1b":                 "trailing esc ",
		"del\x7f":                           "del",
	}
	for in, want := range cases {
		assert.Equal(t, want, SanitizeLine(in), "%q", in)
	}
}

func TestRepoMetadataIsSanitized(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Git("-c", "user.name=Ev\x1b[31mil", "commit", "-q", "--allow-empty",
		"--author=Ma\x1b]0;pwn\x07llory <m@example.com>",
		"-m", "bad \x1b[2Jsubject\n\nCo-authored-by: Co\x1b[1m Author <c@example.com>")
	tr.Git("tag", "v\u009b1m2")
	tr.Git("branch", "feat\u009b2J")
	r, err := Open(tr.Dir)
	require.NoError(t, err)

	commits, err := r.Log(ScopeAll, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, commits)
	c := commits[0]
	assert.Equal(t, "bad subject", c.Subject)
	assert.Equal(t, "Mallory", c.AuthorName)
	var names []string
	for _, ref := range c.Refs {
		names = append(names, ref.Name)
	}
	assert.Contains(t, names, "v2")
	assert.Contains(t, names, "feat")

	meta, err := r.CommitMeta(c.Hash)
	require.NoError(t, err)
	assert.Equal(t, "bad subject", meta.Subject)
	assert.Equal(t, "Mallory", meta.AuthorName)
	assert.Equal(t, "Evil", meta.CommitterName)
	assert.Equal(t, []string{"Co Author"}, meta.CoAuthors)

	refs, err := r.ListRefs()
	require.NoError(t, err)
	for _, ref := range refs {
		assert.Equal(t, SanitizeLine(ref.Name), ref.Name)
	}
}
