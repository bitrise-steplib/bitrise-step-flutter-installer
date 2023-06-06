package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-utils/v2/fileutil"
	"github.com/bitrise-io/go-utils/v2/pathutil"

	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/errorutil"
	. "github.com/bitrise-io/go-utils/v2/exitcode"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
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
	IsUpdate  bool   `env:"is_update,required"`
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
	proj, err := flutterproject.New("./", fileutil.NewFileManager(), pathutil.NewPathChecker())
	if err != nil {
		log.Warnf("Failed to open project: %s", err)
	} else {
		sdkVersions, err := proj.FlutterAndDartSDKVersions()
		if err != nil {
			log.Warnf("Failed to read project SDK versions: %s", err)
		} else {
			stepTracker := tracker.NewStepTracker(logv2.NewLogger(), env.NewRepository())
			stepTracker.LogSDKVersions(sdkVersions)
			defer stepTracker.Wait()
		}
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
	if !cfg.IsUpdate && preInstalled && !cfg.BundleSpecified &&
		requiredVersion == versionInfo.channel &&
		sliceutil.IsStringInSlice(requiredVersion, []string{"stable", "beta", "dev", "master"}) {
		log.Infof("Required Flutter channel (%s) matches preinstalled Flutter channel (%s), skipping installation.",
			requiredVersion, versionInfo.channel)
		log.Infof(`Set input "Update to the latest version (is_update)" to "true"
to use the latest version from channel %s.`, requiredVersion)

		if cfg.IsDebug {
			if err := runFlutterDoctor(); err != nil {
				return err
			}
		}

		return nil
	}

	fmt.Println()
	log.Infof("Downloading Flutter SDK")
	fmt.Println()

	sdkPathParent := filepath.Join(os.Getenv("HOME"), "flutter-sdk")
	flutterSDKPath := filepath.Join(sdkPathParent, "flutter")

	log.Printf("Cleaning SDK target path: %s", sdkPathParent)
	if err := os.RemoveAll(sdkPathParent); err != nil {
		return fmt.Errorf("failed to remove path(%s), error: %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		return fmt.Errorf("failed to create folder (%s), error: %s", sdkPathParent, err)
	}

	if cfg.BundleSpecified {
		fmt.Println()
		log.Infof("Downloading and unarchiving Flutter from installation bundle: %s", cfg.BundleURL)

		if err := downloadAndUnarchiveBundle(cfg.BundleURL, sdkPathParent); err != nil {
			return fmt.Errorf("failed to download and unarchive bundle, error: %s", err)
		}
	} else {
		log.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		log.Infof("Selected branch/tag: %s", cfg.Version)

		// repository name ('flutter') is in the path, will be checked out there
		gitRepo, err := git.New(flutterSDKPath)
		if err != nil {
			return fmt.Errorf("failed to open git repo, error: %s", err)
		}

		if err := gitRepo.CloneTagOrBranch("https://github.com/flutter/flutter.git", cfg.Version).Run(); err != nil {
			return fmt.Errorf("failed to clone git repo for tag/branch: %s, error: %s", cfg.Version, err)
		}
	}

	log.Printf("Adding flutter bin directory to $PATH")
	log.Debugf("PATH: %s", os.Getenv("PATH"))

	path := filepath.Join(flutterSDKPath, "bin")
	path += ":" + filepath.Join(flutterSDKPath, "bin", "cache", "dart-sdk", "bin")
	path += ":" + filepath.Join(flutterSDKPath, ".pub-cache", "bin")
	path += ":" + filepath.Join(os.Getenv("HOME"), ".pub-cache", "bin")
	path += ":" + os.Getenv("PATH")

	if err := os.Setenv("PATH", path); err != nil {
		return fmt.Errorf("failed to set env, error: %s", err)
	}

	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		return fmt.Errorf("failed to export env with envman, error: %s", err)
	}

	log.Donef("Added to $PATH")
	log.Debugf("PATH: %s", os.Getenv("PATH"))

	if cfg.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			return fmt.Errorf("failed to get Flutter binary path")
		}
		log.Infof("Flutter binary path: %s", flutterBinPath)

		treeCmd := command.New("tree", "-L", "3", sdkPathParent).SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", treeCmd.PrintableCommandArgs())
		fmt.Println()
		if err := treeCmd.Run(); err != nil {
			log.Warnf("Failed to run tree command: %s", err)
		}

		printDirOwner(flutterSDKPath)
	}

	fmt.Println()
	log.Infof("Flutter version")
	versionCmd := command.New("flutter", "--version").SetStdout(os.Stdout).SetStderr(os.Stderr)
	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()
	if err := versionCmd.Run(); err != nil {
		return fmt.Errorf("failed to check flutter version, error: %s", err)
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
