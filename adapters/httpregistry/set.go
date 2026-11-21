package httpregistry

import (
	"context"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// Set fronts several registries as one, so no single registry is a point of
// failure or a choke for publishers. A publish goes to the first registry, then
// to the next only while each answers ErrUnavailable; once one has accepted the
// entry, the client waits on that registry's commit and never re-sends it
// elsewhere. A refusal ends the publish, since every honest registry refuses the
// same entry. Reads take the first registry that answers.
//
// Every validator's registry fronts the same chain, so which one a publisher uses
// changes where its request lands, never what commits.
type Set struct {
	clients  []*Client
	accepted func(base string)
}

var _ ports.Registry = (*Set)(nil)

// NewSet orders clients starting at first (mod len), so publishers that each pick
// a different first registry spread their load across all of them.
func NewSet(clients []*Client, first int) *Set {
	n := len(clients)
	ordered := make([]*Client, 0, n)
	for i := 0; i < n; i++ {
		ordered = append(ordered, clients[((first%n)+n+i)%n])
	}
	s := &Set{clients: ordered}
	for _, c := range ordered {
		c.OnAccepted(func(base string) {
			if s.accepted != nil {
				s.accepted(base)
			}
		})
	}
	return s
}

// OnAccepted registers fn to run when whichever registry takes the entry accepts
// it, with that registry's base URL.
func (s *Set) OnAccepted(fn func(base string)) { s.accepted = fn }

func (s *Set) Publish(ctx context.Context, e ports.Entry) error {
	var errs []error
	for _, c := range s.clients {
		err := c.Publish(ctx, e)
		if err == nil || !errors.Is(err, ErrUnavailable) {
			return err
		}
		errs = append(errs, err)
	}
	return fmt.Errorf("every registry was unavailable (%d tried): %w", len(errs), errors.Join(errs...))
}

func (s *Set) Lookup(ctx context.Context, root ports.Hash) (ports.Entry, bool, error) {
	var errs []error
	for _, c := range s.clients {
		e, ok, err := c.Lookup(ctx, root)
		if err == nil {
			return e, ok, nil
		}
		errs = append(errs, err)
	}
	return ports.Entry{}, false, errors.Join(errs...)
}

func (s *Set) All(ctx context.Context) ([]ports.Entry, error) {
	var errs []error
	for _, c := range s.clients {
		es, err := c.All(ctx)
		if err == nil {
			return es, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}
