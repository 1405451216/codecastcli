//go:build !windows

package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ReadHiddenInput reads a password-like input from the terminal.
// On Unix systems it uses golang.org/x/term.ReadPassword to hide input.
// 注意：syscall.ReadPassword 在 Go 1.17 后被废弃并移除，必须用 x/term。
func ReadHiddenInput(prompt string) (string, error) {
	fmt.Print(prompt)

	fd := int(os.Stdin.Fd())
	b, err := term.ReadPassword(fd)
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
