package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-flutter/fluttersdk"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-steplib/bitrise-step-flutter-installer/tracker"
)

// Channels represents the available Flutter channels.
var Channels = []string{
	"stable",
	"beta",
	"dev",
	"main",
	"master",
}

const flutterVersionRegexp = `v?([0-9]+\.[0-9]+\.[0-9]+)(?:[-\.][A-Za-z0-9\.\-]+)?`

type flutterVersion struct {
	version string
	channel string
	// installType indicates the tool used to install the Flutter version, e.g., "fvm", "asdf" parsed from version output.
	installType string
}

// NewFlutterVersion creates a new flutterVersion from the input string.
//
// It is capable of parsing both JSON formatted input and plain text lines.
func NewFlutterVersion(input string) (flutterVersion, error) {
	if versions, err := parseVersionsFromJson(input, true); err == nil && len(versions) > 0 {
		return versions[0], nil
	}

	// If the input is not JSON, try to parse it as plain text lines.
	if versions, err := parseVersionFromStringLines(input, true); err == nil && len(versions) > 0 {
		return versions[0], nil
	}

	return flutterVersion{}, fmt.Errorf("parse flutter version and channel from input: %s", input)
}

// NewFlutterVersionList creates a list of flutterVersion from the input string.
//
// It is capable of parsing both JSON formatted input and plain text lines.
// It is optimized for fvm and asdf outputs, but can handle other formats as well.
func NewFlutterVersionList(input string) ([]flutterVersion, error) {
	if versions, err := parseVersionsFromJson(input, false); err == nil && len(versions) > 0 {
		return versions, nil
	}

	// If the input is not JSON, try to parse it as plain text lines.
	if versions, err := parseVersionFromStringLines(input, false); err == nil && len(versions) > 0 {
		return versions, nil
	}

	return []flutterVersion{}, fmt.Errorf("parse flutter version and channel from input: %s", input)
}

// NewFlutterVersionFromCurrent retrieves the current Flutter version using the `flutter --version --machine` command.
func (f *FlutterInstaller) NewFlutterVersionFromCurrent() (flutterVersion, error) {
	versionCmd := f.CmdFactory.Create("flutter", []string{"--version", "--machine"}, nil)
	f.Donef("$ %s", versionCmd.PrintableCommandArgs())
	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return flutterVersion{}, fmt.Errorf("get flutter version: %s %s", err, out)
	}
	f.Debugf("Flutter version output: %s", out)

	flutterVer, err := NewFlutterVersion(out)
	f.Debugf("Current Flutter: %s", f.NewVersionString(flutterVer))

	return flutterVer, err
}

// NewFlutterVersionFromInputAndProject retrieves the Flutter version from the input or project configuration files.
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

// NewVersionString formats the flutterVersion into a human-readable string.
func (f *FlutterInstaller) NewVersionString(version flutterVersion) string {
	versionString := version.version

	if versionString != "" {
		if version.channel != "" {
			versionString += "(" + version.channel + ")"
		}
		return versionString
	}

	if version.channel != "" {
		return version.channel
	}

	return "unknown"
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
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return nil, fmt.Errorf("input is not valid JSON object or array")
	}

	if versionsRaw, ok := obj["versions"]; ok {
		// fvm API returns versions as an array.
		if versionsArr, ok := versionsRaw.([]any); ok {
			var versions []flutterVersion
			for _, v := range versionsArr {
				data, ok := v.(map[string]any)
				if !ok {
					continue
				}

				fv, err := parseVersionFromJsonMap(data)
				if err != nil {
					continue
				}

				if fv.installType == "" && defaultManager != "" {
					fv.installType = defaultManager
				}

				if singleResult {
					return []flutterVersion{fv}, nil
				}

				versions = append(versions, fv)

			}
			if len(versions) > 0 {
				return versions, nil
			}
		}
	}

	// If the input is a single JSON object, parse it directly.
	fv, err := parseVersionFromJsonMap(obj)
	if err != nil {
		return nil, fmt.Errorf("parse single version from JSON object: %w", err)
	}

	if fv.installType == "" && defaultManager != "" {
		fv.installType = defaultManager
	}
	fv.installType = defaultManager
	return []flutterVersion{fv}, nil
}

