package palette

import (
	"fmt"
	"os/exec"
)

// DetectBackend returns the first available palette backend found in PATH, in
// priority order: rofi, fuzzel, wofi, dmenu.
func DetectBackend() (string, error) {
	if _, err := exec.LookPath("rofi"); err == nil {
		return "rofi", nil
	}
	if _, err := exec.LookPath("fuzzel"); err == nil {
		return "fuzzel", nil
	}
	if _, err := exec.LookPath("wofi"); err == nil {
		return "wofi", nil
	}
	if _, err := exec.LookPath("dmenu"); err == nil {
		return "dmenu", nil
	}
	return "", fmt.Errorf("no palette backend found in PATH (looked for: rofi, fuzzel, wofi, dmenu)")
}

