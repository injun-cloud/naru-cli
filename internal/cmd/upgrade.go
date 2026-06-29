package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/output"
)

const ghRepo = "injun-cloud/naru-cli"

func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "upgrade",
		Short:   "Update the naru CLI to the latest release",
		Long:    "Download the latest release for this OS/arch from GitHub, verify its\nchecksum, and replace the running binary in place.",
		Args:    cobra.NoArgs,
		Example: "  naru upgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(cmd.Context())
		},
	}
}

func runUpgrade(ctx context.Context) error {
	hc := &http.Client{Timeout: 60 * time.Second}

	latest, err := latestTag(ctx, hc)
	if err != nil {
		return err
	}
	if version != "dev" && strings.TrimPrefix(latest, "v") == strings.TrimPrefix(version, "v") {
		output.Info("already on the latest version (" + latest + ")")
		return nil
	}

	ext, binName := "tar.gz", "naru"
	if runtime.GOOS == "windows" {
		ext, binName = "zip", "naru.exe"
	}
	asset := fmt.Sprintf("naru_%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", ghRepo, latest)

	output.Info("downloading " + asset + " (" + latest + ")")
	archive, err := download(ctx, hc, base+"/"+asset)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	sums, err := download(ctx, hc, base+"/checksums.txt")
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(archive, sums, asset); err != nil {
		return err
	}
	bin, err := extractBinary(archive, ext, binName)
	if err != nil {
		return err
	}
	if err := selfupdate.Apply(bytes.NewReader(bin), selfupdate.Options{}); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	output.Success("upgraded to " + latest)
	return nil
}

func latestTag(ctx context.Context, hc *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+ghRepo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases API returned %d", resp.StatusCode)
	}
	var r struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.TagName == "" {
		return "", fmt.Errorf("no published release found")
	}
	return r.TagName, nil
}

func download(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET returned %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func verifyChecksum(archive, sums []byte, asset string) error {
	sum := sha256.Sum256(archive)
	want := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == asset {
			if f[0] != want {
				return fmt.Errorf("checksum mismatch for %s", asset)
			}
			return nil
		}
	}
	return fmt.Errorf("no checksum entry for %s", asset)
}

func extractBinary(archive []byte, ext, binName string) ([]byte, error) {
	if ext == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == binName {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("%s not found in archive", binName)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in archive", binName)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == binName {
			return io.ReadAll(tr)
		}
	}
}
