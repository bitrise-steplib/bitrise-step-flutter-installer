package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/bitrise-tools/go-steputils/tools"
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

	fmt.Println()
	log.Infof("Downloading Flutter SDK")
	log.Printf("git clone")

	sdkLocation := filepath.Join(os.Getenv("HOME"), "flutter-sdk")

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

	log.Printf("set in $PATH")

	path := filepath.Join(sdkLocation, "bin") + ":" + os.Getenv("PATH")

	if err := os.Setenv("PATH", path); err != nil {
		failf("Failed to set env, error: %s", err)
	}

	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		failf("Failed to export env with envman, error: %s", err)
	}

	log.Donef("Done")

	fmt.Println()
	log.Infof("Flutter version")

	versionCmd := command.New("flutter", "--version").SetStdout(os.Stdout).SetStderr(os.Stderr)

	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()

	if err := versionCmd.Run(); err != nil {
		failf("Failed to check flutter version, error: %s", err)
	}

	fmt.Println()
	log.Infof("Check flutter doctor")

	doctorCmd := command.New("flutter", "doctor").SetStdout(os.Stdout).SetStderr(os.Stderr)

	log.Donef("$ %s", doctorCmd.PrintableCommandArgs())
	fmt.Println()

	if err := doctorCmd.Run(); err != nil {
		failf("Failed to check flutter doctor, error: %s", err)
	}
}
