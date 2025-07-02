package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-utils/v2/retryhttp"
)

func (f *FlutterInstaller) downloadAndUnarchiveBundle(bundleURL, targetDir string) error {
	if err := validateFlutterURL(bundleURL); err != nil {
		return err
	}

	bundleTarPth, err := f.downloadBundle(bundleURL)
	if err != nil {
		return err
	}

	if err := f.unarchiveBundle(bundleTarPth, targetDir); err != nil {
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

func (f *FlutterInstaller) downloadBundle(bundleURL string) (string, error) {
	resp, err := retryhttp.NewClient(f.Logger).Get(bundleURL)
	if err != nil {
		return "", err
	}
	defer func(body io.ReadCloser) {
		if err := resp.Body.Close(); err != nil {
			f.Debugf("Failed to close response body: %s", err)
		}
	}(resp.Body)

	tmpDir, err := pathutil.NewPathProvider().CreateTempDir("__flutter-sdk__")
	if err != nil {
		return "", err
	}

	sdkTarPth := filepath.Join(tmpDir, "flutter.tar")
	file, err := os.Create(sdkTarPth)
	if err != nil {
		return "", err
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			f.Debugf("Failed to close file: %s", err)
		}
	}(file)

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", err
	}

	if err := resp.Body.Close(); err != nil {
		return "", err
	}

	return sdkTarPth, nil
}

func (f *FlutterInstaller) unarchiveBundle(tarPth, targetDir string) error {
	// using -J to support tar.xz
	// --no-same-owner to NOT preserve owners (default is to preserve, if ran as user 'root'),
	// we want to set to current user as owner to prevent error due to git configuration (https://git-scm.com/docs/git-config/2.35.2#Documentation/git-config.txt-safedirectory)
	tarCmd := f.CmdFactory.Create("tar", []string{"--no-same-owner", "-xJf", tarPth, "-C", targetDir}, nil)

	f.Donef("$ %s", tarCmd.PrintableCommandArgs())
	out, err := tarCmd.RunAndReturnTrimmedCombinedOutput()
	fmt.Println(out)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("tar command failed: %s, out: %s", err, out)
		}
		return fmt.Errorf("run tar command: %s", err)
	}

	return nil
}
