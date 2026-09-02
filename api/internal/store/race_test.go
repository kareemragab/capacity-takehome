package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/config"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// These tests need a real Mongo (make up). They open a throwaway database per
// test and drop it afterwards, so they never touch the app's data. Without a
// reachable Mongo they skip and say so; set REQUIRE_MONGO=1 to make that a
// failure instead (what CI should do).

func testCaps() capacity.Caps {
	return capacity.Caps{
		Budget:  8,
		PerTier: map[capacity.Tier]int{capacity.Pink: 1, capacity.Blue: 3, capacity.Green: 5},
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	uri := config.MongoURI()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	name := fmt.Sprintf("capacity_test_%d", time.Now().UnixNano())
	s, err := store.ConnectDB(ctx, uri, name)
	if err != nil {
		if os.Getenv("REQUIRE_MONGO") != "" {
			t.Fatalf("mongo at %s is required (REQUIRE_MONGO set): %v", uri, err)
		}
		t.Skipf("no Mongo at %s, run `make up` to prove rule 4: %v", uri, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.DB.Drop(ctx)
		_ = s.Client.Disconnect(ctx)
	})
	return s
}

func addUser(t *testing.T, s *store.Store, name string) bson.ObjectID {
	t.Helper()
	u := store.User{ID: bson.NewObjectID(), Name: name}
	if _, err := s.Users.InsertOne(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// befriend writes both sides of a pair straight into the collection, as
// fixture data. It bypasses the rules on purpose: that is how an over-budget
// user comes to exist.
func befriend(t *testing.T, s *store.Store, a, b bson.ObjectID, tier capacity.Tier) {
	t.Helper()
	now := time.Now().UTC()
	_, err := s.Contacts.InsertMany(context.Background(), []any{
		store.Contact{ID: bson.NewObjectID(), OwnerID: a, OtherID: b, Tier: tier, CreatedAt: now},
		store.Contact{ID: bson.NewObjectID(), OwnerID: b, OtherID: a, Tier: tier, CreatedAt: now},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// fill gives user seven contacts: Pink and Blue full, Green at 3 of 5, so the
// budget has exactly one seat left and Green has room for it.
func fill(t *testing.T, s *store.Store, user bson.ObjectID, label string) {
	t.Helper()
	tiers := []capacity.Tier{capacity.Pink, capacity.Blue, capacity.Blue, capacity.Blue, capacity.Green, capacity.Green, capacity.Green}
	for i, tier := range tiers {
		befriend(t, s, user, addUser(t, s, fmt.Sprintf("%s-filler-%d", label, i)), tier)
	}
}

func contactCount(t *testing.T, s *store.Store, owner bson.ObjectID) int {
	t.Helper()
	n, err := s.Contacts.CountDocuments(context.Background(), bson.M{"ownerId": owner})
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func pendingTo(t *testing.T, s *store.Store, to bson.ObjectID) int {
	t.Helper()
	n, err := s.Requests.CountDocuments(context.Background(), bson.M{"toId": to, "status": store.RequestPending})
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// Rule 4 - two accepts landing at the same moment on a user with exactly one
// free seat must not both succeed. Exactly one wins; the other fails cleanly.
//
// This one needs a real Mongo (make up), which is why it lives here and not in
// the capacity package. Read-then-write will pass a serial test and fail this
// one - that is the point of it.
func TestConcurrentAcceptsTakeOneSeat(t *testing.T) {
	s := testStore(t)
	caps := testCaps()
	ctx := context.Background()

	// Six people, not two: the same seat contested harder, several rounds.
	const contenders = 6
	for round := 1; round <= 3; round++ {
		target := addUser(t, s, fmt.Sprintf("target-%d", round))
		fill(t, s, target, fmt.Sprintf("r%d", round))
		if got := contactCount(t, s, target); got != 7 {
			t.Fatalf("fixture: want 7 contacts, got %d", got)
		}

		requests := make([]bson.ObjectID, contenders)
		senders := make([]bson.ObjectID, contenders)
		for i := range requests {
			senders[i] = addUser(t, s, fmt.Sprintf("r%d-sender-%d", round, i))
			req, err := s.SendRequest(ctx, caps, senders[i], target, capacity.Green)
			if err != nil {
				t.Fatalf("send %d: %v", i, err)
			}
			requests[i] = req.ID
		}

		// Release every accept at the same instant.
		results := make([]error, contenders)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range requests {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, results[i] = s.AcceptRequest(ctx, caps, target, requests[i])
			}(i)
		}
		close(start)
		wg.Wait()

		wins := 0
		for i, err := range results {
			if err == nil {
				wins++
				continue
			}
			// The loser gets the real reason, not a conflict or a retry error.
			if !errors.Is(err, capacity.ErrBudgetFull) {
				t.Fatalf("round %d accept %d: loser must be refused on the budget, got %v", round, i, err)
			}
			var sr *store.SeatRefusal
			if !errors.As(err, &sr) || sr.UserID != target {
				t.Fatalf("round %d accept %d: refusal must name the target, got %v", round, i, err)
			}
		}
		if wins != 1 {
			t.Fatalf("round %d: want exactly one winner, got %d (%v)", round, wins, results)
		}
		if got := contactCount(t, s, target); got != 8 {
			t.Fatalf("round %d: target holds %d contacts, want 8 and never 9", round, got)
		}
		// The losers' requests are still pending, so they can be retried once a
		// seat frees; nothing was half-written on the senders' sides.
		if got := pendingTo(t, s, target); got != contenders-1 {
			t.Fatalf("round %d: want %d requests still pending, got %d", round, contenders-1, got)
		}
		for i, sender := range senders {
			want := 0
			if results[i] == nil {
				want = 1
			}
			if got := contactCount(t, s, sender); got != want {
				t.Fatalf("round %d: sender %d holds %d contacts, want %d", round, i, got, want)
			}
		}
	}
}

// Rule 2 - capacity is checked at accept, against both people. Here the sender
// fills up after sending, so the receiver's accept fails on the sender's side.
func TestAcceptChecksBothSides(t *testing.T) {
	s := testStore(t)
	caps := testCaps()
	ctx := context.Background()

	sender := addUser(t, s, "sender")
	receiver := addUser(t, s, "receiver")
	fill(t, s, sender, "s") // 7 of 8: one seat left, sending is allowed

	req, err := s.SendRequest(ctx, caps, sender, receiver, capacity.Green)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	// The sender's last seat goes elsewhere in the meantime.
	befriend(t, s, sender, addUser(t, s, "someone-else"), capacity.Green)

	_, err = s.AcceptRequest(ctx, caps, receiver, req.ID)
	if !errors.Is(err, capacity.ErrBudgetFull) {
		t.Fatalf("want the sender's budget refusal, got %v", err)
	}
	var sr *store.SeatRefusal
	if !errors.As(err, &sr) || sr.UserID != sender {
		t.Fatalf("refusal must name the sender, got %v", err)
	}
	if got := contactCount(t, s, receiver); got != 0 {
		t.Fatalf("receiver must have nothing, got %d", got)
	}
	if got := pendingTo(t, s, receiver); got != 1 {
		t.Fatalf("request must stay pending, got %d pending", got)
	}
}

// Rule 2 - a pending request holds no seat: one free seat buys any number of
// outstanding requests, and declining touches nothing.
func TestPendingRequestsHoldNoSeat(t *testing.T) {
	s := testStore(t)
	caps := testCaps()
	ctx := context.Background()

	me := addUser(t, s, "me")
	fill(t, s, me, "me") // 7 of 8
	for i := 0; i < 5; i++ {
		if _, err := s.SendRequest(ctx, caps, me, addUser(t, s, fmt.Sprintf("p%d", i)), capacity.Pink); err != nil {
			t.Fatalf("send %d with one free seat: %v", i, err)
		}
	}
	have, err := s.CountsFor(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if have.Total() != 7 {
		t.Fatalf("sending spent seats: %v", have)
	}

	// Full budget refuses sending at all.
	befriend(t, s, me, addUser(t, s, "eighth"), capacity.Green)
	if _, err := s.SendRequest(ctx, caps, me, addUser(t, s, "late"), capacity.Green); !errors.Is(err, capacity.ErrBudgetFull) {
		t.Fatalf("want budget refusal at 8 of 8, got %v", err)
	}

	// Declining an incoming request costs nothing either.
	other := addUser(t, s, "other")
	req, err := s.SendRequest(ctx, caps, other, me, capacity.Blue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeclineRequest(ctx, me, req.ID); err != nil {
		t.Fatalf("decline: %v", err)
	}
	if _, err := s.DeclineRequest(ctx, me, req.ID); !errors.Is(err, store.ErrRequestClosed) {
		t.Fatalf("second decline: want ErrRequestClosed, got %v", err)
	}
}

// Rule 3 - re-filing is not adding. An over-budget user can still move a
// contact into a tier with room, and is refused only by the destination cap.
func TestMoveIsNotAdd(t *testing.T) {
	s := testStore(t)
	caps := testCaps()
	ctx := context.Background()

	me := addUser(t, s, "me")
	// 1 Pink, 2 Blue, 6 Green: 9 of 8 (Green cap was lowered, say).
	tiers := []capacity.Tier{capacity.Pink, capacity.Blue, capacity.Blue,
		capacity.Green, capacity.Green, capacity.Green, capacity.Green, capacity.Green, capacity.Green}
	var greenContact bson.ObjectID
	for i, tier := range tiers {
		other := addUser(t, s, fmt.Sprintf("c%d", i))
		befriend(t, s, me, other, tier)
		if tier == capacity.Green {
			var c store.Contact
			if err := s.Contacts.FindOne(ctx, bson.M{"ownerId": me, "otherId": other}).Decode(&c); err != nil {
				t.Fatal(err)
			}
			greenContact = c.ID
		}
	}

	// Adding is refused on the budget, and so is sending.
	if _, err := s.SendRequest(ctx, caps, me, addUser(t, s, "x"), capacity.Blue); !errors.Is(err, capacity.ErrBudgetFull) {
		t.Fatalf("want budget refusal, got %v", err)
	}
	// Green -> Blue has room (2 of 3) and must not be blocked by the budget.
	moved, err := s.MoveContact(ctx, caps, me, greenContact, capacity.Blue)
	if err != nil {
		t.Fatalf("move Green->Blue over budget: %v", err)
	}
	if moved.Tier != capacity.Blue {
		t.Fatalf("want Blue, got %s", moved.Tier)
	}
	// Blue -> Pink is refused by Pink's cap (1 of 1), never by the budget.
	_, err = s.MoveContact(ctx, caps, me, greenContact, capacity.Pink)
	if !errors.Is(err, capacity.ErrTierFull) {
		t.Fatalf("want Pink tier refusal, got %v", err)
	}
	if errors.Is(err, capacity.ErrBudgetFull) {
		t.Fatalf("a move must never be refused on the budget: %v", err)
	}
	// The other side's tier is their business and untouched by my move.
	var theirs store.Contact
	if err := s.Contacts.FindOne(ctx, bson.M{"otherId": me, "ownerId": moved.OtherID}).Decode(&theirs); err != nil {
		t.Fatal(err)
	}
	if theirs.Tier != capacity.Green {
		t.Fatalf("other side changed tier to %s", theirs.Tier)
	}
}

// R2 and R4 - accept creates the contact on both sides, remove frees both.
func TestAcceptAndRemoveAreSymmetric(t *testing.T) {
	s := testStore(t)
	caps := testCaps()
	ctx := context.Background()

	a := addUser(t, s, "a")
	b := addUser(t, s, "b")
	req, err := s.SendRequest(ctx, caps, a, b, capacity.Blue)
	if err != nil {
		t.Fatal(err)
	}
	// The reverse direction is refused while this one is pending, and so is a
	// duplicate of the same direction.
	if _, err := s.SendRequest(ctx, caps, b, a, capacity.Blue); !errors.Is(err, store.ErrReverseRequestExists) {
		t.Fatalf("reverse: want ErrReverseRequestExists, got %v", err)
	}
	if _, err := s.SendRequest(ctx, caps, a, b, capacity.Green); !errors.Is(err, store.ErrRequestExists) {
		t.Fatalf("duplicate: want ErrRequestExists, got %v", err)
	}
	// Only the addressee can accept.
	if _, err := s.AcceptRequest(ctx, caps, a, req.ID); !errors.Is(err, store.ErrNotYours) {
		t.Fatalf("sender accepting: want ErrNotYours, got %v", err)
	}

	mine, err := s.AcceptRequest(ctx, caps, b, req.ID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if mine.OwnerID != b || mine.OtherID != a || mine.Tier != capacity.Blue {
		t.Fatalf("unexpected contact %+v", mine)
	}
	if contactCount(t, s, a) != 1 || contactCount(t, s, b) != 1 {
		t.Fatalf("accept must create both sides")
	}
	if _, err := s.SendRequest(ctx, caps, a, b, capacity.Green); !errors.Is(err, store.ErrAlreadyContacts) {
		t.Fatalf("want ErrAlreadyContacts, got %v", err)
	}
	// Accepting again is a closed request, not a second contact.
	if _, err := s.AcceptRequest(ctx, caps, b, req.ID); !errors.Is(err, store.ErrRequestClosed) {
		t.Fatalf("second accept: want ErrRequestClosed, got %v", err)
	}

	if err := s.RemoveContact(ctx, a, mine.ID); !errors.Is(err, store.ErrNotYours) {
		t.Fatalf("removing someone else's contact document: want ErrNotYours, got %v", err)
	}
	if err := s.RemoveContact(ctx, b, mine.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if contactCount(t, s, a) != 0 || contactCount(t, s, b) != 0 {
		t.Fatalf("remove must free both sides")
	}
}
