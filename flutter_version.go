package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-flutter/fluttersdk"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-steplib/bitrise-step-flutter-installer/tracker"
)

var channels = []string{
	"stable",
	"beta",
	"dev",
	"main",
	"master",
}

type flutterVersion struct {
	version     string
	channel     string
	installType string
}

func NewFlutterVersion(input string) (flutterVersion, error) {
	if versions, err := parseVersionsFromJson(input, true); err == nil && len(versions) > 0 {
		return versions[0], nil
	}

	if versions, err := parseVersionFromStringLines(input, true); err == nil && len(versions) > 0 {
		return versions[0], nil
	}

	return flutterVersion{}, fmt.Errorf("parse flutter version and channel from input: %s", input)
}

func NewFlutterVersions(input string) ([]flutterVersion, error) {
	if versions, err := parseVersionsFromJson(input, false); err == nil && len(versions) > 0 {
		return versions, nil
	}

	if versions, err := parseVersionFromStringLines(input, false); err == nil && len(versions) > 0 {
		return versions, nil
	}

	return []flutterVersion{}, fmt.Errorf("parse flutter version and channel from input: %s", input)
}

func (f *FlutterInstaller) NewFlutterVersionFromCurrent() (flutterVersion, string, error) {
	versionCmd := f.CmdFactory.Create("flutter", []string{"--version", "--machine"}, nil)
	f.Donef("$ %s", versionCmd.PrintableCommandArgs())
	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return flutterVersion{}, out, fmt.Errorf("get flutter version: %s, out: %s", err, out)
		}
		return flutterVersion{}, "", fmt.Errorf("get flutter version: %w", err)
	} else {
		f.Debugf("Flutter version output: %s", out)
	}

	flutterVer, err := NewFlutterVersion(out)
	f.Debugf("Current Flutter version: %s, channel: %s", flutterVer.version, flutterVer.channel)

	return flutterVer, out, err
}

func (f *FlutterInstaller) NewFlutterVersionFromInputAndProject() (flutterVersion, error) {
	parsedVersion, err := NewFlutterVersion(strings.TrimSpace(f.Input.Version))
	if err != nil {
		f.Debugf("parse version from input: %w", err)
	} else if parsedVersion.version != "" || parsedVersion.channel != "" {
		return parsedVersion, nil
	}

	parsedVersion, err = parseProjectConfigFiles()
	if err != nil {
		f.Debugf("parse version from project config files: %w", err)
	} else if parsedVersion.version != "" || parsedVersion.channel != "" {
		return parsedVersion, nil
	}

	return flutterVersion{}, fmt.Errorf("no Flutter version specified in the configuration or project files")
}

func parseVersionsFromJson(input string, singleResult bool) ([]flutterVersion, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("input is empty")
	}

	defaultManager := ""
	if strings.Contains(input, FVMName) {
		defaultManager = FVMName
	} else if strings.Contains(input, ASDFName) {
		defaultManager = ASDFName
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(input), &obj); err == nil {
		if versionsRaw, ok := obj["versions"]; ok {
			// fvm API returns versions as an array
			if versionsArr, ok := versionsRaw.([]any); ok {
				var versions []flutterVersion
				for _, v := range versionsArr {
					if data, ok := v.(map[string]any); ok {
						fv, err := parseVersionFromJsonMap(data)
						if err == nil {
							if fv.installType == "" && defaultManager != "" {
								fv.installType = defaultManager
							}
							versions = append(versions, fv)
							if singleResult {
								return versions, nil
							}
						}
					}
				}
				if len(versions) > 0 {
					return versions, nil
				}
			}
		}

		fv, err := parseVersionFromJsonMap(obj)
		if err != nil {
			return nil, fmt.Errorf("parse single version from JSON object: %w", err)
		} else {
			if fv.installType == "" && defaultManager != "" {
				fv.installType = defaultManager
			}
			fv.installType = defaultManager
			return []flutterVersion{fv}, nil
		}
	}

	return nil, fmt.Errorf("input is not valid JSON object or array")
}

