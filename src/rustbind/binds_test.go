package rustbind

import (
	"fmt"
	"strings"
	"testing"
)

func TestFullCancelBindCoversEveryBatchQuantity(t *testing.T) {
	v := Commands("keypad7", "keypad8", "keypad9", "-97956382")
	lines := strings.Split(v, "\r\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 F1 commands, got %d", len(lines))
	}
	if lines[3] != "writecfg" {
		t.Fatalf("missing writecfg")
	}
	if !strings.Contains(lines[1], "craft.cancel -97956382 1") {
		t.Fatal("refresh missing explicit quantity")
	}
	if !strings.HasPrefix(lines[2], "bind keypad9 \"craft.cancel -97956382 10;") {
		t.Fatal("cancel key wrong")
	}
	for qty := 10; qty >= 1; qty-- {
		cmd := fmt.Sprintf("craft.cancel -97956382 %d", qty)
		if n := strings.Count(lines[2], cmd+";"); qty > 1 && n != 1 {
			t.Fatalf("quantity %d: expected 1, got %d", qty, n)
		}
		if qty == 1 && !strings.HasSuffix(lines[2], cmd+"\"") {
			t.Fatal("missing final qty1")
		}
	}
	if strings.Count(lines[2], "craft.cancel") != 10 {
		t.Fatal("cancel bind must have exactly ten cancel commands")
	}
}
