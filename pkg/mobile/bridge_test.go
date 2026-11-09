package mobile

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedWebHandler(t *testing.T) {
	handler := GetWebHandler()

	// Test 1: Root path "/"
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for '/', got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SHADOWSTAR") {
		t.Fatalf("expected index.html to contain 'SHADOWSTAR', got %s", body)
	}

	// Test 2: style.css
	reqCSS := httptest.NewRequest("GET", "/style.css", nil)
	recCSS := httptest.NewRecorder()
	handler.ServeHTTP(recCSS, reqCSS)
	if recCSS.Code != http.StatusOK {
		t.Fatalf("expected status 200 for '/style.css', got %d", recCSS.Code)
	}

	// Test 3: app.js
	reqJS := httptest.NewRequest("GET", "/app.js", nil)
	recJS := httptest.NewRecorder()
	handler.ServeHTTP(recJS, reqJS)
	if recJS.Code != http.StatusOK {
		t.Fatalf("expected status 200 for '/app.js', got %d", recJS.Code)
	}
}
