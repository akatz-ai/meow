package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm asks a yes/no question with the given default.
// Returns true for yes, false for no.
func Confirm(prompt string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}

	fmt.Printf("%s %s ", prompt, suffix)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" {
		return defaultYes, nil
	}

	return response == "y" || response == "yes", nil
}
