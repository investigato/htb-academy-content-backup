package main

import (
	"fmt"
	"regexp"
	"strings"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := nonAlphanumericRegex.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(slug, "-")
}

// "Active Directory Enumeration & Attacks", "143" -> "active-directory-enumeration-attacks-module-143.md"
func moduleFilename(name, moduleID string) string {
	return fmt.Sprintf("%s-module-%s.md", slugify(name), moduleID)
}

// e.g. "Active Directory Enumeration & Attacks", "143" -> "active-directory-enumeration-attacks-module-143-walkthrough.md"
func walkthroughFilename(name, moduleID string) string {
	return fmt.Sprintf("%s-module-%s-walkthrough.md", slugify(name), moduleID)
}

func cleanMarkdown(sections []string) string {
	var markdown string
	for _, content := range sections {
		markdown += content + "\n\n\n"
	}

	// Strip some content for proper code blocks.
	markdown = strings.ReplaceAll(markdown, "shell-session", "shell")
	markdown = strings.ReplaceAll(markdown, "powershell-session", "powershell")
	markdown = strings.ReplaceAll(markdown, "cmd-session", "shell")
	// Remove bash prompts - handle both with and without leading space
	markdown = strings.ReplaceAll(markdown, " [!bash!]$ ", " ")
	markdown = strings.ReplaceAll(markdown, "[!bash!]$ ", "")

	return markdown
}
