package capacity_test

import (
	"errors"
	"testing"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

// testCaps mirrors the README defaults: sub-caps sum to 9, budget is 8.
func testCaps() capacity.Caps {
	return capacity.Caps{
		Budget: 8,
		PerTier: map[capacity.Tier]int{
			capacity.Pink:  1,
			capacity.Blue:  3,
			capacity.Green: 5,
		},
	}
}

func counts(pink, blue, green int) capacity.Counts {
	return capacity.Counts{capacity.Pink: pink, capacity.Blue: blue, capacity.Green: green}
}

// wantRefusal asserts err is a Refusal for the given reason and checks the
// numbers it carries, because the resolver turns those into the sentence the
// user reads.
func wantRefusal(t *testing.T, err error, reason error, used, cap int) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %v, got nil", reason)
	}
	if !errors.Is(err, reason) {
		t.Fatalf("want %v, got %v", reason, err)
	}
	var r *capacity.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("want a *Refusal, got %T (%v)", err, err)
	}
	if r.Used != used || r.Cap != cap {
		t.Fatalf("refusal numbers: want used=%d cap=%d, got used=%d cap=%d", used, cap, r.Used, r.Cap)
	}
}

func wantOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

// Rule 1 - the shared budget binds before the sub-cap.
// 3 in Blue and 5 in Green is 8 of 8, so Pink is unreachable even though
// Pink is empty and its cap is 1.
func TestBudgetBindsBeforeSubCap(t *testing.T) {
	caps := testCaps()
	have := counts(0, 3, 5) // 8 of 8, Pink empty

	err := capacity.CanAdd(caps, have, capacity.Pink)
	wantRefusal(t, err, capacity.ErrBudgetFull, 8, 8)
	if errors.Is(err, capacity.ErrTierFull) {
		t.Fatalf("Pink is empty; the refusal must be the budget, not the tier: %v", err)
	}

	// The same budget refuses every tier, full or not.
	for _, tier := range capacity.Tiers() {
		wantRefusal(t, capacity.CanAdd(caps, have, tier), capacity.ErrBudgetFull, 8, 8)
	}

	// One seat back and Pink is reachable again.
	wantOK(t, capacity.CanAdd(caps, counts(0, 3, 4), capacity.Pink))
}

// Rule 1b - a full tier fails even when the budget has room.
func TestTierFullWithBudgetRemaining(t *testing.T) {
	caps := testCaps()

	// 1 of 8 spent, Pink full at 1 of 1.
	have := counts(1, 0, 0)
	wantRefusal(t, capacity.CanAdd(caps, have, capacity.Pink), capacity.ErrTierFull, 1, 1)
	wantOK(t, capacity.CanAdd(caps, have, capacity.Blue))
	wantOK(t, capacity.CanAdd(caps, have, capacity.Green))

	// 3 of 8 spent, Blue full at 3 of 3.
	have = counts(0, 3, 0)
	wantRefusal(t, capacity.CanAdd(caps, have, capacity.Blue), capacity.ErrTierFull, 3, 3)
	wantOK(t, capacity.CanAdd(caps, have, capacity.Pink))
	wantOK(t, capacity.CanAdd(caps, have, capacity.Green))

	// The refusal names the tier so the user can be told which one.
	var r *capacity.Refusal
	if err := capacity.CanAdd(caps, have, capacity.Blue); !errors.As(err, &r) || r.Tier != capacity.Blue {
		t.Fatalf("want a Blue refusal, got %v", err)
	}
}

// Rule 2 - a pending request spends no seat, so sending is a budget question
// only, and one free seat permits any number of outstanding requests.
func TestSendChecksBudgetOnly(t *testing.T) {
	caps := testCaps()

	// 7 of 8: Pink and Blue are both full, one seat is free.
	have := counts(1, 3, 3)
	wantOK(t, capacity.CanSend(caps, have))

	// Full tiers do not block sending, because nothing is filed until accept.
	// The same person is refused an *add* into those tiers right now.
	wantRefusal(t, capacity.CanAdd(caps, have, capacity.Pink), capacity.ErrTierFull, 1, 1)
	wantRefusal(t, capacity.CanAdd(caps, have, capacity.Blue), capacity.ErrTierFull, 3, 3)

	// Sending never spends the seat, so the answer does not change no matter
	// how many requests are already outstanding: the counts are the same.
	for i := 0; i < 100; i++ {
		wantOK(t, capacity.CanSend(caps, have))
	}

	// Once the budget is spent there is nothing an accept could ever succeed
	// into, so sending is refused up front.
	wantRefusal(t, capacity.CanSend(caps, counts(0, 3, 5)), capacity.ErrBudgetFull, 8, 8)
}

