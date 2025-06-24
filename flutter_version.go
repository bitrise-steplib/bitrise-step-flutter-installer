package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
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
	installType *FlutterInstallType
}

func NewFlutterVersion(input string) (flutterVersion, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(input), &data); err == nil {
		// JSON output from `flutter --version --machine`
		version := ""
		channel := ""
		var installType *FlutterInstallType
		if v, ok := data["frameworkVersion"].(string); ok && v != "" {
			version = v
		} else if v, ok := data["flutterVersion"].(string); ok && v != "" {
			version = v
		}
		if c, ok := data["channel"].(string); ok && c != "" {
			channel = c
		}
		if version == "" && channel == "" {
			return flutterVersion{}, fmt.Errorf("find flutter version and channel in JSON output")
		}
		if m, ok := data["flutterRoot"].(string); ok && m != "" {
			if strings.Contains(m, FlutterInstallTypeFVM.Name) {
				installType = &FlutterInstallTypeFVM
			} else if strings.Contains(m, FlutterInstallTypeAsdf.Name) {
				installType = &FlutterInstallTypeAsdf
			}
		}

		return flutterVersion{
			version:     version,
			channel:     channel,
			installType: installType,
		}, nil
	}

	return newFlutterVersionFromString(input)
}

func newFlutterVersionFromString(input string) (flutterVersion, error) {
	versionRegexp := regexp.MustCompile(`v?([0-9]+\.[0-9]+\.[0-9]+)(?:[-\.][A-Za-z0-9\.\-]+)?`)
	channelsString := strings.Join(channels, "|")
	channelRegexp := regexp.MustCompile(`(?i)\b(` + channelsString + `)\b`)

	var version, channel string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "dart") {
			continue
		}
		if version == "" {
			match := versionRegexp.FindString(line)
			if match != "" {
				match = strings.TrimPrefix(match, "v")
				for _, channel := range channels {
					suffix := fmt.Sprintf("-%s", channel)
					if index := strings.Index(match, suffix); index != -1 {
						match = match[:index]
					}
				}
				version = match
			}
		}
		if channel == "" && channelRegexp.MatchString(line) {
			c := channelRegexp.FindString(line)
			if c != "" {
				channel = strings.ToLower(c)
			}
		}
		if version != "" && channel != "" {
			break
		}
	}
	if version == "" && channel == "" {
		return flutterVersion{}, fmt.Errorf("parse flutter version and channel from input")
	}

	return flutterVersion{
		channel: channel,
		version: version,
	}, nil
}
