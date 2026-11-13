// Package rustbind builds the four console commands copied from RustPilot
// into Rust's F1 console. No game memory, anti-cheat bypass or background input.
package rustbind

import (
	"fmt"
	"strings"
)

// Commands installs one queue bind, a refresh bind, and a *batch-aware*
// cancel bind. Rust's craft.cancel targets queued entries by item ID and
// their queued quantity. A one-size cancellation leaves other batches behind.
// We sweep possible batch sizes 10 down to 1 for every refund keypress.
// Repeated presses are bounded by RustPilot's RefundTries setting.
func Commands(queueKey, refreshKey, cancelKey, tcID string) string {
	cancels := make([]string, 0, 10)
	for qty := 10; qty >= 1; qty-- {
		cancels = append(cancels, fmt.Sprintf("craft.cancel %s %d", tcID, qty))
	}
	return fmt.Sprintf("bind %s \"craft.add %s 1\"\r\n"+
		"bind %s \"craft.add %s 1;craft.cancel %s 1;craft.add %s 1\"\r\n"+
		"bind %s \"%s\"\r\nwritecfg",
		queueKey, tcID, refreshKey, tcID, tcID, tcID, cancelKey, strings.Join(cancels, ";"))
}
