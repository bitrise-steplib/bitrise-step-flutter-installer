package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/retry"
)

func unarchiveBundle(bundleURL, targetDir string) (err error) {
	url, err := url.Parse(bundleURL)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	if url.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s, expecting https", url.Scheme)
	}

	const storageHost = "storage.googleapis.com"
	if url.Host != storageHost {
		return fmt.Errorf("invalid hostname, expecting %s", storageHost)
	}

	const sep = "/"
	pathParts := strings.Split(strings.TrimLeft(url.EscapedPath(), sep), sep)

	const flutterPath = "flutter_infra"
	if !(len(pathParts) > 0 && pathParts[0] == flutterPath) {
		return fmt.Errorf("invalid path, expecting it to begin with: %s", flutterPath)
	}

	if err := runRequest(bundleURL, targetDir); err != nil {
		return fmt.Errorf("failed to download bundle, error: %s", err)
	}
	return nil
}

func runRequest(bundleURL string, targetDir string) (err error) {
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

		if resp.StatusCode != http.StatusOK {
			responseBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			return fmt.Errorf("query failed, status code: %d, response body: %s", resp.StatusCode, responseBody)
		}

		// using -J to support tar.xz
		tarCmd, err := command.NewWithParams("tar", "-xJf", "-", "-C", targetDir)
		if err != nil {
			return fmt.Errorf("failed to create command, error: %s", err)
		}
		tarCmd.SetStdin(resp.Body)

		out, err := tarCmd.RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			if errorutil.IsExitStatusError(err) {
				return fmt.Errorf("tar command failed: %s, out: %s", err, out)
			}
			return fmt.Errorf("failed to run tar command, error: %s", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("%s", err)
	}
	return nil
}
