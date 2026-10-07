// Package server exposes the JSON API and web UI.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ParadoxiusBlack/Launcher-Locator/internal/store"
)

const (
	maxUpload = 10 << 20
	maxJSON   = 1 << 20
)

type Server struct {
	store     *store.Store
	uploadDir string
	admin     bool
	mux       *http.ServeMux
}

// New builds the HTTP handler. When admin is true, the developer endpoints
// (adding maps, official indicators, reviewing submissions) are enabled.
func New(st *store.Store, ui fs.FS, uploadDir string, admin bool) (*Server, error) {
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, err
	}
	s := &Server{store: st, uploadDir: uploadDir, admin: admin, mux: http.NewServeMux()}
	m := s.mux
	m.Handle("/", http.FileServerFS(ui))
	m.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))
	m.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"admin": s.admin})
	})
	m.HandleFunc("GET /api/maps", s.listMaps)
	m.HandleFunc("GET /api/types", s.listTypes)
	m.HandleFunc("GET /api/indicators", s.listIndicators)
	m.HandleFunc("POST /api/indicators", s.addIndicator)
	m.HandleFunc("DELETE /api/indicators/{id}", s.deleteIndicator)
	m.HandleFunc("POST /api/screenshots", s.uploadScreenshot)
	m.HandleFunc("POST /api/admin/maps", s.admined(s.addMap))
	m.HandleFunc("POST /api/admin/types", s.admined(s.addType))
	m.HandleFunc("GET /api/admin/submissions", s.admined(s.listSubmissions))
	m.HandleFunc("POST /api/admin/submissions/{id}/approve", s.admined(s.approve))
	m.HandleFunc("DELETE /api/admin/indicators/{id}", s.admined(s.deleteOfficial))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) admined(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.admin {
			writeErr(w, http.StatusForbidden, "admin mode is not enabled (run with -admin)")
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (s *Server) listMaps(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Maps()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) listTypes(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Types()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// listIndicators supports ?map=ID&types=1,2 and returns official and user
// indicators (pending submissions are only visible to admins).
func (s *Server) listIndicators(w http.ResponseWriter, r *http.Request) {
	f := store.Filter{Sources: []string{store.SourceOfficial, store.SourceUser}}
	q := r.URL.Query()
	if v := q.Get("map"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, 400, "invalid map")
			return
		}
		f.MapID = id
	}
	if v := q.Get("types"); v != "" {
		for _, p := range strings.Split(v, ",") {
			id, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				writeErr(w, 400, "invalid types")
				return
			}
			f.TypeIDs = append(f.TypeIDs, id)
		}
	}
	v, err := s.store.Indicators(f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

type indicatorReq struct {
	store.Indicator
	// Mode is "custom" (private, persisted locally), "submit" (queued for
	// review) or "official" (admin only).
	Mode string `json:"mode"`
}

func (s *Server) addIndicator(w http.ResponseWriter, r *http.Request) {
	var req indicatorReq
	if !decode(w, r, &req) {
		return
	}
	switch req.Mode {
	case "custom":
		req.Source = store.SourceUser
	case "submit":
		req.Source = store.SourceSubmission
	case "official":
		if !s.admin {
			writeErr(w, http.StatusForbidden, "admin mode is not enabled (run with -admin)")
			return
		}
		req.Source = store.SourceOfficial
	default:
		writeErr(w, 400, `mode must be "custom", "submit" or "official"`)
		return
	}
	if !validUpload(req.Screenshot) {
		writeErr(w, 400, "invalid screenshot reference")
		return
	}
	req.ID = 0
	v, err := s.store.AddIndicator(req.Indicator)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// validUpload only allows empty or a path produced by uploadScreenshot.
func validUpload(p string) bool {
	if p == "" {
		return true
	}
	name, ok := strings.CutPrefix(p, "/uploads/")
	return ok && name == filepath.Base(name) && !strings.HasPrefix(name, ".")
}

func (s *Server) deleteIndicator(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	if err := s.store.Delete(id, store.SourceUser); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var imageExt = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
}

// saveImage stores an uploaded image (multipart field "file") and returns its URL.
func (s *Server) saveImage(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "missing or too large file (max 10MB)")
		return "", false
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	ext, ok := imageExt[http.DetectContentType(head[:n])]
	if !ok {
		writeErr(w, 400, "file must be a PNG, JPEG, GIF or WebP image")
		return "", false
	}
	b := make([]byte, 12)
	rand.Read(b)
	name := hex.EncodeToString(b) + ext
	out, err := os.Create(filepath.Join(s.uploadDir, name))
	if err != nil {
		fail(w, err)
		return "", false
	}
	defer out.Close()
	if _, err := out.Write(head[:n]); err == nil {
		_, err = io.Copy(out, file)
	}
	if err != nil {
		os.Remove(out.Name())
		writeErr(w, 400, "could not save file")
		return "", false
	}
	return "/uploads/" + name, true
}

func (s *Server) uploadScreenshot(w http.ResponseWriter, r *http.Request) {
	if url, ok := s.saveImage(w, r); ok {
		writeJSON(w, http.StatusCreated, map[string]string{"url": url})
	}
}

// addMap accepts multipart fields slug, game, name and file (the map image).
func (s *Server) addMap(w http.ResponseWriter, r *http.Request) {
	url, ok := s.saveImage(w, r)
	if !ok {
		return
	}
	v, err := s.store.UpsertMap(store.Map{
		Slug: r.FormValue("slug"), Game: r.FormValue("game"), Name: r.FormValue("name"), Image: url,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) addType(w http.ResponseWriter, r *http.Request) {
	var t store.IndicatorType
	if !decode(w, r, &t) {
		return
	}
	v, err := s.store.UpsertType(t)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) listSubmissions(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Indicators(store.Filter{Sources: []string{store.SourceSubmission}})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	if err := s.store.Approve(id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteOfficial rejects a submission or removes an official indicator.
func (s *Server) deleteOfficial(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	err = s.store.Delete(id, store.SourceSubmission)
	if errors.Is(err, store.ErrNotFound) {
		err = s.store.Delete(id, store.SourceOfficial)
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
