package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/sliceutil"
)

type config struct {
	Version string `env:"version,required"`
}

func failf(msg string, args ...interface{}) {
	log.Errorf(msg, args...)
	os.Exit(1)
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
	} else {
		log.Infof("Preinstalled Flutter version:")
		versionCmd := command.New("flutter", "--version").SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", versionCmd.PrintableCommandArgs())
		fmt.Println()
		if err := versionCmd.Run(); err != nil {
			failf("Failed to check flutter version, error: %s", err)
		}
	}

	// Upgrade, if already installed and a release is channel is required
	if preInstalled && sliceutil.IsStringInSlice(cfg.Version, []string{"stable", "beta", "dev", "master"}) {
		fmt.Println()
		log.Infof("Setting flutter channel")
		channelCmd := command.New("flutter", "channel", cfg.Version).SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", channelCmd.PrintableCommandArgs())
		fmt.Println()
		if err := channelCmd.Run(); err != nil {
			failf("Failed to set flutter channel, error: %s", err)
		}

		fmt.Println()
		log.Infof("Upgrading flutter")
		upgradeCmd := command.New("flutter", "upgrade").SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", channelCmd.PrintableCommandArgs())
		fmt.Println()
		if err := upgradeCmd.Run(); err != nil {
			failf("Failed to set flutter channel, error: %s", err)
		}
	} else {
		fmt.Println()
		log.Infof("Downloading Flutter SDK")
		log.Printf("git clone")

		sdkLocation := filepath.Join(os.Getenv("HOME"), "flutter-sdk")

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
		log.Infof("Check flutter doctor")
		doctorCmd := command.New("flutter", "doctor").SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", doctorCmd.PrintableCommandArgs())
		fmt.Println()
		if err := doctorCmd.Run(); err != nil {
			failf("Failed to check flutter doctor, error: %s", err)
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
}
