package store

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// These are the ways an operation can be refused for a reason other than
// capacity. The resolver layer turns each one into a sentence.
var (
	ErrNotFound             = errors.New("store: not found")
	ErrNotYours             = errors.New("store: not yours")
	ErrRequestClosed        = errors.New("store: request already decided")
	ErrSelfRequest          = errors.New("store: cannot send a request to yourself")
	ErrAlreadyContacts      = errors.New("store: already contacts")
	ErrRequestExists        = errors.New("store: a pending request already exists")
	ErrReverseRequestExists = errors.New("store: the other person already sent a request")
	ErrSameTier             = errors.New("store: contact is already in that tier")
)

// SeatRefusal wraps a capacity refusal with whose seats ran out, because an
// accept checks two people and the user needs to be told which one is full.
type SeatRefusal struct {
	UserID bson.ObjectID
	Err    error // a *capacity.Refusal
}

func (e *SeatRefusal) Error() string { return fmt.Sprintf("%s: %v", e.UserID.Hex(), e.Err) }
func (e *SeatRefusal) Unwrap() error { return e.Err }
