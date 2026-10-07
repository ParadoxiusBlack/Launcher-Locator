package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ParadoxiusBlack/Launcher-Locator/internal/store"
)

func setup(t *testing.T, admin bool) (*Server, store.Map, store.IndicatorType) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m, _ := st.UpsertMap(store.Map{Slug: "m", Game: "g", Name: "M", Image: "x.svg"})
	ty, _ := st.UpsertType(store.IndicatorType{Slug: "piat", Name: "PIAT", Color: "#f00"})
	s, err := New(st, fstest.MapFS{"index.html": {Data: []byte("hi")}}, t.TempDir(), admin)
	if err != nil {
		t.Fatal(err)
	}
	return s, m, ty
}

func do(s *Server, method, url string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(method, url, &buf))
	return w
}

func TestCustomAndSubmit(t *testing.T) {
	s, m, ty := setup(t, false)
	ind := map[string]any{"mapId": m.ID, "typeId": ty.ID, "x": .3, "y": .4, "title": "t"}
	ind["mode"] = "custom"
	if w := do(s, "POST", "/api/indicators", ind); w.Code != 201 {
		t.Fatalf("custom: %d %s", w.Code, w.Body)
	}
	ind["mode"] = "submit"
	if w := do(s, "POST", "/api/indicators", ind); w.Code != 201 {
		t.Fatalf("submit: %d", w.Code)
	}
	ind["mode"] = "official"
	if w := do(s, "POST", "/api/indicators", ind); w.Code != 403 {
		t.Fatalf("official without admin: %d", w.Code)
	}
	w := do(s, "GET", "/api/indicators?map=1&types=1", nil)
	var got []store.Indicator
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Source != store.SourceUser {
		t.Fatalf("pending submissions must be hidden: %+v", got)
	}
	if w := do(s, "GET", "/api/admin/submissions", nil); w.Code != 403 {
		t.Fatalf("admin list: %d", w.Code)
	}
	if w := do(s, "DELETE", "/api/indicators/1", nil); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
	ind["mode"] = "custom"
	ind["screenshot"] = "/uploads/../../etc/passwd"
	if w := do(s, "POST", "/api/indicators", ind); w.Code != 400 {
		t.Fatalf("bad screenshot: %d", w.Code)
	}
}

func TestAdminReviewAndUpload(t *testing.T) {
	s, m, ty := setup(t, true)
	ind := map[string]any{"mapId": m.ID, "typeId": ty.ID, "x": .3, "y": .4, "mode": "submit"}
	do(s, "POST", "/api/indicators", ind)
	if w := do(s, "POST", "/api/admin/submissions/1/approve", nil); w.Code != 204 {
		t.Fatalf("approve: %d", w.Code)
	}
	var got []store.Indicator
	json.Unmarshal(do(s, "GET", "/api/indicators", nil).Body.Bytes(), &got)
	if len(got) != 1 || got[0].Source != store.SourceOfficial {
		t.Fatalf("%+v", got)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "a.png")
	fw.Write([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("0", 64)))
	mw.Close()
	req := httptest.NewRequest("POST", "/api/screenshots", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}

	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("file", "a.png")
	fw.Write([]byte("<script>alert(1)</script>"))
	mw.Close()
	req = httptest.NewRequest("POST", "/api/screenshots", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("non-image upload accepted: %d", w.Code)
	}
}
