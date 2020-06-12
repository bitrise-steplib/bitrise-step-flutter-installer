package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
)

func downloadAndUnarchiveBundle(bundleURL, targetDir string) error {
	url, err := url.Parse(bundleURL)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	// Expecting URL similar to: https://storage.googleapis.com/flutter_infra/releases/beta/macos/flutter_macos_v1.6.3-beta.zip
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

	bundleTarPth, err := downloadBundle(bundleURL)
	if err != nil {
		return err
	}

	if err := unarchiveBundle(bundleTarPth, targetDir); err != nil {
		return err
	}
	return nil
}

func downloadBundle(bundleURL string) (string, error) {
	var resp *http.Response
	if err := retry.Times(2).Wait(5 * time.Second).Try(func(attempt uint) error {
		if attempt > 0 {
			log.TWarnf("%d query attempt failed", attempt)
		}

		var err error
		resp, err = http.Get(bundleURL)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			return fmt.Errorf("query failed, status code: %d, response body: %s", resp.StatusCode, body)
		}

		return nil
	}); err != nil {
		return "", err
	}

	tmpDir, err := pathutil.NormalizedOSTempDirPath("__flutter-sdk__")
	if err != nil {
		return "", err
	}

	sdkTarPth := filepath.Join(tmpDir, "flutter.tar")
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := fileutil.WriteBytesToFile(sdkTarPth, body); err != nil {
		return "", err
	}

	return sdkTarPth, nil
}

func unarchiveBundle(tarPth, targetDir string) error {
	// using -J to support tar.xz
	tarCmd, err := command.NewWithParams("tar", "-xJf", tarPth, "-C", targetDir)
	if err != nil {
		return fmt.Errorf("failed to create command, error: %s", err)
	}

	out, err := tarCmd.RunAndReturnTrimmedCombinedOutput()
	fmt.Println(out)
	if err != nil {
		if errorutil.IsExitStatusError(err) {
			return fmt.Errorf("tar command failed: %s, out: %s", err, out)
		}
		return fmt.Errorf("failed to run tar command, error: %s", err)
	}

	return nil
}
