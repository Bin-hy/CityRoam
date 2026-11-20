package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShare(t *testing.T) {
	t.Run("creates a link to the final version and serves the plan", func(t *testing.T) {
		store, trip, versions := versionFixture(t, versionPlan(30), versionPlan(50))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/share", nil))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var created shareResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if created.Slug == "" || created.URL != "/api/share/"+created.Slug {
			t.Fatalf("body = %+v", created)
		}
		if created.VersionSeq != 2 || created.TripID != trip.ID {
			t.Fatalf("body = %+v", created)
		}
		link := store.shares[created.Slug]
		if link.VersionID != versions[0].ID {
			t.Fatalf("link = %+v versions = %+v", link, versions)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, created.URL, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("share status = %d body=%s", rec.Code, rec.Body.String())
		}
		var shared sharedTripResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &shared); err != nil {
			t.Fatalf("decode share: %v", err)
		}
		if shared.Slug != created.Slug || shared.Version.ID != versions[0].ID || len(shared.Version.Plan) == 0 {
			t.Fatalf("share = %+v", shared)
		}
	})

	t.Run("refuses to share before the plan is accepted", func(t *testing.T) {
		store, trip, versions := versionFixture(t, versionPlan(30))
		for index := range store.versions[trip.ID] {
			store.versions[trip.ID][index].Status = "draft"
		}
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/share", nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		if body := decodeError(t, rec); body.Code != "no_final_version" {
			t.Fatalf("error = %+v", body)
		}
		if len(versions) != 1 {
			t.Fatalf("versions = %+v", versions)
		}
	})

	t.Run("missing trip or slug is 404", func(t *testing.T) {
		store, _, _ := versionFixture(t, versionPlan(30))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/nope/share", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("trip status = %d", rec.Code)
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/share/nope", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("slug status = %d", rec.Code)
		}
	})

	t.Run("revoked links disappear", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/share", nil))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}
		var created shareResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &created)

		link := store.shares[created.Slug]
		now := time.Unix(1700000001, 0).UTC()
		link.RevokedAt = &now
		store.shares[created.Slug] = link

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, created.URL, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

func TestShareResearch(t *testing.T) {
	t.Run("sanitizes research to public fields", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		store.versions[trip.ID][0].Research = researchRaw(t)
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/share", nil))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
		}
		var created shareResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode: %v", err)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, created.URL, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("share status = %d body=%s", rec.Code, rec.Body.String())
		}
		raw := rec.Body.String()
		for _, leak := range []string{"tool_calls", "duration_ms", "sidecar", "secret.example.com", "localhost", `"id":"h1"`, `"provider":"flyai"`, "fetched_at"} {
			if strings.Contains(raw, leak) {
				t.Fatalf("share leaks %q: %s", leak, raw)
			}
		}
		for _, want := range []string{`"accommodation"`, `"guides"`, `"notes"`, `"warnings"`, `"攻略推荐"`, `"北京攻略"`, `"测试酒店"`} {
			if !strings.Contains(raw, want) {
				t.Fatalf("share missing %q: %s", want, raw)
			}
		}

		var shared struct {
			Version struct {
				Research map[string]json.RawMessage `json:"research"`
			} `json:"version"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &shared); err != nil {
			t.Fatalf("decode share: %v", err)
		}
		for key := range shared.Version.Research {
			switch key {
			case "accommodation", "guides", "notes", "warnings":
			default:
				t.Fatalf("unexpected research key %q", key)
			}
		}
	})

	t.Run("empty research serializes as an object", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/share", nil))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
		}
		var created shareResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode: %v", err)
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, created.URL, nil))
		if !strings.Contains(rec.Body.String(), `"research":{}`) {
			t.Fatalf("empty research serialized differently: %s", rec.Body.String())
		}
	})
}
