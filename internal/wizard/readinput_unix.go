//go:build !windows

package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// ReadHiddenInput reads a password-like input from the terminal.
// On Unix systems it uses syscall.ReadPassword to hide input.
func ReadHiddenInput(prompt string) (string, error) {
	fmt.Print(prompt)

	fd := int(syscall.Stdin)
	b, err := syscall.ReadPassword(fd)
	fmt.Println()
	if err == nil {
		return string(b), nil
	}
	// Fallback to normal input if ReadPassword fails (e.g. not a terminal)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimSpace(line), nil
}