func parseVersionFromJsonMap(data map[string]any) (flutterVersion, error) {
	version := ""
	if v, ok := data["flutterVersion"].(string); ok && v != "" {
		version = v
	} else if v, ok := data["flutterSdkVersion"].(string); ok && v != "" {
		version = v
	} else if v, ok := data["frameworkVersion"].(string); ok && v != "" {
		version = v
	} else if t, ok := data["type"].(string); ok && t == "release" {
		if n, ok := data["name"].(string); ok && n != "" {
			version = n
		}
	}

	channel := ""
	if c, ok := data["channel"].(string); ok && c != "" {
		channel = c
	} else if c, ok := data["releaseFromChannel"].(string); ok && c != "" {
		channel = c
	} else if t, ok := data["type"].(string); ok && t == "channel" {
		if n, ok := data["name"].(string); ok && n != "" && containsString(channels, n) {
			channel = n
		}
	}
	if version == "" && channel == "" {
		return flutterVersion{}, fmt.Errorf("find flutter version and channel in JSON output")
	}

	var installType string
	if m, ok := data["flutterRoot"].(string); ok && m != "" {
		if strings.Contains(m, FVMName) {
			installType = FVMName
		} else if strings.Contains(m, ASDFName) {
			installType = ASDFName
		}
	} else if m, ok := data["binPath"].(string); ok && m != "" {
		if strings.Contains(m, FVMName) {
			installType = FVMName
		} else if strings.Contains(m, ASDFName) {
			installType = ASDFName
		}
	}

	return flutterVersion{
		version:     version,
		channel:     channel,
		installType: installType,
	}, nil
}

func containsString(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func parseVersionFromStringLines(input string, singleResult bool) ([]flutterVersion, error) {
	versionRegexp := regexp.MustCompile(`v?([0-9]+\.[0-9]+\.[0-9]+)(?:[-\.][A-Za-z0-9\.\-]+)?`)
	channelsString := strings.Join(channels, "|")
	channelRegexp := regexp.MustCompile(`(?i)(` + channelsString + `)\b`)

	defaultManager := ""
	if strings.Contains(input, FVMName) {
		defaultManager = FVMName
	} else if strings.Contains(input, ASDFName) {
		defaultManager = ASDFName
	}

	versions := []flutterVersion{}
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "dart") {
			continue
		}

		currentVersion := versionRegexp.FindString(lowerLine)
		if currentVersion != "" {
			for _, channel := range channels {
				suffix := fmt.Sprintf("-%s", channel)
				if index := strings.Index(currentVersion, suffix); index != -1 {
					currentVersion = currentVersion[:index]
				}
			}
		}

		currentChannel := channelRegexp.FindString(lowerLine)

		if currentVersion != "" || currentChannel != "" {
			versions = append(versions, flutterVersion{
				version:     currentVersion,
				channel:     currentChannel,
				installType: defaultManager,
			})
			if singleResult {
				return versions, nil
			}
		}
	}
	if len(versions) == 0 {
		return versions, fmt.Errorf("parse flutter version and channel from input")
	}

	return versions, nil
}

func parseProjectConfigFiles() (flutterVersion, error) {
	proj, err := flutterproject.New("./", fileutil.NewFileManager(), pathutil.NewPathChecker(), fluttersdk.NewSDKVersionFinder())
	if err != nil {
		return flutterVersion{}, fmt.Errorf("open project: %s", err)
	}
	sdkVersions, err := proj.FlutterAndDartSDKVersions()
	if err != nil {
		return flutterVersion{}, fmt.Errorf("get Flutter and Dart SDK versions: %s", err)
	}
	stepTracker := tracker.NewStepTracker(logv2.NewLogger(), env.NewRepository())
	stepTracker.LogSDKVersions(sdkVersions)
	defer stepTracker.Wait()

	if sdkVersions.PubspecFlutterVersion != nil {
		return flutterVersion{
			version: sdkVersions.PubspecFlutterVersion.String(),
		}, nil
	} else if sdkVersions.FVMFlutterVersion != nil {
		var channel string
		if sdkVersions.FVMFlutterChannel != "" {
			channel = sdkVersions.FVMFlutterChannel
		}
		return flutterVersion{
			version: sdkVersions.FVMFlutterVersion.String(),
			channel: channel,
		}, nil
	} else if sdkVersions.ASDFFlutterVersion != nil {
		var channel string
		if sdkVersions.ASDFFlutterChannel != "" {
			channel = sdkVersions.ASDFFlutterChannel
		}
		return flutterVersion{
			version: sdkVersions.ASDFFlutterVersion.String(),
			channel: channel,
		}, nil
	}
	return flutterVersion{}, fmt.Errorf("no Flutter version found in the project files")
}
