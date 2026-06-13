//go:build windows

package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadHiddenInput reads a password-like input from the terminal.
// On Windows it falls back to normal (visible) input since
// syscall.ReadPassword is not available.
func ReadHiddenInput(prompt string) (string, error) {
	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimSpace(line), nil
}
