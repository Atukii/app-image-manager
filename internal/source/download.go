package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ErrDigestMismatch means a downloaded Asset does not match the Digest its
// Release Source published, so it must not be installed.
var ErrDigestMismatch = errors.New("download does not match the published digest")

// Progress is told how many bytes of total are downloaded; total is 0 when
// unknown.
type Progress func(done, total int64)

// Download fetches asset to dest. When the asset has a Digest, the download
// must match it: otherwise dest is not created and ErrDigestMismatch is
// returned. dest only ever appears complete and verified.
func Download(ctx context.Context, client *http.Client, asset Asset, dest string, progress Progress) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", asset.URL, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	sum := sha256.New()
	w := io.MultiWriter(tmp, sum, &counter{total: max(asset.Size, resp.ContentLength, 0), progress: progress})
	_, err = io.Copy(w, resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("downloading %s: %w", asset.URL, err)
	}
	if asset.Digest != "" && asset.Digest != digestOf(sum) {
		return fmt.Errorf("%s: %w", asset.URL, ErrDigestMismatch)
	}
	return os.Rename(tmp.Name(), dest)
}

func digestOf(h hash.Hash) string {
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// counter reports progress as bytes are written through it.
type counter struct {
	done, total int64
	progress    Progress
}

func (c *counter) Write(p []byte) (int, error) {
	c.done += int64(len(p))
	if c.progress != nil {
		c.progress(c.done, c.total)
	}
	return len(p), nil
}
