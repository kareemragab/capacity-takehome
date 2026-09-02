// Package store is the data layer: Mongo connection, collection handles and
// the document shapes. Business rules do not live here; the seat-changing
// operations in seats.go ask the capacity package for every decision and only
// provide the atomicity around it.
package store

import (
	"context"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/tktaofik/capacity-takehome/api/internal/capacity"
)

type User struct {
	ID   bson.ObjectID `bson:"_id,omitempty"`
	Name string        `bson:"name"`
	// SeatVersion is bumped inside every transaction that changes this user's
	// contacts. It carries no meaning of its own: touching it is what makes two
	// concurrent seat changes on the same person collide in Mongo, so exactly
	// one commits. See touchSeats in seats.go.
	SeatVersion int64 `bson:"seatVersion"`
}

// Contact is one side of a pair. Adding a contact writes two of these, one for
// each user; removing it must free both.
type Contact struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	OwnerID   bson.ObjectID `bson:"ownerId"`
	OtherID   bson.ObjectID `bson:"otherId"`
	Tier      capacity.Tier `bson:"tier"`
	CreatedAt time.Time     `bson:"createdAt"`
}

type RequestStatus string

const (
	RequestPending  RequestStatus = "PENDING"
	RequestAccepted RequestStatus = "ACCEPTED"
	RequestDeclined RequestStatus = "DECLINED"
)

type Request struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	FromID    bson.ObjectID `bson:"fromId"`
	ToID      bson.ObjectID `bson:"toId"`
	Tier      capacity.Tier `bson:"tier"`
	Status    RequestStatus `bson:"status"`
	CreatedAt time.Time     `bson:"createdAt"`
	DecidedAt *time.Time    `bson:"decidedAt,omitempty"`
}

type Store struct {
	Client   *mongo.Client
	DB       *mongo.Database
	Users    *mongo.Collection
	Contacts *mongo.Collection
	Requests *mongo.Collection
}

// Connect opens the app database.
func Connect(ctx context.Context, uri string) (*Store, error) {
	return ConnectDB(ctx, uri, "capacity")
}

// ConnectDB opens a named database; tests use a throwaway name so they never
// touch the data behind the running app.
func ConnectDB(ctx context.Context, uri, name string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(name)
	s := &Store{
		Client:   client,
		DB:       db,
		Users:    db.Collection("users"),
		Contacts: db.Collection("contacts"),
		Requests: db.Collection("requests"),
	}
	return s, s.ensureIndexes(ctx)
}

// ensureIndexes declares what the app cannot be correct without.
func (s *Store) ensureIndexes(ctx context.Context) error {
	// A pair may exist only once per owner.
	_, err := s.Contacts.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "ownerId", Value: 1}, {Key: "otherId", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("owner_other_unique"),
	})
	if err != nil {
		return err
	}
	// CountsFor groups by tier under one owner; this keeps it an index read.
	_, err = s.Contacts.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "ownerId", Value: 1}, {Key: "tier", Value: 1}},
		Options: options.Index().SetName("owner_tier"),
	})
	if err != nil {
		return err
	}
	// One pending request per direction. Enforced by Mongo, not by a lookup,
	// so two double-taps on "send" cannot both get through.
	_, err = s.Requests.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "fromId", Value: 1}, {Key: "toId", Value: 1}},
		Options: options.Index().
			SetUnique(true).
			SetName("pending_from_to_unique").
			SetPartialFilterExpression(bson.M{"status": string(RequestPending)}),
	})
	if err != nil {
		return err
	}
	// Inbox and outbox reads.
	_, err = s.Requests.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "toId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}, Options: options.Index().SetName("inbox")},
		{Keys: bson.D{{Key: "fromId", Value: 1}, {Key: "createdAt", Value: -1}}, Options: options.Index().SetName("outbox")},
	})
	return err
}

// Seed inserts a few people to talk to, so the client has something to show on
// first run. It is idempotent.
func (s *Store) Seed(ctx context.Context) error {
	names := []string{"You", "Ada", "Grace", "Alan", "Katherine", "Barbara", "Edsger", "Radia", "Ken", "Margaret"}
	for _, n := range names {
		filter := bson.M{"name": n}
		update := bson.M{"$setOnInsert": bson.M{"name": n, "seatVersion": int64(0)}}
		if _, err := s.Users.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
	}
	return nil
}

// CountsFor returns the caller's active contacts per tier.
func (s *Store) CountsFor(ctx context.Context, ownerID bson.ObjectID) (capacity.Counts, error) {
	counts := capacity.Counts{}
	cur, err := s.Contacts.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"ownerId": ownerID}}},
		{{Key: "$group", Value: bson.M{"_id": "$tier", "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var rows []struct {
		Tier capacity.Tier `bson:"_id"`
		N    int           `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.Tier] = r.N
	}
	return counts, nil
}

// ContactsFor lists the owner's contacts, closest tier first, oldest first
// inside a tier.
func (s *Store) ContactsFor(ctx context.Context, ownerID bson.ObjectID) ([]Contact, error) {
	cur, err := s.Contacts.Find(ctx, bson.M{"ownerId": ownerID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []Contact
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	rank := map[capacity.Tier]int{}
	for i, t := range capacity.Tiers() {
		rank[t] = i
	}
	// Stable sort keeps the createdAt order inside each tier.
	slices.SortStableFunc(docs, func(a, b Contact) int { return rank[a.Tier] - rank[b.Tier] })
	return docs, nil
}

// PendingRequestsTo is the inbox: what the user still has to answer.
func (s *Store) PendingRequestsTo(ctx context.Context, userID bson.ObjectID) ([]Request, error) {
	return s.findRequests(ctx, bson.M{"toId": userID, "status": RequestPending})
}

// RequestsFrom is the outbox: everything the user sent, newest first, with its
// current status so a decline is visible to the sender.
func (s *Store) RequestsFrom(ctx context.Context, userID bson.ObjectID) ([]Request, error) {
	return s.findRequests(ctx, bson.M{"fromId": userID})
}

func (s *Store) findRequests(ctx context.Context, filter bson.M) ([]Request, error) {
	cur, err := s.Requests.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []Request
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// UsersByID loads a set of users in one query, for resolving the people on
// contacts and requests without a lookup per row.
func (s *Store) UsersByID(ctx context.Context, ids []bson.ObjectID) (map[bson.ObjectID]User, error) {
	out := map[bson.ObjectID]User{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.Users.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []User
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	for _, u := range docs {
		out[u.ID] = u
	}
	return out, nil
}

// UserByID loads one user. ErrNotFound when there is no such person.
func (s *Store) UserByID(ctx context.Context, id bson.ObjectID) (User, error) {
	var u User
	err := s.Users.FindOne(ctx, bson.M{"_id": id}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	return u, err
}
