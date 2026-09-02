package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

// Every operation in this file that can change who holds a seat runs inside
// one Mongo transaction and starts by touching the user documents involved
// (touchSeats). That touch is the whole concurrency story:
//
//   - Two transactions that write the same document conflict in Mongo. The
//     second one to reach the user document is aborted with a transient
//     error and WithTransaction re-runs it from the top.
//   - On the re-run its snapshot includes the first transaction's commit, so
//     the counts it reads are current and the capacity package refuses it
//     cleanly, with the real reason ("full, 8 of 8"), not a conflict error.
//
// So the rule is still decided in exactly one place, capacity.CanAdd, and it
// is decided against counts that cannot be stale. Nothing here re-implements
// the rule in a query filter, and there is no read-then-write window.

// withTx runs fn inside a transaction with snapshot reads and majority writes.
func (s *Store) withTx(ctx context.Context, fn func(ctx context.Context) error) error {
	sess, err := s.Client.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)
	opts := options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority())
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	}, opts)
	return err
}

// touchSeats bumps seatVersion on each user inside the current transaction.
// It also proves the users exist: a missing one is ErrNotFound.
func (s *Store) touchSeats(ctx context.Context, ids ...bson.ObjectID) error {
	for _, id := range ids {
		res, err := s.Users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"seatVersion": 1}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("user %s: %w", id.Hex(), ErrNotFound)
		}
	}
	return nil
}

// SendRequest files a pending request. It spends no seat (rule 2), so the only
// capacity question is whether the sender has any room at all; the receiver is
// not consulted until accept.
func (s *Store) SendRequest(ctx context.Context, caps capacity.Caps, from, to bson.ObjectID, tier capacity.Tier) (*Request, error) {
	if from == to {
		return nil, ErrSelfRequest
	}
	if _, err := s.UserByID(ctx, to); err != nil {
		return nil, err
	}
	have, err := s.CountsFor(ctx, from)
	if err != nil {
		return nil, err
	}
	if err := capacity.CanSend(caps, have); err != nil {
		return nil, &SeatRefusal{UserID: from, Err: err}
	}
	if n, err := s.Contacts.CountDocuments(ctx, bson.M{"ownerId": from, "otherId": to}); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, ErrAlreadyContacts
	}
	if n, err := s.Requests.CountDocuments(ctx, bson.M{"fromId": to, "toId": from, "status": RequestPending}); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, ErrReverseRequestExists
	}
	req := Request{
		ID:        bson.NewObjectID(),
		FromID:    from,
		ToID:      to,
		Tier:      tier,
		Status:    RequestPending,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.Requests.InsertOne(ctx, req); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			// The partial unique index: one pending request per direction.
			return nil, ErrRequestExists
		}
		return nil, err
	}
	return &req, nil
}

// AcceptRequest turns a pending request into a contact on both sides, in the
// request's tier for both people. Capacity is checked for both, inside the
// transaction, against counts the transaction is guaranteed to have current.
// A refusal leaves the request pending so it can be retried once a seat frees.
func (s *Store) AcceptRequest(ctx context.Context, caps capacity.Caps, caller, requestID bson.ObjectID) (*Contact, error) {
	var mine Contact
	err := s.withTx(ctx, func(ctx context.Context) error {
		req, err := s.pendingRequest(ctx, requestID)
		if err != nil {
			return err
		}
		if req.ToID != caller {
			return ErrNotYours
		}
		// Serialize against every other seat change on either person.
		if err := s.touchSeats(ctx, req.ToID, req.FromID); err != nil {
			return err
		}
		for _, side := range []bson.ObjectID{req.ToID, req.FromID} {
			have, err := s.CountsFor(ctx, side)
			if err != nil {
				return err
			}
			if err := capacity.CanAdd(caps, have, req.Tier); err != nil {
				return &SeatRefusal{UserID: side, Err: err}
			}
		}
		now := time.Now().UTC()
		mine = Contact{ID: bson.NewObjectID(), OwnerID: req.ToID, OtherID: req.FromID, Tier: req.Tier, CreatedAt: now}
		theirs := Contact{ID: bson.NewObjectID(), OwnerID: req.FromID, OtherID: req.ToID, Tier: req.Tier, CreatedAt: now}
		if _, err := s.Contacts.InsertMany(ctx, []any{mine, theirs}); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return ErrAlreadyContacts
			}
			return err
		}
		return s.decide(ctx, req.ID, RequestAccepted, now)
	})
	if err != nil {
		return nil, err
	}
	return &mine, nil
}