// parseVersionFromJsonMap extracts the Flutter version and channel from a JSON map.
//
// It looks for specific keys handling fvm API and flutter --version --machine output formats.
func parseVersionFromJsonMap(data map[string]any) (flutterVersion, error) {
	versionKeys := []string{"flutterVersion", "flutterSdkVersion", "frameworkVersion"}
	version := ""
	for _, key := range versionKeys {
		version = extractVersion(&data, key)
		if version != "" {
			break
		}
	}
	if version == "" {
		// Special case: if type == "release", check "name"
		if t, ok := data["type"].(string); ok && t == "release" {
			version = extractVersion(&data, "name")
		}
	}

	channelKeys := []string{"channel", "releaseFromChannel"}
	channel := ""
	for _, key := range channelKeys {
		channel = extractChannel(&data, key)
		if channel != "" {
			break
		}
	}
	if channel == "" {
		// Special case: if type == "channel", check "name"
		if t, ok := data["type"].(string); ok && t == "channel" {
			channel = extractChannel(&data, "name")
		}
	}

	if version == "" && channel == "" {
		return flutterVersion{}, fmt.Errorf("find flutter version and channel in JSON output")
	}

	// Determine the install type based on the presence of 'fvm' or 'asdf' in the paths
	var installType string
	if it := extractRoot(&data, "flutterRoot"); it != "" {
		installType = it
	} else if it = extractRoot(&data, "binPath"); it != "" {
		installType = it
	}

	return flutterVersion{
		version:     version,
		channel:     channel,
		installType: installType,
	}, nil
}

func extractVersion(data *map[string]any, key string) string {
	if v, ok := (*data)[key].(string); ok {
		v = strings.TrimSpace(v)
		v = strings.ToLower(v)
		if v != "" && regexp.MustCompile(flutterVersionRegexp).MatchString(v) {
			return v
		}
	}
	return ""
}

func extractChannel(data *map[string]any, key string) string {
	if c, ok := (*data)[key].(string); ok {
		c = strings.TrimSpace(c)
		c = strings.ToLower(c)
		if c != "" && slices.Contains(Channels, c) {
			return c
		}
	}
	return ""
}

func extractRoot(data *map[string]any, key string) string {
	if m, ok := (*data)[key].(string); ok {
		if strings.Contains(m, FVMName) {
			return FVMName
		} else if strings.Contains(m, ASDFName) {
			return ASDFName
		}
	}
	return ""
}

// parseVersionFromStringLines extracts Flutter versions and channels from plain text lines.
//
// It uses regular expressions to find versions and channels in the input string.
func parseVersionFromStringLines(input string, singleResult bool) ([]flutterVersion, error) {
	versionRegexp := regexp.MustCompile(flutterVersionRegexp)
	channelsString := strings.Join(Channels, "|")
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

		currentChannel := channelRegexp.FindString(lowerLine)
		currentVersion := versionRegexp.FindString(lowerLine)
		if currentVersion != "" && currentChannel != "" {
			suffix := fmt.Sprintf("-%s", currentChannel)
			if index := strings.Index(currentVersion, suffix); index != -1 {
				// If the version contains the channel, remove it from the version string.
				currentVersion = currentVersion[:index]
			}
		}

		if currentVersion == "" && currentChannel == "" {
			continue
		}

		versions = append(versions, flutterVersion{
			version:     currentVersion,
			channel:     currentChannel,
			installType: defaultManager,
		})
		if singleResult {
			return versions, nil
		}

	}
	if len(versions) == 0 {
		return versions, fmt.Errorf("parse flutter version and channel from input")
	}

	return versions, nil
}

// parseProjectConfigFiles retrieves the Flutter version from the project configuration files.
//
// It checks for versions in pubspec.yaml, fvm, and asdf configurations.
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
	}

	if sdkVersions.FVMFlutterVersion != nil {
		var channel string
		if sdkVersions.FVMFlutterChannel != "" {
			channel = sdkVersions.FVMFlutterChannel
		}
		return flutterVersion{
			version: sdkVersions.FVMFlutterVersion.String(),
			channel: channel,
		}, nil
	}

	if sdkVersions.ASDFFlutterVersion != nil {
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
