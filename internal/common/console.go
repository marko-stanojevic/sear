package common

import (
	"fmt"
	"os"
	"strings"
)

// PrintBannerMessage prints a visually distinct message box to stderr.
func PrintBannerMessage(title, content string) {
	maxLen := len(content)
	if len(title) > maxLen {
		maxLen = len(title)
	}
	width := maxLen + 4
	bar := strings.Repeat("─", width)

	titlePadTotal := width - len(title)
	if titlePadTotal < 0 {
		titlePadTotal = 0
	}
	leftPad := titlePadTotal / 2
	rightPad := titlePadTotal - leftPad
	leftPadStr := strings.Repeat(" ", leftPad)
	rightPadStr := strings.Repeat(" ", rightPad)

	fmt.Fprintf(os.Stderr, "\n┌%s┐\n│%s%s%s│\n│  %s  │\n└%s┘\n\n",
		bar, leftPadStr, title, rightPadStr, content, bar)
}
