package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/retry"
)

func downloadAndUnarchiveBundle(bundleURL, targetDir string) error {
	if err := validateFlutterURL(bundleURL); err != nil {
		return err
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

/*
Expecting URL similar to: https://storage.googleapis.com/flutter_infra/releases/beta/macos/flutter_macos_v1.6.3-beta.zip
*/
func validateFlutterURL(bundleURL string) error {
	flutterURL, err := url.Parse(bundleURL)
	if err != nil {
		return err
	}

	if flutterURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s, expecting https", flutterURL.Scheme)
	}

	const storageHost = "storage.googleapis.com"
	if flutterURL.Host != storageHost {
		return fmt.Errorf("invalid hostname, expecting %s", storageHost)
	}

	const sep = "/"
	pathParts := strings.Split(strings.TrimLeft(flutterURL.EscapedPath(), sep), sep)
	foundMatch := false
	flutterPaths := []string{"flutter_infra", "flutter_infra_release"}
	if len(pathParts) > 0 {
		path := pathParts[0]
		for _, validPath := range flutterPaths {
			if validPath == path {
				foundMatch = true
				break
			}
		}
	}
	if !foundMatch {
		return fmt.Errorf("invalid path, expecting it to begin with one of: %v", flutterPaths)
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
	f, err := os.Create(sdkTarPth)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}

	if err := resp.Body.Close(); err != nil {
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
