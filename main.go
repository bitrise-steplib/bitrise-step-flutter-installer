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

func main() {
	os.Exit(int(run()))
}

type FlutterInstaller struct {
	logv2.Logger
	EnvRepo    env.Repository
	CmdFactory command.Factory
	Config     Config
}

func NewFlutterInstaller(logger logv2.Logger, envRepo env.Repository, cmdFactory command.Factory, config Config) FlutterInstaller {
	return FlutterInstaller{
		Logger:     logger,
		EnvRepo:    envRepo,
		CmdFactory: cmdFactory,
		Config:     config,
	}
}

func run() exitcode.ExitCode {
	f, err := ConfigureFlutterInstaller()
	if err != nil {
		f.Errorf(errorutil.FormattedError(fmt.Errorf("process Step inputs: %w", err)))
		return exitcode.Failure
	}

	if err := f.Run(); err != nil {
		f.Errorf(errorutil.FormattedError(fmt.Errorf("execute Step: %w", err)))
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

func ConfigureFlutterInstaller() (FlutterInstaller, error) {
	envRepo := env.NewRepository()

	var input Input
	if err := stepconf.NewInputParser(envRepo).Parse(&input); err != nil {
		return FlutterInstaller{}, err
	}
	stepconf.Print(input)

	logger := logv2.NewLogger()
	logger.EnableDebugLog(input.IsDebug)

	bundleSpecified := strings.TrimSpace(input.BundleURL) != ""
	gitBranchSpecified := strings.TrimSpace(input.Version) != ""
	if !bundleSpecified && !gitBranchSpecified {
		return FlutterInstaller{}, errors.New(`one of the following inputs needs to be specified:
"Flutter SDK git repository version" (version)
"Flutter SDK installation bundle URL" (installation_bundle_url)`)
	}

	if bundleSpecified && gitBranchSpecified {
		logger.Warnf("Input: 'Flutter SDK git repository version' (version) is ignored, " +
			"using 'Flutter SDK installation bundle URL' (installation_bundle_url).")
	}

	config := Config{Input: input, BundleSpecified: bundleSpecified}

	cmdFactory := command.NewFactory(envRepo)

	// TODO: remove this when the step is stable
	// Test CI env var
	cmd := cmdFactory.Create("echo", []string{"$CI"}, nil)
	out, _ := cmd.RunAndReturnTrimmedCombinedOutput()
	logger.Debugf("echo $CI: %s", out)

	if err := envRepo.Set("CI", "true"); err != nil {
		logger.Warnf("set env: %s", err)
	}

	fi := NewFlutterInstaller(logger, envRepo, cmdFactory, config)

	return fi, nil
}

func (f *FlutterInstaller) Run() error {
	// getting SDK versions from project files (fvm, asdf, pubspec)
	proj, err := flutterproject.New("./", fileutil.NewFileManager(), pathutil.NewPathChecker(), fluttersdk.NewSDKVersionFinder())
	var sdkVersions *flutterproject.FlutterAndDartSDKVersions
	if err != nil {
		f.Warnf("open project: %s", err)
	} else {
		sdkVersions, err := proj.FlutterAndDartSDKVersions()
		if err != nil {
			f.Warnf("read project SDK versions: %s", err)
		} else {
			stepTracker := tracker.NewStepTracker(logv2.NewLogger(), env.NewRepository())
			stepTracker.LogSDKVersions(sdkVersions)
			defer stepTracker.Wait()
		}
	}

	if err = f.EnsureFlutterVersion(sdkVersions); err != nil {
		return fmt.Errorf("ensure Flutter version: %w", err)
	}

	if f.Config.IsDebug {
		if err := f.runFlutterDoctor(); err != nil {
			return err
		}
	}

	return nil
}

func (f *FlutterInstaller) runFlutterDoctor() error {
	f.Infof("Check flutter doctor")

	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	doctorCmd := f.CmdFactory.Create("flutter", []string{"doctor"}, &cmdOpts)
	f.Donef("$ %s", doctorCmd.PrintableCommandArgs())
	if err := doctorCmd.Run(); err != nil {
		return fmt.Errorf("check flutter doctor: %s", err)
	}
	return nil
}
