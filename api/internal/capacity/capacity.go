// Package capacity holds the tier rules.
//
// It is pure on purpose: no database, no context, no clock, no IO. Everything
// a decision needs is passed in, which is what makes the rules cheap to test
// and impossible to accidentally scatter across resolvers.
//
// The four rules these functions must satisfy are in the README. Read them there,
// not here.
package capacity

import (
	"errors"
	"fmt"
)

type Tier string

const (
	Pink  Tier = "PINK"
	Blue  Tier = "BLUE"
	Green Tier = "GREEN"
)

// Tiers lists every tier, closest first.
func Tiers() []Tier { return []Tier{Pink, Blue, Green} }

// Valid reports whether t is one of the tiers this package knows about.
func (t Tier) Valid() bool {
	switch t {
	case Pink, Blue, Green:
		return true
	}
	return false
}

// Caps is configuration, loaded at startup. Raising a cap must never require
// a code change in the enforcement path.
type Caps struct {
	Budget  int
	PerTier map[Tier]int
}

// Counts is a snapshot of one user's active contacts, keyed by tier.
type Counts map[Tier]int

// Total is the number of seats currently spent across every tier.
func (c Counts) Total() int {
	n := 0
	for _, t := range Tiers() {
		n += c[t]
	}
	return n
}

var (
	// ErrBudgetFull means the shared budget is spent, regardless of sub-caps.
	ErrBudgetFull = errors.New("capacity: shared budget is full")
	// ErrTierFull means the destination tier is full, even though the budget has room.
	ErrTierFull = errors.New("capacity: tier is full")
	// ErrUnknownTier means the caller named a tier this package does not know.
	// It is a programming or input error, never a capacity refusal.
	ErrUnknownTier = errors.New("capacity: unknown tier")
)

// Refusal is what a rule returns when the answer is no. It carries the numbers
// behind the decision so the caller can turn it into a sentence for a human
// ("Blue is full, 3 of 3"). errors.Is(err, ErrBudgetFull) and
// errors.Is(err, ErrTierFull) keep working on it.
type Refusal struct {
	// Reason is ErrBudgetFull or ErrTierFull.
	Reason error
	// Tier is set when Reason is ErrTierFull.
	Tier Tier
	// Used and Cap are the numbers that made the decision. Used may exceed Cap.
	Used, Cap int
}

func (r *Refusal) Error() string {
	if r.Reason == ErrTierFull {
		return fmt.Sprintf("%s is full (%d of %d)", r.Tier, r.Used, r.Cap)
	}
	return fmt.Sprintf("shared budget is full (%d of %d)", r.Used, r.Cap)
}

// Is lets errors.Is match a Refusal against its sentinel reason.
func (r *Refusal) Is(target error) bool { return target == r.Reason }

// CanSend reports whether a user holding these counts may send a new request.
// A pending request creates no contact and spends no seat, so this is a budget
// question only: one free seat permits any number of outstanding requests, and
// a full tier does not block sending to it (the accept will decide).
func CanSend(caps Caps, have Counts) error {
	return budgetHasRoom(caps, have)
}

// CanAdd reports whether a new contact may be added to tier t.
// Called for both sides of an accept. The shared budget is checked first, so a
// spent budget refuses even an empty tier; the tier sub-cap is checked second.
func CanAdd(caps Caps, have Counts, t Tier) error {
	if !t.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownTier, t)
	}
	if err := budgetHasRoom(caps, have); err != nil {
		return err
	}
	return tierHasRoom(caps, have, t)
}

// CanMove reports whether an existing contact may be re-filed from one tier
// to another. The contact already occupies a seat inside the budget, so only
// the destination sub-cap is consulted, never the budget; a budget check here
// would block a legal move for an over-budget user.
func CanMove(caps Caps, have Counts, from, to Tier) error {
	if !from.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownTier, from)
	}
	if !to.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownTier, to)
	}
	if from == to {
		// Nothing changes hands, so there is nothing to refuse.
		return nil
	}
	return tierHasRoom(caps, have, to)
}

// budgetHasRoom fails closed: any total at or above the budget refuses, which
// covers the legal case of used exceeding cap after a lowered cap or a merge.
func budgetHasRoom(caps Caps, have Counts) error {
	if used := have.Total(); used >= caps.Budget {
		return &Refusal{Reason: ErrBudgetFull, Used: used, Cap: caps.Budget}
	}
	return nil
}

// tierHasRoom fails closed the same way. A tier missing from caps.PerTier
// reads as cap 0, so an unconfigured tier admits nobody rather than everybody.
func tierHasRoom(caps Caps, have Counts, t Tier) error {
	if used, cap := have[t], caps.PerTier[t]; used >= cap {
		return &Refusal{Reason: ErrTierFull, Tier: t, Used: used, Cap: cap}
	}
	return nil
}
