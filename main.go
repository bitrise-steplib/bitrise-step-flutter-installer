package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-flutter/fluttersdk"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/errorutil"
	"github.com/bitrise-io/go-utils/v2/exitcode"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"

	"github.com/bitrise-steplib/bitrise-step-flutter-installer/tracker"
)

// TODO: organize
var logger = logv2.NewLogger()
var envRepo = env.NewRepository()
var cmdFactory = command.NewFactory(envRepo)

func main() {
	exitCode := run()
	os.Exit(int(exitCode))
}

func run() exitcode.ExitCode {
	flutterInstaller := NewFlutterInstaller(envRepo)

	config, err := flutterInstaller.ProcessConfig()
	if err != nil {
		logger.Println()
		logger.Errorf(errorutil.FormattedError(fmt.Errorf("process Step inputs: %w", err)))
		return exitcode.Failure
	}

	if err := flutterInstaller.Run(config); err != nil {
		logger.Println()
		logger.Errorf(errorutil.FormattedError(fmt.Errorf("execute Step: %w", err)))
		return exitcode.Failure
	}

	return exitcode.Success
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
	envRepo env.Repository
}

func NewFlutterInstaller(envRepo env.Repository) FlutterInstaller {
	return FlutterInstaller{
		envRepo: envRepo,
	}
}

func (b FlutterInstaller) ProcessConfig() (Config, error) {
	var input Input
	envRepo := env.NewRepository()
	if err := stepconf.NewInputParser(envRepo).Parse(&input); err != nil {
		return Config{}, err
	}
	stepconf.Print(input)
	logger.Println()

	logger.EnableDebugLog(input.IsDebug)

	bundleSpecified := strings.TrimSpace(input.BundleURL) != ""
	gitBranchSpecified := strings.TrimSpace(input.Version) != ""
	if !bundleSpecified && !gitBranchSpecified {
		return Config{}, errors.New(`one of the following inputs needs to be specified:
"Flutter SDK git repository version" (version)
"Flutter SDK installation bundle URL" (installation_bundle_url)`)
	}

	if bundleSpecified && gitBranchSpecified {
		logger.Warnf("Input: 'Flutter SDK git repository version' (version) is ignored, " +
			"using 'Flutter SDK installation bundle URL' (installation_bundle_url).")
	}

	config := Config{Input: input, BundleSpecified: bundleSpecified}

	return config, nil
}

func (b FlutterInstaller) Run(cfg Config) error {
	// getting SDK versions from project files like fvm_config.json (fvm), .tool_versions (asdf), pubspec.yaml and pubspec.lock
	proj, err := flutterproject.New("./", fileutil.NewFileManager(), pathutil.NewPathChecker(), fluttersdk.NewSDKVersionFinder())
	var sdkVersions *flutterproject.FlutterAndDartSDKVersions
	if err != nil {
		logger.Warnf("open project: %s", err)
	} else {
		sdkVersions, err := proj.FlutterAndDartSDKVersions()
		if err != nil {
			logger.Warnf("read project SDK versions: %s", err)
		} else {
			stepTracker := tracker.NewStepTracker(logv2.NewLogger(), env.NewRepository())
			stepTracker.LogSDKVersions(sdkVersions)
			defer stepTracker.Wait()
		}
	}

	if err = EnsureFlutterVersion(&cfg, sdkVersions); err != nil {
		return fmt.Errorf("ensure Flutter version: %w", err)
	}

	if cfg.IsDebug {
		if err := runFlutterDoctor(); err != nil {
			return err
		}
	}

	return nil
}

func runFlutterDoctor() error {
	logger.Println()
	logger.Infof("Check flutter doctor")

	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	doctorCmd := cmdFactory.Create("flutter", []string{"doctor"}, &cmdOpts)
	logger.Donef("$ %s", doctorCmd.PrintableCommandArgs())
	logger.Println()
	if err := doctorCmd.Run(); err != nil {
		return fmt.Errorf("check flutter doctor: %s", err)
	}
	return nil
}
