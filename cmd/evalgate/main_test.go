package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"evalgate/internal/gate"
)

func TestReleaseFixtures(t *testing.T) {
	for _, test := range []struct{ version, status string }{
		{"support-v2", "Blocked"},
		{"support-v3", "Review required"},
	} {
		run, err := evaluate(t.Context(), "@suite.json", "@support-v1.json", versionPath(test.version))
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != test.status || len(run.Results) != 6 {
			t.Fatalf("%s: got %s with %d cases", test.version, run.Status, len(run.Results))
		}
		if run.Results[0].Regression != (test.status == "Blocked") {
			t.Fatalf("%s: unexpected refund regression state", test.version)
		}
	}
}

func TestReviewPersistsAndUpdatesGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	store, err := gate.OpenStore(t.Context(), os.Getenv("EVALGATE_TEST_DATABASE_URL"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := routes(store, nil, t.TempDir())
	call := func(method, url, body string, want int) []byte {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, url, strings.NewReader(body)))
		if recorder.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, url, recorder.Code, want, recorder.Body.String())
		}
		return recorder.Body.Bytes()
	}
	call(http.MethodPost, "/api/runs", `{"baseline":"support-v1","candidate":"support-v1"}`, 400)
	var run gate.Run
	data := call(http.MethodPost, "/api/runs", `{"baseline":"support-v1","candidate":"support-v3"}`, 201)
	if err = json.Unmarshal(data, &run); err != nil || run.Status != "Review required" {
		t.Fatalf("unexpected initial run: %v, %s", err, data)
	}
	prefix := "/api/runs/" + run.ID + "/cases/"
	call(http.MethodPatch, prefix+"refund-ar", `{"decision":"approved"}`, 404)
	call(http.MethodPatch, prefix+"handoff-ar", `{"decision":"rejected"}`, 200)
	call(http.MethodPatch, prefix+"handoff-ar", `{"decision":"approved"}`, 200)

	reopened, err := gate.OpenStore(t.Context(), os.Getenv("EVALGATE_TEST_DATABASE_URL"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	history, err := reopened.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, saved := range history {
		if saved.ID == run.ID && saved.Status == "Passed" && saved.Results[3].Review == "approved" {
			return
		}
	}
	t.Fatal("approved review was not persisted")
}

func TestPublicDemoSessionIsolationAndImportDisabled(t *testing.T) {
	handler := routes(nil, gate.NewDemoStore(10, 5, time.Hour), t.TempDir())
	call := func(method, url, body string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, url, strings.NewReader(body))
		if cookie != nil {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, url, recorder.Code, want, recorder.Body.String())
		}
		return recorder
	}
	newSession := func() *http.Cookie {
		t.Helper()
		cookies := call(http.MethodGet, "/api/config", "", nil, 200).Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatalf("unexpected session cookie: %#v", cookies)
		}
		return cookies[0]
	}
	visitorA, visitorB := newSession(), newSession()
	var run gate.Run
	created := call(http.MethodPost, "/api/runs", `{"baseline":"support-v1","candidate":"support-v3"}`, visitorA, 201)
	if err := json.Unmarshal(created.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	var otherHistory []gate.Run
	other := call(http.MethodGet, "/api/runs", "", visitorB, 200)
	if err := json.Unmarshal(other.Body.Bytes(), &otherHistory); err != nil || len(otherHistory) != 0 {
		t.Fatalf("another visitor saw history: %v, %s", err, other.Body.String())
	}
	reviewURL := "/api/runs/" + run.ID + "/cases/handoff-ar"
	call(http.MethodPatch, reviewURL, `{"decision":"approved"}`, visitorB, 404)
	approved := call(http.MethodPatch, reviewURL, `{"decision":"approved"}`, visitorA, 200)
	if err := json.Unmarshal(approved.Body.Bytes(), &run); err != nil || run.Status != "Passed" {
		t.Fatalf("owner could not approve review: %v, %s", err, approved.Body.String())
	}
	call(http.MethodPost, "/api/import", `{}`, visitorA, http.StatusForbidden)
}
