package git

// Fingerprint summarizes HEAD and every ref (branches, remote branches, tags,
// the stash) with their commits. Two equal fingerprints mean the log, labels
// and current branch would load the same, so polling it is a cheap way to
// notice changes made outside gitree. The index and working tree are not
// part of it (git status refreshes the index on its own).
func (r *Repo) Fingerprint() (string, error) {
	refs, err := r.run("for-each-ref", "--format=%(HEAD)%(refname) %(objectname)")
	if err != nil {
		return "", err
	}
	// Exit 1 without output: an unborn HEAD.
	head, err := r.runAllow([]int{1}, "rev-parse", "-q", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	return head + refs, nil
}
