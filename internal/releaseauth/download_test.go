package releaseauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDiscoveryPersistsAndPinsFeed(t *testing.T) {
	m, p, key := fixture(t)
	path := filepath.Join(t.TempDir(), "trust.db")
	if err := Bootstrap(path, p.PublicKey, p.Channel, p.Platform); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	document := signed(t, m, key)
	client := &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://github.com/cottman99/pf-remote/releases/download/update-preview/windows-x64.release.json" {
			t.Fatal("unexpected release source")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(document))), Header: make(http.Header)}, nil
	})}
	if _, err := s.Discover(context.Background(), client, p); err != nil {
		t.Fatal(err)
	}
	m.Sequence--
	document = signed(t, m, key)
	_, err = s.Discover(context.Background(), client, p)
	expectCode(t, err, "RELEASE_REPLAY_REJECTED")
}

func TestVerifiedDownloadAndCleanup(t *testing.T) {
	m, p, key := fixture(t)
	r, err := Verify(signed(t, m, key), p)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"synthetic artifact", "truncated", "SYNTHETIC ARTIFACT", "synthetic artifact too long"} {
		t.Run(content, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { io.WriteString(w, content) }))
			defer server.Close()
			stage := t.TempDir()
			f, err := r.DownloadArtifact(context.Background(), server.Client(), server.URL+"/", "payload.zip", stage)
			if content == "synthetic artifact" {
				if err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(f)
				f.Close()
				os.Remove(f.Name())
				if err != nil || string(got) != content {
					t.Fatal("verified handle not rewound")
				}
			} else if err == nil {
				f.Close()
				t.Fatal("bad content accepted")
			}
			files, err := os.ReadDir(stage)
			if err != nil || len(files) != 0 {
				t.Fatal("staged content leaked")
			}
		})
	}
}

func TestDownloadRejectsHTTPRedirectAndSize(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "http://example.com/unsigned", http.StatusFound)
		case "/oversize":
			io.WriteString(w, strings.Repeat("x", 20))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, address := range []string{"http://example.com/", server.URL + "/redirect", server.URL + "/oversize", server.URL + "/missing"} {
		if err := download(context.Background(), server.Client(), address, 10, io.Discard); err == nil {
			t.Fatal("unsafe response accepted")
		}
	}
}