// Rule 3 - re-filing checks the destination sub-cap and never the budget,
// because the contact is already inside the budget.
func TestMoveIgnoresBudget(t *testing.T) {
	caps := testCaps()

	// 8 of 8 spent. A budget check here would refuse every move.
	have := counts(1, 2, 5)
	wantOK(t, capacity.CanMove(caps, have, capacity.Green, capacity.Blue)) // Blue 2 of 3, room
	wantRefusal(t, capacity.CanMove(caps, have, capacity.Green, capacity.Pink), capacity.ErrTierFull, 1, 1)
	wantRefusal(t, capacity.CanMove(caps, have, capacity.Blue, capacity.Pink), capacity.ErrTierFull, 1, 1)

	// Moving into the same tier changes nothing and refuses nothing.
	wantOK(t, capacity.CanMove(caps, have, capacity.Green, capacity.Green))

	// Even over budget (9 of 8) a legal re-file stays legal.
	over := counts(0, 3, 6)
	wantOK(t, capacity.CanMove(caps, over, capacity.Green, capacity.Pink))
	wantRefusal(t, capacity.CanMove(caps, over, capacity.Green, capacity.Blue), capacity.ErrTierFull, 3, 3)

	// The budget is the same person's add answer, proving the two questions differ.
	wantRefusal(t, capacity.CanAdd(caps, have, capacity.Blue), capacity.ErrBudgetFull, 8, 8)
}

// used may legally exceed cap (a lowered cap, a merge). Nothing may assume
// used <= cap, and an over-budget user must fail closed rather than panic.
func TestOverBudgetIsHandled(t *testing.T) {
	caps := testCaps()

	// 10 of 8 after a cap was lowered: Green holds 6 against a cap of 5.
	have := counts(1, 3, 6)
	wantRefusal(t, capacity.CanSend(caps, have), capacity.ErrBudgetFull, 10, 8)
	for _, tier := range capacity.Tiers() {
		wantRefusal(t, capacity.CanAdd(caps, have, tier), capacity.ErrBudgetFull, 10, 8)
	}
	// Re-filing into the over-full tier is refused on the tier, not the budget.
	wantRefusal(t, capacity.CanMove(caps, have, capacity.Blue, capacity.Green), capacity.ErrTierFull, 6, 5)

	// A tier over its own cap with budget to spare is still closed.
	wantRefusal(t, capacity.CanAdd(caps, counts(0, 0, 6), capacity.Green), capacity.ErrTierFull, 6, 5)

	// Zero and missing caps admit nobody rather than everybody.
	tight := capacity.Caps{Budget: 0, PerTier: map[capacity.Tier]int{}}
	wantRefusal(t, capacity.CanSend(tight, nil), capacity.ErrBudgetFull, 0, 0)
	wantRefusal(t, capacity.CanAdd(tight, nil, capacity.Pink), capacity.ErrBudgetFull, 0, 0)
	wantRefusal(t, capacity.CanAdd(capacity.Caps{Budget: 8}, nil, capacity.Pink), capacity.ErrTierFull, 0, 0)

	// A nil snapshot is an empty one, never a panic.
	wantOK(t, capacity.CanSend(caps, nil))
	wantOK(t, capacity.CanAdd(caps, nil, capacity.Green))
	wantOK(t, capacity.CanMove(caps, nil, capacity.Green, capacity.Pink))

	// A tier the rules do not know is an error, not a refusal and not a panic.
	if err := capacity.CanAdd(caps, have, capacity.Tier("GOLD")); !errors.Is(err, capacity.ErrUnknownTier) {
		t.Fatalf("want ErrUnknownTier, got %v", err)
	}
	if err := capacity.CanMove(caps, have, capacity.Green, capacity.Tier("")); !errors.Is(err, capacity.ErrUnknownTier) {
		t.Fatalf("want ErrUnknownTier, got %v", err)
	}
}

// Caps are configuration. Nothing in the rules knows the numbers 1, 3, 5 or 8.
func TestCapsComeFromConfig(t *testing.T) {
	wide := capacity.Caps{
		Budget:  1000,
		PerTier: map[capacity.Tier]int{capacity.Pink: 1, capacity.Blue: 3, capacity.Green: 500},
	}
	have := counts(1, 3, 400) // far past the README defaults, well inside these
	wantOK(t, capacity.CanSend(wide, have))
	wantOK(t, capacity.CanAdd(wide, have, capacity.Green))
	wantRefusal(t, capacity.CanAdd(wide, have, capacity.Blue), capacity.ErrTierFull, 3, 3)
}

func TestTotalCountsEveryTier(t *testing.T) {
	if got := counts(1, 2, 3).Total(); got != 6 {
		t.Fatalf("Total: want 6, got %d", got)
	}
	if got := (capacity.Counts)(nil).Total(); got != 0 {
		t.Fatalf("nil Total: want 0, got %d", got)
	}
}
