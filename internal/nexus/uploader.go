package nexus

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

type Uploader struct {
	Client *http.Client
}

func (u Uploader) PutFile(ctx context.Context, url, filePath, user, pass string) error {
	st, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat file: path=%q: %w", filePath, err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, f)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)

	}

	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/octet-stream")

	// Optional bei Proxies/Servern
	req.ContentLength = st.Size()

	c := u.Client
	if c == nil {
		c = http.DefaultClient
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload failed: url=%q file=%q bytes=%d status=%s body=%q",
			url, filePath, st.Size(), resp.Status, string(b))
	}
	return nil
}
