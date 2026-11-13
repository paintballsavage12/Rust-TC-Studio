package trader

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseNumber reads one manually entered planner field. It deliberately rejects
// invalid or out-of-range text; the app must not silently plan from stale values.
func ParseNumber(text string, max int) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("enter a number")
	}
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("only whole positive numbers are allowed")
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 || n > max {
		return 0, fmt.Errorf("value must be between 0 and %d", max)
	}
	return n, nil
}
