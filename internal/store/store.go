// Package store persists maps, indicator types and indicators in SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Indicator sources.
const (
	SourceOfficial   = "official"   // curated by the developer, shipped to everyone
	SourceUser       = "user"       // private to this installation
	SourceSubmission = "submission" // proposed by a user, awaiting review
)

var ErrNotFound = errors.New("not found")
var ErrInvalid = errors.New("invalid input")

type Map struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Game  string `json:"game"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

type IndicatorType struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

// Indicator marks a point on a map. X and Y are fractions (0..1) of the map
// image's width and height so they survive image rescaling.
type Indicator struct {
	ID         int64   `json:"id"`
	MapID      int64   `json:"mapId"`
	TypeID     int64   `json:"typeId"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Title      string  `json:"title"`
	Details    string  `json:"details"`
	Screenshot string  `json:"screenshot"`
	Source     string  `json:"source"`
}

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS maps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  game TEXT NOT NULL,
  name TEXT NOT NULL,
  image TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS indicator_types (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  color TEXT NOT NULL,
  icon TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS indicators (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  map_id INTEGER NOT NULL REFERENCES maps(id) ON DELETE CASCADE,
  type_id INTEGER NOT NULL REFERENCES indicator_types(id) ON DELETE CASCADE,
  x REAL NOT NULL,
  y REAL NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  details TEXT NOT NULL DEFAULT '',
  screenshot TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_indicators_map ON indicators(map_id);
`

// Open opens (creating if needed) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA foreign_keys = ON", "PRAGMA busy_timeout = 5000", schema} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// UpsertMap inserts a map or updates it if the slug already exists.
func (s *Store) UpsertMap(m Map) (Map, error) {
	if m.Slug == "" || m.Name == "" || m.Game == "" || m.Image == "" {
		return m, fmt.Errorf("%w: map needs slug, game, name and image", ErrInvalid)
	}
	_, err := s.db.Exec(`INSERT INTO maps(slug,game,name,image) VALUES(?,?,?,?)
ON CONFLICT(slug) DO UPDATE SET game=excluded.game, name=excluded.name, image=excluded.image`,
		m.Slug, m.Game, m.Name, m.Image)
	if err != nil {
		return m, err
	}
	err = s.db.QueryRow(`SELECT id FROM maps WHERE slug=?`, m.Slug).Scan(&m.ID)
	return m, err
}

// UpsertType inserts an indicator type or updates it if the slug exists.
func (s *Store) UpsertType(t IndicatorType) (IndicatorType, error) {
	if t.Slug == "" || t.Name == "" || t.Color == "" {
		return t, fmt.Errorf("%w: type needs slug, name and color", ErrInvalid)
	}
	_, err := s.db.Exec(`INSERT INTO indicator_types(slug,name,color,icon) VALUES(?,?,?,?)
ON CONFLICT(slug) DO UPDATE SET name=excluded.name, color=excluded.color, icon=excluded.icon`,
		t.Slug, t.Name, t.Color, t.Icon)
	if err != nil {
		return t, err
	}
	err = s.db.QueryRow(`SELECT id FROM indicator_types WHERE slug=?`, t.Slug).Scan(&t.ID)
	return t, err
}

func (s *Store) Maps() ([]Map, error) {
	rows, err := s.db.Query(`SELECT id,slug,game,name,image FROM maps ORDER BY game, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Map{}
	for rows.Next() {
		var m Map
		if err := rows.Scan(&m.ID, &m.Slug, &m.Game, &m.Name, &m.Image); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Types() ([]IndicatorType, error) {
	rows, err := s.db.Query(`SELECT id,slug,name,color,icon FROM indicator_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IndicatorType{}
	for rows.Next() {
		var t IndicatorType
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Color, &t.Icon); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Filter narrows Indicators results. Zero values mean "any".
type Filter struct {
	MapID   int64
	TypeIDs []int64
	Sources []string
}

func (s *Store) Indicators(f Filter) ([]Indicator, error) {
	q := `SELECT id,map_id,type_id,x,y,title,details,screenshot,source FROM indicators WHERE 1=1`
	var args []any
	if f.MapID != 0 {
		q += ` AND map_id=?`
		args = append(args, f.MapID)
	}
	if len(f.TypeIDs) > 0 {
		q += ` AND type_id IN (` + placeholders(len(f.TypeIDs)) + `)`
		for _, id := range f.TypeIDs {
			args = append(args, id)
		}
	}
	if len(f.Sources) > 0 {
		q += ` AND source IN (` + placeholders(len(f.Sources)) + `)`
		for _, v := range f.Sources {
			args = append(args, v)
		}
	}
	rows, err := s.db.Query(q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Indicator{}
	for rows.Next() {
		var i Indicator
		if err := rows.Scan(&i.ID, &i.MapID, &i.TypeID, &i.X, &i.Y, &i.Title, &i.Details, &i.Screenshot, &i.Source); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// AddIndicator validates and inserts an indicator.
func (s *Store) AddIndicator(i Indicator) (Indicator, error) {
	switch i.Source {
	case SourceOfficial, SourceUser, SourceSubmission:
	default:
		return i, fmt.Errorf("%w: unknown source %q", ErrInvalid, i.Source)
	}
	if i.X < 0 || i.X > 1 || i.Y < 0 || i.Y > 1 {
		return i, fmt.Errorf("%w: x and y must be between 0 and 1", ErrInvalid)
	}
	if len(i.Title) > 200 || len(i.Details) > 5000 {
		return i, fmt.Errorf("%w: title or details too long", ErrInvalid)
	}
	res, err := s.db.Exec(`INSERT INTO indicators(map_id,type_id,x,y,title,details,screenshot,source)
VALUES(?,?,?,?,?,?,?,?)`, i.MapID, i.TypeID, i.X, i.Y, i.Title, i.Details, i.Screenshot, i.Source)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return i, fmt.Errorf("%w: unknown map or type", ErrInvalid)
		}
		return i, err
	}
	i.ID, err = res.LastInsertId()
	return i, err
}

// Get returns one indicator.
func (s *Store) Get(id int64) (Indicator, error) {
	l, err := s.Indicators(Filter{})
	if err != nil {
		return Indicator{}, err
	}
	for _, i := range l {
		if i.ID == id {
			return i, nil
		}
	}
	return Indicator{}, ErrNotFound
}

// Delete removes an indicator that has the given source.
func (s *Store) Delete(id int64, source string) error {
	res, err := s.db.Exec(`DELETE FROM indicators WHERE id=? AND source=?`, id, source)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Approve promotes a pending submission to an official indicator.
func (s *Store) Approve(id int64) error {
	res, err := s.db.Exec(`UPDATE indicators SET source=? WHERE id=? AND source=?`, SourceOfficial, id, SourceSubmission)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
