package ui

import (
	"os/exec"
	"strings"
)

// System is the model's access to the desktop: clipboard, browser and PATH.
// It is an interface so tests can record calls instead of performing them.
type System interface {
	Copy(text string) error
	OpenURL(url string) error
	LookPath(name string) (string, error)
}

// RealSystem implements System with macOS tools (pbcopy, open) and exec.LookPath.
type RealSystem struct{}

// Copy puts text on the clipboard via pbcopy.
func (RealSystem) Copy(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// OpenURL opens url in the default browser.
func (RealSystem) OpenURL(url string) error {
	return exec.Command("open", url).Run()
}

// LookPath reports where name is on PATH.
func (RealSystem) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}
