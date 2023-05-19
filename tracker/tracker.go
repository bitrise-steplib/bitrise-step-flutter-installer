package tracker

import (
	"github.com/bitrise-io/go-utils/v2/analytics"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-steplib/bitrise-step-flutter-installer/flutterproject"
)

type StepTracker struct {
	tracker analytics.Tracker
	logger  log.Logger
}

func NewStepTracker(logger log.Logger, envRepo env.Repository) StepTracker {
	p := analytics.Properties{
		"app_slug":   envRepo.Get("BITRISE_APP_SLUG"),
		"build_slug": envRepo.Get("BITRISE_BUILD_SLUG"),
	}
	return StepTracker{
		tracker: analytics.NewDefaultTracker(logger, p),
		logger:  logger,
	}
}

func (t *StepTracker) LogSDKVersions(projectSDKVersions flutterproject.FlutterAndDartSDKVersions) {
	p := analytics.Properties{}
	for _, flutterSDK := range projectSDKVersions.FlutterSDKVersions {
		key := "flutter_sdk_" + string(flutterSDK.Source)
		value := ""

		if flutterSDK.Version != nil {
			value = flutterSDK.Version.String()
		} else if flutterSDK.Constraint != nil {
			value = flutterSDK.Constraint.String()
		}

		if value != "" {
			p[key] = value
		}
	}

	for _, dartSDK := range projectSDKVersions.DartSDKVersions {
		key := "dart_sdk_" + string(dartSDK.Source)
		value := ""

		if dartSDK.Version != nil {
			value = dartSDK.Version.String()
		} else if dartSDK.Constraint != nil {
			value = dartSDK.Constraint.String()
		}

		if value != "" {
			p[key] = value
		}
	}

	t.tracker.Enqueue("flutter_scanner_project_sdk_versions", p)
}

func (t *StepTracker) Wait() {
	t.tracker.Wait()
}
