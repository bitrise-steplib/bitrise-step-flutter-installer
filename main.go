package main

import (
	"fmt"
	"os"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/errorutil"
	"github.com/bitrise-io/go-utils/v2/exitcode"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
)

func main() {
	os.Exit(int(run()))
}

type FlutterInstaller struct {
	logv2.Logger
	EnvRepo    env.Repository
	CmdFactory command.Factory
	Input      Input
}

func NewFlutterInstaller(logger logv2.Logger, envRepo env.Repository, cmdFactory command.Factory, Input Input) FlutterInstaller {
	return FlutterInstaller{
		Logger:     logger,
		EnvRepo:    envRepo,
		CmdFactory: cmdFactory,
		Input:      Input,
	}
}

func run() exitcode.ExitCode {
	f, err := ConfigureFlutterInstaller()
	if err != nil {
		logv2.NewLogger().Errorf(errorutil.FormattedError(fmt.Errorf("process Step inputs: %w", err)))
		return exitcode.Failure
	}

	if err := f.Run(); err != nil {
		f.Errorf(errorutil.FormattedError(fmt.Errorf("execute Step: %w", err)))
		return exitcode.Failure
	}

	return exitcode.Success
}

type Input struct {
	Version string `env:"version"`
	IsDebug bool   `env:"is_debug"`
}

func ConfigureFlutterInstaller() (*FlutterInstaller, error) {
	envRepo := env.NewRepository()

	var input Input
	if err := stepconf.NewInputParser(envRepo).Parse(&input); err != nil {
		return &FlutterInstaller{}, err
	}
	stepconf.Print(input)

	logger := logv2.NewLogger()
	logger.EnableDebugLog(input.IsDebug)

	if input.Version == "" {
		logger.Warnf("Input: 'Flutter SDK git repository version' (version) is not specified, using 'stable' as a default version.")
		input.Version = "stable"
	}

	if err := envRepo.Set("CI", "true"); err != nil {
		logger.Debugf("Set env 'CI': %s", err)
	}

	cmdFactory := command.NewFactory(envRepo)

	fi := NewFlutterInstaller(logger, envRepo, cmdFactory, input)

	return &fi, nil
}

func (f *FlutterInstaller) Run() error {
	// getting SDK versions from project files (fvm, asdf, pubspec)
	if err := f.EnsureFlutterVersion(); err != nil {
		return fmt.Errorf("ensure Flutter version: %w", err)
	}

	if f.Input.IsDebug {
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
