package main

import (
	"fmt"
	"regexp"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
)

type flutterVersion struct {
	version string
	channel string
}

func flutterVersionInfo() (flutterVersion, string, error) {
	versionCmd := command.New("flutter", "--version")
	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()
	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		if errorutil.IsExitStatusError(err) {
			return flutterVersion{}, out, fmt.Errorf("failed to get flutter version, error: %s, out: %s", err, out)
		}
		return flutterVersion{}, "", fmt.Errorf("failed to get flutter version, error: %s", err)
	}

	channel, err := matchChannel(out)
	if err != nil {
		return flutterVersion{}, out, err
	}

	version, err := matchVersion(out)
	if err != nil {
		return flutterVersion{channel: channel}, out, err
	}

	return flutterVersion{
		channel: channel,
		version: version,
	}, out, nil
}

func matchVersion(versionOutput string) (string, error) {
	versionRegexp := regexp.MustCompile(`(?im)^Flutter\s+(\S+?)\s+`)
	submatches := versionRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter version")
	}
	return submatches[1], nil
}

func matchChannel(versionOutput string) (string, error) {
	channelRegexp := regexp.MustCompile(`(?im)\s+channel\s+(\S+?)\s+`)
	submatches := channelRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter channel")
	}
	return submatches[1], nil
}
