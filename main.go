package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/mholt/archiver"
)

type config struct {
	Version string `env:"version,required"`

	BundleURL string `env:"installation_bundle_url"`

	IsDebug bool `env:"is_debug,required"`
}

func failf(msg string, args ...interface{}) {
	log.Errorf(msg, args...)
	os.Exit(1)
}

func matchVersion(versionOutput string) (string, error) {
	versionRegexp := regexp.MustCompile(`(?im)^Flutter\s+(\S+?)\s+`)
	submatches := versionRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter version")
	}
	return submatches[1], nil
}

func matchChannel(versionOutput string) (string, error) {
	channelRegexp := regexp.MustCompile(`(?im)\s+channel\s+(\S+?)\s+`)
	submatches := channelRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter channel")
	}
	return submatches[1], nil
}

type flutterVersion struct {
	version string
	channel string
}

func flutterVersionInfo() (flutterVersion, string, error) {
	versionCmd := command.New("flutter", "--version")
	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()
	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		if errorutil.IsExitStatusError(err) {
			return flutterVersion{}, out, fmt.Errorf("failed to get flutter version, error: %s, out: %s", err, out)
		}
		return flutterVersion{}, "", fmt.Errorf("failed to get flutter version, error: %s", err)
	}

	channel, err := matchChannel(out)
	if err != nil {
		return flutterVersion{}, out, err
	}

	version, err := matchVersion(out)
	if err != nil {
		return flutterVersion{channel: channel}, out, err
	}

	return flutterVersion{
		channel: channel,
		version: version,
	}, out, nil
}

func unarchiveBundle(bundleURL string, targetPath string) error {
	url, err := url.Parse(bundleURL)
	if err != nil {
		return err
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

	archive, err := ioutil.TempFile("", "*"+path.Ext(url.Path))
	if err != nil {
		return err
	}

	defer func() {
		if err := archive.Close(); err != nil {
			log.Warnf("Failed to close file, error: %s", err)
		}
	}()

	_, err = io.Copy(archive, resp.Body)
	if err != nil {
		return err
	}

	if err := archiver.Unarchive(archive.Name(), targetPath); err != nil {
		return err
	}
	return nil
}

func runFlutterDoctor() error {
	fmt.Println()
	log.Infof("Check flutter doctor")
	doctorCmd := command.New("flutter", "doctor").SetStdout(os.Stdout).SetStderr(os.Stderr)
	log.Donef("$ %s", doctorCmd.PrintableCommandArgs())
	fmt.Println()
	if err := doctorCmd.Run(); err != nil {
		return fmt.Errorf("failed to check flutter doctor, error: %s", err)
	}
	return nil
}

func main() {
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(cfg)

	preInstalled := true
	_, err := exec.LookPath("flutter")
	if err != nil {
		preInstalled = false
		log.Printf("Flutter is not preinstalled.")
	}

	var versionInfo flutterVersion
	if preInstalled {
		var rawVersionOutput string
		versionInfo, rawVersionOutput, err = flutterVersionInfo()
		if err != nil {
			log.Warnf("%s", err)
		}
		log.Printf(rawVersionOutput)
	}

	if versionInfo.version != "" {
		log.Infof("Preinstalled Flutter version: %s", versionInfo.version)
	}

	requiredVersion := strings.TrimSpace(cfg.Version)
	if preInstalled && sliceutil.IsStringInSlice(requiredVersion, []string{"stable", "beta", "dev", "master"}) && requiredVersion == versionInfo.channel {
		log.Infof("Required Flutter channel (%s) matches preinstalled Flutter channel (%s), skipping installation.", requiredVersion, versionInfo.channel)

		if cfg.IsDebug {
			if err := runFlutterDoctor(); err != nil {
				failf("%s", err)
			}
		}
		return
	}

	fmt.Println()
	log.Infof("Downloading Flutter SDK")

	sdkLocation := filepath.Join(os.Getenv("HOME"), "flutter-sdk")

	log.Printf("Cleaning SDK target path: %s", sdkLocation)
	if err := os.RemoveAll(sdkLocation); err != nil {
		failf("Failed to remove path(%s), error: %s", sdkLocation, err)
	}

	installed := false
	if strings.TrimSpace(cfg.BundleURL) != "" {
		url, err := url.Parse(cfg.BundleURL)
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

		if err := unarchiveBundle(cfg.BundleURL, sdkLocation); err != nil {
			log.Warnf("Installing Flutter from installation bundle failed, error: %s", err)
			log.Infof("Falling back to cloning Flutter git repository.")
		}
		installed = true
	}

	if !installed {
		log.Printf("git clone")
		gitRepo, err := git.New(sdkLocation)
		if err != nil {
			failf("Failed to open git repo, error: %s", err)
		}

		if err := gitRepo.CloneTagOrBranch("https://github.com/flutter/flutter.git", cfg.Version).Run(); err != nil {
			failf("Failed to clone git repo for tag/branch: %s, error: %s", cfg.Version, err)
		}
	}

	log.Printf("adding flutter bin directory to $PATH")
	path := filepath.Join(sdkLocation, "bin") + ":" + os.Getenv("PATH")
	if err := os.Setenv("PATH", path); err != nil {
		failf("Failed to set env, error: %s", err)
	}
	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		failf("Failed to export env with envman, error: %s", err)
	}
	log.Donef("Added to $PATH")

	fmt.Println()
	log.Infof("Flutter version")
	_, rawVersionOutput, err := flutterVersionInfo()
	if err != nil {
		log.Warnf("%s", err)
	}
	log.Printf(rawVersionOutput)

	if cfg.IsDebug {
		if err := runFlutterDoctor(); err != nil {
			failf("%s", err)
		}
	}
}
