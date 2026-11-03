package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	relayrelease "github.com/0cv/herdr-mobile-relay/internal/release"
)

func TestServesAllShippedLazyChunks(t *testing.T) {
	root := filepath.Join("..", "..", "web")
	descriptor, err := relayrelease.LoadWebDescriptor(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := filepath.Glob(filepath.Join(root, "assets", fmt.Sprintf("*-%d.js", descriptor.Assets)))
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatalf("no lazy chunk for shipped asset version %d", descriptor.Assets)
	}

	h, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	for _, chunk := range chunks {
		requestPath := "/assets/" + filepath.Base(chunk)
		plain := httptest.NewRecorder()
		h.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if plain.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", requestPath, plain.Code, http.StatusOK)
		}
		if got := plain.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Fatalf("lazy chunk cache control = %q", got)
		}
		if plain.Header().Get("ETag") == "" {
			t.Fatal("uncompressed lazy chunk is missing an ETag")
		}

		compressedRequest := httptest.NewRequest(http.MethodGet, requestPath, nil)
		compressedRequest.Header.Set("Accept-Encoding", "br")
		compressed := httptest.NewRecorder()
		h.ServeHTTP(compressed, compressedRequest)
		if compressed.Code != http.StatusOK {
			t.Fatalf("Brotli GET %s = %d, want %d", requestPath, compressed.Code, http.StatusOK)
		}
		if compressed.Header().Get("Content-Encoding") != "br" {
			t.Fatal("lazy chunk did not serve its Brotli representation")
		}
		if compressed.Header().Get("Vary") != "Accept-Encoding" {
			t.Fatalf("Brotli Vary = %q", compressed.Header().Get("Vary"))
		}
		if compressed.Header().Get("ETag") == plain.Header().Get("ETag") {
			t.Fatal("compressed and uncompressed representations share an ETag")
		}
	}
}

func TestLazyAssetsFollowOnlyReachableCurrentVersionImports(t *testing.T) {
	h := &Handler{descriptorLoaded: true, bundleAssets: 363, lazyAssets: map[string]bool{}, files: fstest.MapFS{
		"assets/app.js":             {Data: []byte(`import("./View-363.js"); import "./Stale-364.js";`)},
		"assets/View-363.js":        {Data: []byte(`import { x } from "./shared-363.js"; import("./Nested-363.js");`)},
		"assets/shared-363.js":      {Data: []byte(`import "./View-363.js";`)},
		"assets/Nested-363.js":      {Data: []byte(`export default {};`)},
		"assets/Stale-364.js":       {Data: []byte(`import "./Unreachable-363.js";`)},
		"assets/Unreachable-363.js": {Data: []byte(`export default {};`)},
	}}
	h.loadLazyAssets("assets/app.js")
	for _, name := range []string{"View-363.js", "shared-363.js", "Nested-363.js"} {
		if !h.isVersionedLazyAsset("assets/" + name) {
			t.Fatalf("reachable chunk %s is not served", name)
		}
	}
	for _, name := range []string{"Stale-364.js", "Unreachable-363.js", "../View-363.js"} {
		if h.isVersionedLazyAsset("assets/" + name) {
			t.Fatalf("unapproved chunk %s was admitted", name)
		}
	}
}

func TestRejectsLazyChunkFromAnotherReleaseVersion(t *testing.T) {
	root := setupContentAddressedWebRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "ConversationHistory-364.js"), []byte("export default {};"), 0o644); err != nil {
		t.Fatal(err)
	}
	currentVersion := fmt.Sprintf("ConversationHistory-%d.js", 363)
	if err := os.WriteFile(filepath.Join(root, "assets", currentVersion), []byte("export default {};"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	for _, name := range []string{"ConversationHistory-364.js", currentVersion} {
		request := httptest.NewRequest(http.MethodGet, "/assets/"+name, nil)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unapproved lazy chunk %s status = %d, want %d", name, response.Code, http.StatusNotFound)
		}
	}
}