// DeclineRequest closes a pending request without touching any seat.
func (s *Store) DeclineRequest(ctx context.Context, caller, requestID bson.ObjectID) (*Request, error) {
	req, err := s.pendingRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if req.ToID != caller {
		return nil, ErrNotYours
	}
	now := time.Now().UTC()
	if err := s.decide(ctx, req.ID, RequestDeclined, now); err != nil {
		return nil, err
	}
	req.Status = RequestDeclined
	req.DecidedAt = &now
	return req, nil
}

// MoveContact re-files one of the caller's contacts. The contact already holds
// a seat, so only the destination sub-cap is asked (rule 3). The other person's
// side is untouched: tiers are private to each owner.
func (s *Store) MoveContact(ctx context.Context, caps capacity.Caps, caller, contactID bson.ObjectID, to capacity.Tier) (*Contact, error) {
	var out Contact
	err := s.withTx(ctx, func(ctx context.Context) error {
		c, err := s.contact(ctx, contactID)
		if err != nil {
			return err
		}
		if c.OwnerID != caller {
			return ErrNotYours
		}
		if c.Tier == to {
			return ErrSameTier
		}
		if err := s.touchSeats(ctx, caller); err != nil {
			return err
		}
		have, err := s.CountsFor(ctx, caller)
		if err != nil {
			return err
		}
		if err := capacity.CanMove(caps, have, c.Tier, to); err != nil {
			return &SeatRefusal{UserID: caller, Err: err}
		}
		if _, err := s.Contacts.UpdateOne(ctx, bson.M{"_id": c.ID}, bson.M{"$set": bson.M{"tier": to}}); err != nil {
			return err
		}
		c.Tier = to
		out = *c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveContact deletes both sides of the pair, freeing a seat for each.
func (s *Store) RemoveContact(ctx context.Context, caller, contactID bson.ObjectID) error {
	return s.withTx(ctx, func(ctx context.Context) error {
		c, err := s.contact(ctx, contactID)
		if err != nil {
			return err
		}
		if c.OwnerID != caller {
			return ErrNotYours
		}
		if err := s.touchSeats(ctx, c.OwnerID, c.OtherID); err != nil {
			return err
		}
		_, err = s.Contacts.DeleteMany(ctx, bson.M{"$or": []bson.M{
			{"ownerId": c.OwnerID, "otherId": c.OtherID},
			{"ownerId": c.OtherID, "otherId": c.OwnerID},
		}})
		return err
	})
}

func (s *Store) pendingRequest(ctx context.Context, id bson.ObjectID) (*Request, error) {
	var req Request
	err := s.Requests.FindOne(ctx, bson.M{"_id": id}).Decode(&req)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if req.Status != RequestPending {
		return nil, ErrRequestClosed
	}
	return &req, nil
}

func (s *Store) contact(ctx context.Context, id bson.ObjectID) (*Contact, error) {
	var c Contact
	err := s.Contacts.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// decide closes a request. The status filter makes it a no-op if someone else
// decided first, and that shows up as ErrRequestClosed.
func (s *Store) decide(ctx context.Context, id bson.ObjectID, status RequestStatus, at time.Time) error {
	res, err := s.Requests.UpdateOne(ctx,
		bson.M{"_id": id, "status": RequestPending},
		bson.M{"$set": bson.M{"status": status, "decidedAt": at}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrRequestClosed
	}
	return nil
}
