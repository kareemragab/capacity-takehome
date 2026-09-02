package graph

// Presentation: store documents become GraphQL models, and every refusal
// becomes one sentence a person can act on. Nothing here decides anything.

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/tktaofik/capacity-takehome/api/graph/model"
	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
	"github.com/tktaofik/capacity-takehome/api/internal/store"
)

// op says which mutation a refusal came from, so the sentence fits the button
// the user just pressed.
type op int

const (
	opSend op = iota
	opAccept
	opDecline
	opMove
	opRemove
)

// Error codes in extensions.code, for a client that wants to branch (an
// optimistic accept rolling back on CAPACITY_FULL, say). The message is still
// the thing to show.
const (
	codeCapacityFull    = "CAPACITY_FULL"
	codeNotFound        = "NOT_FOUND"
	codeForbidden       = "FORBIDDEN"
	codeAlreadyDecided  = "ALREADY_DECIDED"
	codeBadRequest      = "BAD_REQUEST"
	codeUnauthenticated = "UNAUTHENTICATED"
	codeInternal        = "INTERNAL"
)

func userError(code, msg string, extra map[string]any) error {
	ext := map[string]any{"code": code}
	for k, v := range extra {
		ext[k] = v
	}
	return &gqlerror.Error{Message: msg, Extensions: ext}
}

// explain turns any error out of the store into a sentence. caller is the
// person pressing the button; names are looked up so "Ada is full" reads as
// such, and "you are full" is said to the caller directly.
func (r *Resolver) explain(ctx context.Context, err error, caller bson.ObjectID, what op) error {
	var sr *store.SeatRefusal
	if errors.As(err, &sr) {
		var ref *capacity.Refusal
		if errors.As(sr.Err, &ref) {
			return r.capacitySentence(ctx, sr.UserID, caller, ref, what)
		}
	}
	switch {
	case errors.Is(err, store.ErrNoUser):
		return userError(codeUnauthenticated, "Say who you are first: send an X-User-Id header.", nil)
	case errors.Is(err, store.ErrNotFound):
		switch what {
		case opSend:
			return userError(codeNotFound, "That person doesn't exist.", nil)
		case opAccept, opDecline:
			return userError(codeNotFound, "That request doesn't exist anymore.", nil)
		default:
			return userError(codeNotFound, "That contact doesn't exist anymore.", nil)
		}
	case errors.Is(err, store.ErrNotYours):
		if what == opAccept || what == opDecline {
			return userError(codeForbidden, "That request isn't addressed to you.", nil)
		}
		return userError(codeForbidden, "That contact isn't yours.", nil)
	case errors.Is(err, store.ErrRequestClosed):
		return userError(codeAlreadyDecided, "That request was already accepted or declined.", nil)
	case errors.Is(err, store.ErrSelfRequest):
		return userError(codeBadRequest, "You can't send a request to yourself.", nil)
	case errors.Is(err, store.ErrAlreadyContacts):
		return userError(codeBadRequest, "You're already contacts.", nil)
	case errors.Is(err, store.ErrRequestExists):
		return userError(codeBadRequest, "You already have a pending request to that person.", nil)
	case errors.Is(err, store.ErrReverseRequestExists):
		return userError(codeBadRequest, "That person already sent you a request. Accept it from your inbox instead.", nil)
	case errors.Is(err, store.ErrSameTier):
		return userError(codeBadRequest, "That contact is already in that tier.", nil)
	case errors.Is(err, capacity.ErrUnknownTier):
		return userError(codeBadRequest, "That tier doesn't exist.", nil)
	}
	log.Printf("internal: %v", err)
	return userError(codeInternal, "Something went wrong on the server. Please try again.", map[string]any{"detail": err.Error()})
}

