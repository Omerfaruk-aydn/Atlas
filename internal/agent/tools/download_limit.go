package tools

import (
	"errors"
	"fmt"
	"io"
)

// ErrDownloadTooLarge is returned when a download exceeds the workspace's
// configured size cap.
var ErrDownloadTooLarge = errors.New("download exceeds the configured size limit")

// copyLimited copies src into dst, refusing to write more than maxBytes.
// A maxBytes of zero or less means no limit.
//
// It reads one byte past the limit rather than stopping exactly at it, so a
// body that is exactly maxBytes long succeeds and one byte more fails --
// without that, the two are indistinguishable and every file at exactly the
// limit would be rejected.
func copyLimited(dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	if maxBytes <= 0 {
		return io.Copy(dst, src)
	}

	written, err := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return written, err
	}
	if written > maxBytes {
		return written, fmt.Errorf("%w of %d bytes", ErrDownloadTooLarge, maxBytes)
	}
	return written, nil
}

// declaredTooLarge reports whether a server's declared content length is
// already over the cap, so an oversized download can be refused before a
// byte of it is written. A length of -1 means the server did not say.
func declaredTooLarge(contentLength, maxBytes int64) bool {
	return maxBytes > 0 && contentLength > maxBytes
}
