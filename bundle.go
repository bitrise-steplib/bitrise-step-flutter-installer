package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/retry"
	"github.com/mholt/archiver"
)

func installBundle(bundleURL string, targetDir string) error {
	url, err := url.Parse(bundleURL)
	if err != nil {
		failf("%s", err)
	}

	if url.Scheme != "https" {
		failf("Invalid URL scheme: %s, expecting https", url.Scheme)
	}
	const storageHost = "storage.googleapis.com"
	if url.Host != storageHost {
		failf("Invalid hostname, expecting %s", storageHost)
	}

	archive, err := ioutil.TempFile("", "*"+path.Base(url.Path))
	if err != nil {
		failf("%s", err)
	}

	defer func() {
		if err := archive.Close(); err != nil {
			log.Warnf("Failed to close file, error: %s", err)
		}
	}()

	if err := downloadBundle(bundleURL, archive); err != nil {
		return fmt.Errorf("failed to download bundle, error: %s", err)
	}

	if err := unarchiveBundle(archive.Name(), targetDir); err != nil {
		return fmt.Errorf("failed to unarchive, error: %s", err)
	}

	return nil
}

func downloadBundle(bundleURL string, output io.Writer) error {
	if err := retry.Times(3).Wait(5 * time.Second).Try(func(attempt uint) error {
		if attempt > 0 {
			log.TWarnf("%d query attempt failed", attempt)
		}

		resp, err := http.Get(bundleURL)

		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Warnf("Failed to close http resonse body, error: %s", err)
			}
		}()

		if err != nil {
			return err
		}

		_, err = io.Copy(output, resp.Body)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to upload, error: %s", err)
	}
	return nil
}

func unarchiveBundle(archivePath string, targetDir string) error {
	if err := archiver.Unarchive(archivePath, targetDir); err != nil {
		return err
	}
	return nil
}
