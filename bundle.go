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
)

func downloadBundle(bundleURL string) (archivePath string, err error) {
	url, err := url.Parse(bundleURL)
	if err != nil {
		return "", fmt.Errorf("%s", err)
	}

	if url.Scheme != "https" {
		return "", fmt.Errorf("invalid URL scheme: %s, expecting https", url.Scheme)
	}
	const storageHost = "storage.googleapis.com"
	if url.Host != storageHost {
		return "", fmt.Errorf("invalid hostname, expecting %s", storageHost)
	}

	archive, err := ioutil.TempFile("", "*"+path.Base(url.Path))
	if err != nil {
		return "", fmt.Errorf("%s", err)
	}

	defer func() {
		cerr := archive.Close()
		if err == nil {
			err = cerr
		}
	}()

	if err := runRequest(bundleURL, archive); err != nil {
		return "", fmt.Errorf("failed to download bundle, error: %s", err)
	}

	return archive.Name(), nil
}

func runRequest(bundleURL string, output io.Writer) (err error) {
	if err = retry.Times(3).Wait(5 * time.Second).Try(func(attempt uint) error {
		if attempt > 0 {
			log.TWarnf("%d query attempt failed", attempt)
		}

		resp, err := http.Get(bundleURL)

		defer func() {
			cerr := resp.Body.Close()
			if err == nil {
				err = cerr
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
