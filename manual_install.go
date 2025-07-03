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

	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-utils/v2/retryhttp"
)

func (f *FlutterInstaller) DownloadFlutterSDK() error {
	required := f.Input.Version
	if required == "" {
		return fmt.Errorf("input: 'Flutter SDK git repository version' (version) is not")
	}

	f.Infof("Downloading Flutter SDK")

	sdkPathParent := filepath.Join(os.Getenv("HOME"), "flutter-sdk")
	flutterSDKPath := filepath.Join(sdkPathParent, "flutter")

	f.Printf("Cleaning SDK target path: %s", sdkPathParent)
	if err := os.RemoveAll(sdkPathParent); err != nil {
		return fmt.Errorf("remove path(%s): %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		return fmt.Errorf("create folder (%s): %s", sdkPathParent, err)
	}

	if validateFlutterURL(required) == nil {
		f.Infof("Downloading and unarchiving Flutter from installation bundle: %s", required)

		if err := f.downloadAndUnarchiveBundle(required, sdkPathParent); err != nil {
			return fmt.Errorf("download and unarchive bundle: %s", err)
		}
	} else {
		f.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		f.Infof("Selected branch/tag: %s", required)

		// repository name ('flutter') is in the path, will be checked out there
		cmd := f.CmdFactory.Create("git", []string{
			"clone",
			"https://github.com/flutter/flutter.git",
			flutterSDKPath,
			"--depth", "1",
			"--branch", required,
		}, nil)
		out, err := cmd.RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			return fmt.Errorf("clone git repo for tag/branch: %s: %s", required, out)
		}
	}

	f.Printf("Adding flutter bin directory to $PATH")
	f.Debugf("PATH: %s", os.Getenv("PATH"))

	path := filepath.Join(flutterSDKPath, "bin")
	path += ":" + filepath.Join(flutterSDKPath, "bin", "cache", "dart-sdk", "bin")
	path += ":" + filepath.Join(flutterSDKPath, ".pub-cache", "bin")
	path += ":" + filepath.Join(os.Getenv("HOME"), ".pub-cache", "bin")
	path += ":" + os.Getenv("PATH")

	if err := os.Setenv("PATH", path); err != nil {
		return fmt.Errorf("set env: %s", err)
	}

	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		return fmt.Errorf("export env with envman: %s", err)
	}

	f.Donef("Added to $PATH")
	f.Debugf("PATH: %s", os.Getenv("PATH"))

	if f.Input.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			return fmt.Errorf("get Flutter binary path")
		}
		f.Infof("Flutter binary path: %s", flutterBinPath)

		cmdOpts := command.Opts{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
		treeCmd := f.CmdFactory.Create("tree", []string{"-L", "3", sdkPathParent}, &cmdOpts)
		f.Donef("$ %s", treeCmd.PrintableCommandArgs())
		if err := treeCmd.Run(); err != nil {
			f.Warnf("run tree command: %s", err)
		}

		f.printDirOwner(flutterSDKPath)
	}

	return nil
}

func (f *FlutterInstaller) printDirOwner(flutterSDKPath string) {
	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	dirOwnerCmd := f.CmdFactory.Create("ls", []string{"-al", flutterSDKPath}, &cmdOpts)
	f.Donef("$ %s", dirOwnerCmd.PrintableCommandArgs())
	if err := dirOwnerCmd.Run(); err != nil {
		f.Warnf("run ls: %s", err)
	}
}

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
