package selfhealing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluefermion/feedback/internal/model"
)

// TestCanTriggerRefusesWhileKillswitchEngaged proves the lockout outranks
// everything: the config below would otherwise be a valid opencode run
// (enabled, admin, allowed type), and CanTrigger must refuse before it ever
// gets to the Docker checks.
func TestCanTriggerRefusesWhileKillswitchEngaged(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		lockout  bool
		wantOK   bool
		wantWord string
	}{
		{"opencode: killswitch engaged", "opencode", true, false, "killswitch engaged"},
		{"analyze: killswitch engaged (global — a human pulled the plug)", "analyze", true, false, "killswitch engaged"},
		{"analyze: armed, runs", "analyze", false, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockout := filepath.Join(t.TempDir(), "KILLSWITCH")
			if tt.lockout {
				if err := os.WriteFile(lockout, []byte("KILLSWITCH ENGAGED\nreason: law 3 — printed .env\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			tr := NewTrigger(Config{
				Enabled:        true,
				Mode:           tt.mode,
				AdminEmails:    []string{"admin@example.com"},
				AllowedTypes:   []string{"bug"},
				LLMAPIKey:      "test",
				KillswitchFile: lockout,
			})
			ok, reason := tr.CanTrigger(&model.Feedback{Type: "bug", UserEmail: "admin@example.com"})
			if ok != tt.wantOK {
				t.Fatalf("CanTrigger() = %v (%q), want %v", ok, reason, tt.wantOK)
			}
			if tt.wantWord != "" && !strings.Contains(reason, tt.wantWord) {
				t.Errorf("reason = %q, want it to mention %q", reason, tt.wantWord)
			}
			if tt.lockout && !strings.Contains(reason, "law 3") {
				t.Errorf("reason = %q, want it to surface why the plug was pulled", reason)
			}
		})
	}
}
