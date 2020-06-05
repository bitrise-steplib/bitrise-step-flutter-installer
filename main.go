package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/sliceutil"
)

type config struct {
	Version  string `env:"version"`
	IsUpdate bool   `env:"is_update,required"`

	BundleURL string `env:"installation_bundle_url"`

	IsDebug bool `env:"is_debug,required"`
}

func failf(msg string, args ...interface{}) {
	log.Errorf(msg, args...)
	os.Exit(1)
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

	bundleSpecified := strings.TrimSpace(cfg.BundleURL) != ""
	gitBranchSpecified := strings.TrimSpace(cfg.Version) != ""
	if !bundleSpecified && !gitBranchSpecified {
		failf(`One of the following inputs needs to be specified:
"Flutter SDK git repository version" (version)
"Flutter SDK installation bundle URL" (installation_bundle_url)`)
	}
	fmt.Println()

	log.SetEnableDebugLog(cfg.IsDebug)

	if bundleSpecified && gitBranchSpecified {
		log.Warnf("Input: 'Flutter SDK git repository version' (version) is ignored, " +
			"using 'Flutter SDK installation bundle URL' (installation_bundle_url).")
	}

	preInstalled := true
	flutterBinPath, err := exec.LookPath("flutter")
	if err != nil {
		preInstalled = false
		log.Printf("Flutter is not preinstalled.")
	} else {
		log.Infof("Preinstalled Flutter binary path: %s", flutterBinPath)
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
	if !cfg.IsUpdate && preInstalled && !bundleSpecified &&
		requiredVersion == versionInfo.channel &&
		sliceutil.IsStringInSlice(requiredVersion, []string{"stable", "beta", "dev", "master"}) {
		log.Infof("Required Flutter channel (%s) matches preinstalled Flutter channel (%s), skipping installation.",
			requiredVersion, versionInfo.channel)
		log.Infof(`Set input "Update to the latest version (is_update)" to "true"
to use the latest version from channel %s.`, requiredVersion)

		if cfg.IsDebug {
			if err := runFlutterDoctor(); err != nil {
				failf("%s", err)
			}
		}

		return
	}

	fmt.Println()
	log.Infof("Downloading Flutter SDK")
	fmt.Println()

	sdkPathParent := filepath.Join(os.Getenv("HOME"), "flutter-sdk")
	flutterSDKPath := filepath.Join(sdkPathParent, "flutter")

	log.Printf("Cleaning SDK target path: %s", sdkPathParent)
	if err := os.RemoveAll(sdkPathParent); err != nil {
		failf("Failed to remove path(%s), error: %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		failf("failed to create folder (%s), error: %s", sdkPathParent, err)
	}

	if bundleSpecified {
		log.Infof("Downloading and unarchiving Flutter from installation bundle: %s", cfg.BundleURL)

		if err := unarchiveBundle(cfg.BundleURL, sdkPathParent); err != nil {
			failf("failed to download and unarchive bundle, error: %s", err)
		}
	} else {
		log.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		log.Infof("Selected branch/tag: %s", cfg.Version)

		// repository name ('flutter') is in the path, will be checked out there
		gitRepo, err := git.New(flutterSDKPath)
		if err != nil {
			failf("Failed to open git repo, error: %s", err)
		}

		if err := gitRepo.CloneTagOrBranch("https://github.com/flutter/flutter.git", cfg.Version).Run(); err != nil {
			failf("Failed to clone git repo for tag/branch: %s, error: %s", cfg.Version, err)
		}
	}

	log.Printf("Adding flutter bin directory to $PATH")
	log.Debugf("PATH: %s", os.Getenv("PATH"))

	path := filepath.Join(flutterSDKPath, "bin")
	path += ":" + filepath.Join(flutterSDKPath, "bin", "cache", "dart-sdk", "bin")
	path += ":" + filepath.Join("$HOME", ".pub-cache", "bin")
	path += ":" + os.Getenv("PATH")

	if err := os.Setenv("PATH", path); err != nil {
		failf("Failed to set env, error: %s", err)
	}

	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		failf("Failed to export env with envman, error: %s", err)
	}

	log.Donef("Added to $PATH")
	log.Debugf("PATH: %s", os.Getenv("PATH"))

	if cfg.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			failf("Failed to get Flutter binary path")
		}
		log.Infof("Flutter binary path: %s", flutterBinPath)

		treeCmd := command.New("tree", sdkPathParent).SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", treeCmd.PrintableCommandArgs())
		fmt.Println()
		if err := treeCmd.Run(); err != nil {
			log.Warnf("Failed to run tree, error: %s", err)
		}
	}

	fmt.Println()
	log.Infof("Flutter version")
	versionCmd := command.New("flutter", "--version").SetStdout(os.Stdout).SetStderr(os.Stderr)
	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()
	if err := versionCmd.Run(); err != nil {
		failf("Failed to check flutter version, error: %s", err)
	}

	if cfg.IsDebug {
		if err := runFlutterDoctor(); err != nil {
			failf("%s", err)
		}
	}
}
