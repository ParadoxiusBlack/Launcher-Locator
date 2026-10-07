package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestIndicatorLifecycle(t *testing.T) {
	s := newStore(t)
	m, err := s.UpsertMap(Map{Slug: "m", Game: "g", Name: "M", Image: "x.svg"})
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := s.UpsertMap(Map{Slug: "m", Game: "g", Name: "M2", Image: "x.svg"})
	if m2.ID != m.ID {
		t.Fatal("upsert should keep id")
	}
	ty, _ := s.UpsertType(IndicatorType{Slug: "piat", Name: "PIAT", Color: "#f00"})
	ty2, _ := s.UpsertType(IndicatorType{Slug: "flare", Name: "Flare", Color: "#0f0"})

	a, err := s.AddIndicator(Indicator{MapID: m.ID, TypeID: ty.ID, X: .5, Y: .5, Source: SourceOfficial})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddIndicator(Indicator{MapID: m.ID, TypeID: ty2.ID, X: .1, Y: .2, Source: SourceSubmission}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddIndicator(Indicator{MapID: m.ID, TypeID: ty.ID, X: 2, Y: 0, Source: SourceUser}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	if _, err := s.AddIndicator(Indicator{MapID: 99, TypeID: ty.ID, X: 0, Y: 0, Source: SourceUser}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for bad map, got %v", err)
	}

	got, _ := s.Indicators(Filter{MapID: m.ID, TypeIDs: []int64{ty.ID}})
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("type filter: %+v", got)
	}
	pend, _ := s.Indicators(Filter{Sources: []string{SourceSubmission}})
	if len(pend) != 1 {
		t.Fatalf("pending: %+v", pend)
	}
	if err := s.Approve(pend[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(a.ID, SourceUser); !errors.Is(err, ErrNotFound) {
		t.Fatal("delete must respect source")
	}
	if err := s.Delete(a.ID, SourceOfficial); err != nil {
		t.Fatal(err)
	}
}