func (r *Resolver) capacitySentence(ctx context.Context, full, caller bson.ObjectID, ref *capacity.Refusal, what op) error {
	mine := full == caller
	name := "They"
	if !mine {
		if u, err := r.Store.UserByID(ctx, full); err == nil {
			name = u.Name
		}
	}
	tier := tierWord(ref.Tier)
	budget := ref.Reason == capacity.ErrBudgetFull

	var msg string
	switch {
	case what == opSend:
		msg = fmt.Sprintf("You can't send requests right now: your contact list is full (%d of %d seats). Free a seat first.", ref.Used, ref.Cap)
	case what == opMove:
		msg = fmt.Sprintf("%s is full (%d of %d). Move someone out of %s first.", tier, ref.Used, ref.Cap, tier)
	case mine && budget:
		msg = fmt.Sprintf("You can't accept this right now: your contact list is full (%d of %d seats). Remove a contact to free a seat, then try again.", ref.Used, ref.Cap)
	case mine:
		msg = fmt.Sprintf("You can't accept this into %s right now: your %s is full (%d of %d). Move someone out of %s first.", tier, tier, ref.Used, ref.Cap, tier)
	case budget:
		msg = fmt.Sprintf("%s can't take this right now: %s's contact list is full (%d of %d seats). The request stays pending, so you can try again later.", name, name, ref.Used, ref.Cap)
	default:
		msg = fmt.Sprintf("%s can't file you under %s right now: %s's %s is full (%d of %d). The request stays pending.", name, tier, name, tier, ref.Used, ref.Cap)
	}

	side := "them"
	if mine {
		side = "you"
	}
	reason := "TIER"
	if budget {
		reason = "BUDGET"
	}
	ext := map[string]any{"side": side, "reason": reason, "used": ref.Used, "cap": ref.Cap}
	if ref.Tier != "" {
		ext["tier"] = string(ref.Tier)
	}
	return userError(codeCapacityFull, msg, ext)
}

// tierWord is how a tier reads in a sentence: "Pink", not "PINK".
func tierWord(t capacity.Tier) string {
	switch t {
	case capacity.Pink:
		return "Pink"
	case capacity.Blue:
		return "Blue"
	case capacity.Green:
		return "Green"
	}
	return string(t)
}

func parseID(raw, what string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		return bson.ObjectID{}, userError(codeBadRequest, fmt.Sprintf("That %s id isn't valid.", what), nil)
	}
	return id, nil
}

// caller reads the acting user off the context, as a sentence when missing.
func (r *Resolver) caller(ctx context.Context) (bson.ObjectID, error) {
	id, err := store.CallerID(ctx)
	if err != nil {
		return bson.ObjectID{}, r.explain(ctx, err, bson.ObjectID{}, opSend)
	}
	return id, nil
}

// people loads the users behind a set of ids in one query and hands back a
// lookup that always returns something renderable.
func (r *Resolver) people(ctx context.Context, ids ...bson.ObjectID) (func(bson.ObjectID) *model.User, error) {
	users, err := r.Store.UsersByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	return func(id bson.ObjectID) *model.User {
		if u, ok := users[id]; ok {
			return &model.User{ID: u.ID.Hex(), Name: u.Name}
		}
		return &model.User{ID: id.Hex(), Name: "Unknown"}
	}, nil
}

func toContact(c store.Contact, user func(bson.ObjectID) *model.User) model.Contact {
	return model.Contact{ID: c.ID.Hex(), User: user(c.OtherID), Tier: model.Tier(c.Tier), CreatedAt: c.CreatedAt}
}

func toRequest(q store.Request, user func(bson.ObjectID) *model.User) model.Request {
	return model.Request{
		ID: q.ID.Hex(), From: user(q.FromID), To: user(q.ToID),
		Tier: model.Tier(q.Tier), Status: model.RequestStatus(q.Status), CreatedAt: q.CreatedAt,
	}
}

// requests resolves the people on a list of requests with one user query.
func (r *Resolver) requests(ctx context.Context, docs []store.Request) ([]model.Request, error) {
	ids := make([]bson.ObjectID, 0, 2*len(docs))
	for _, q := range docs {
		ids = append(ids, q.FromID, q.ToID)
	}
	user, err := r.people(ctx, ids...)
	if err != nil {
		return nil, fmt.Errorf("requests: %w", err)
	}
	out := make([]model.Request, 0, len(docs))
	for _, q := range docs {
		out = append(out, toRequest(q, user))
	}
	return out, nil
}
