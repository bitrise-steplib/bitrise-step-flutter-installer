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
	Version   string `env:"version,required"`
	BundleURL string `env:"installation_bundle_url"`
	IsDebug   bool   `env:"is_debug,required"`
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

	installed := true
	if strings.TrimSpace(cfg.BundleURL) != "" {
		log.Infof("Will install Flutter from installation bundle: %s", cfg.BundleURL)

		log.Printf("Cleaning SDK target path: %s", sdkLocation)
		if err := os.RemoveAll(sdkLocation); err != nil {
			failf("Failed to remove path(%s), error: %s", sdkLocation, err)
		}

		if err := installBundle(cfg.BundleURL, sdkLocation); err != nil {
			log.Warnf("Failed to install bundle, error: %s", err)
			log.Infof("Falling back to installation from git repository.")
			installed = false
		}
	}

	if !installed {
		log.Infof("Will install Flutter from the git repositry (https://github.com/flutter/flutter.git), selected branch/tag: %s", cfg.Version)

		log.Printf("Cleaning SDK target path: %s", sdkLocation)
		if err := os.RemoveAll(sdkLocation); err != nil {
			failf("Failed to remove path(%s), error: %s", sdkLocation, err)
		}

		gitRepo, err := git.New(sdkLocation)
		if err != nil {
			failf("Failed to open git repo, error: %s", err)
		}

		if err := gitRepo.CloneTagOrBranch("https://github.com/flutter/flutter.git", cfg.Version).Run(); err != nil {
			failf("Failed to clone git repo for tag/branch: %s, error: %s", cfg.Version, err)
		}
	}

	log.Printf("Adding flutter bin directory to $PATH")
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
