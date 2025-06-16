package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-flutter/fluttersdk"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/errorutil"
	. "github.com/bitrise-io/go-utils/v2/exitcode"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-steplib/bitrise-step-flutter-installer/tracker"
)

func main() {
	exitCode := run()
	os.Exit(int(exitCode))
}

func run() ExitCode {
	logger := logv2.NewLogger()

	flutterInstaller := NewFlutterInstaller()

	config, err := flutterInstaller.ProcessConfig()
	if err != nil {
		logger.Println()
		logger.Errorf(errorutil.FormattedError(fmt.Errorf("Failed to process Step inputs: %w", err)))
		return Failure
	}

	if err := flutterInstaller.Run(config); err != nil {
		logger.Println()
		logger.Errorf(errorutil.FormattedError(fmt.Errorf("Failed to execute Step: %w", err)))
		return Failure
	}

	return Success
}

type Input struct {
	Version   string `env:"version"`
	BundleURL string `env:"installation_bundle_url"`
	IsDebug   bool   `env:"is_debug,required"`
}

type Config struct {
	Input
	BundleSpecified bool
}

type FlutterInstaller struct {
}

func NewFlutterInstaller() FlutterInstaller {
	return FlutterInstaller{}
}

func (b FlutterInstaller) ProcessConfig() (Config, error) {
	var input Input
	if err := stepconf.Parse(&input); err != nil {
		return Config{}, err
	}
	stepconf.Print(input)
	fmt.Println()

	log.SetEnableDebugLog(input.IsDebug)

	bundleSpecified := strings.TrimSpace(input.BundleURL) != ""
	gitBranchSpecified := strings.TrimSpace(input.Version) != ""
	if !bundleSpecified && !gitBranchSpecified {
		return Config{}, errors.New(`One of the following inputs needs to be specified:
"Flutter SDK git repository version" (version)
"Flutter SDK installation bundle URL" (installation_bundle_url)`)
	}

	if bundleSpecified && gitBranchSpecified {
		log.Warnf("Input: 'Flutter SDK git repository version' (version) is ignored, " +
			"using 'Flutter SDK installation bundle URL' (installation_bundle_url).")
	}

	config := Config{Input: input, BundleSpecified: bundleSpecified}

	return config, nil
}

func (b FlutterInstaller) Run(cfg Config) error {
	// getting SDK versions from project files like fvm_config.json (fvm), .tool_versions (asdf), pubspec.yaml and pubspec.lock
	proj, err := flutterproject.New("./", fileutil.NewFileManager(), pathutil.NewPathChecker(), fluttersdk.NewSDKVersionFinder())
	if err != nil {
		log.Warnf("Failed to open project: %s", err)
	}
	sdkVersions, err := proj.FlutterAndDartSDKVersions()
	if err != nil {
		log.Warnf("Failed to read project SDK versions: %s", err)
	} else {
		stepTracker := tracker.NewStepTracker(logv2.NewLogger(), env.NewRepository())
		stepTracker.LogSDKVersions(sdkVersions)
		defer stepTracker.Wait()
	}

	if err = EnsureFlutterVersion(&cfg, sdkVersions); err != nil {
		return fmt.Errorf("failed to ensure Flutter version, error: %w", err)
	}

	if cfg.IsDebug {
		if err := runFlutterDoctor(); err != nil {
			return err
		}
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

func printDirOwner(flutterSDKPath string) {
	dirOwnerCmd := command.NewWithStandardOuts("ls", "-al", flutterSDKPath)
	log.Donef("$ %s", dirOwnerCmd.PrintableCommandArgs())
	fmt.Println()
	if err := dirOwnerCmd.Run(); err != nil {
		log.Warnf("Failed to run ls: %s", err)
	}
}
