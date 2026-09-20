package releaseauth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Discover authenticates the channel feed and commits replay state before return.
func (s *Store) Discover(ctx context.Context, client *http.Client, policy Policy) (Release, error) {
	base, err := GitHubFeed(policy.Channel, policy.Platform)
	if err != nil {
		return Release{}, err
	}
	if client == nil {
		return Release{}, fail("RELEASE_DOWNLOAD_FAILED")
	}
	var document bytes.Buffer
	if err := download(ctx, client, base+policy.Platform+".release.json", MaxEnvelope, &document); err != nil {
		return Release{}, err
	}
	return s.Accept(ctx, document.Bytes(), policy)
}

// GitHubFeed is fixed by the trusted application, never by a peer notification.
// Moving a channel tag is safe only because all downloaded bytes are verified.
func GitHubFeed(channel, platform string) (string, error) {
	if !validChannel(channel) || !validPlatform(platform) {
		return "", fail("RELEASE_POLICY_MISMATCH")
	}
	return "https://github.com/cottman99/pf-remote/releases/download/update-" + channel + "/", nil
}

func download(ctx context.Context, client *http.Client, address string, limit int64, output io.Writer) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fail("RELEASE_DOWNLOAD_FAILED")
	}
	c := *client
	c.Timeout = 15 * time.Minute
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return fail("RELEASE_DOWNLOAD_FAILED")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return fail("RELEASE_DOWNLOAD_FAILED")
	}
	response, err := c.Do(req)
	if err != nil {
		return fail("RELEASE_DOWNLOAD_FAILED")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return fail("RELEASE_DOWNLOAD_FAILED")
	}
	n, err := io.Copy(output, io.LimitReader(response.Body, limit+1))
	if err != nil || n > limit {
		return fail("RELEASE_DOWNLOAD_FAILED")
	}
	return nil
}

// DownloadArtifact creates a unique staging file in a caller-protected directory.
// It returns an open verified handle, not an executable or installation authority.
// Callers must close/remove it after use and must not reopen by an untrusted path.
func (r Release) DownloadArtifact(ctx context.Context, client *http.Client, baseURL, name, staging string) (*os.File, error) {
	if r.checkpoint.Sequence == 0 || client == nil {
		return nil, fail("RELEASE_NOT_VERIFIED")
	}
	var size int64
	for _, a := range r.metadata.Artifacts {
		if a.Name == name {
			size = a.Size
		}
	}
	if size == 0 {
		return nil, fail("RELEASE_ARTIFACT_UNKNOWN")
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "https" || base.RawQuery != "" || base.Fragment != "" || base.User != nil || base.Path == "" || base.Path[len(base.Path)-1] != '/' {
		return nil, fail("RELEASE_DOWNLOAD_FAILED")
	}
	file, err := os.CreateTemp(staging, ".update-*")
	if err != nil {
		return nil, fail("RELEASE_STAGING_FAILED")
	}
	ok := false
	defer func() {
		if !ok {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	if err = download(ctx, client, base.ResolveReference(&url.URL{Path: name}).String(), size, file); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, fail("RELEASE_STAGING_FAILED")
	}
	if err = r.VerifyArtifact(name, file); err != nil {
		return nil, err
	}
	if err = file.Sync(); err != nil {
		return nil, fail("RELEASE_STAGING_FAILED")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, fail("RELEASE_STAGING_FAILED")
	}
	ok = true
	return file, nil
}
