package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
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

// SelectOption represents an option in a selection list.
type SelectOption struct {
	Value string // The value to return if selected
	Label string // The display label
}

// Select displays a numbered list and asks the user to select an option.
// Returns the selected option's Value, or empty string if cancelled.
func Select(prompt string, options []SelectOption) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	fmt.Println(prompt)
	fmt.Println()

	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt.Label)
	}

	fmt.Println()
	fmt.Print("Enter number (or 'q' to cancel): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" || response == "q" || response == "quit" || response == "cancel" {
		return "", nil
	}

	num, err := strconv.Atoi(response)
	if err != nil || num < 1 || num > len(options) {
		return "", fmt.Errorf("invalid selection: %s", response)
	}

	return options[num-1].Value, nil
}
