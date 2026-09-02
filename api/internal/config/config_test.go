package config_test

import (
	"testing"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/config"
)

// Raising a cap is an env change and nothing else.
func TestCapsAreEnv(t *testing.T) {
	t.Setenv("CAP_GREEN", "500")
	t.Setenv("CAP_BUDGET", "1000")
	caps := config.Load()
	if caps.PerTier[capacity.Green] != 500 || caps.Budget != 1000 {
		t.Fatalf("env caps not honoured: %+v", caps)
	}
	if caps.PerTier[capacity.Pink] != 1 || caps.PerTier[capacity.Blue] != 3 {
		t.Fatalf("unset caps must keep the brief's defaults: %+v", caps)
	}
}

func TestBadEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("CAP_BLUE", "three")
	if got := config.Load().PerTier[capacity.Blue]; got != 3 {
		t.Fatalf("want default 3 for unparsable CAP_BLUE, got %d", got)
	}
}
