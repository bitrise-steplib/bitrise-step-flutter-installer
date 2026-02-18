# Flutter Install

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/bitrise-step-flutter-installer?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/bitrise-step-flutter-installer/releases)

Install Flutter SDK.

<details>
<summary>Description</summary>

This Step installs the Flutter SDK using FVM, ASDF, or manual installation methods and runs the initial setup.
Use this step *before* the cache-pull step to make sure caching works correctly.

### Configuring the Step
1. In the **Flutter SDK version or bundle URL** input set the version, tag, branch, or bundle URL. The default value is `stable`.
2. Enable **Print debug information** to run `flutter doctor` to see if there are any missing platform dependencies for setting up Flutter.

### Troubleshooting
If you prefer to install Flutter from an installation bundle instead of the git repository, provide a Flutter SDK bundle URL in the **Flutter SDK version or bundle URL** input. For example, `https://storage.googleapis.com/flutter_infra/releases/dev/windows/flutter_windows_v1.14.5-dev.zip`. When a valid bundle URL is provided, it will download and install from that bundle instead of cloning from git.

### Useful links
- [About Flutter build release channels](https://github.com/flutter/flutter/wiki/Flutter-build-release-channels)
- [Available version tags](https://github.com/flutter/flutter/releases)
- [Available branches](https://github.com/flutter/flutter/branches)

### Related Steps
- [Flutter Test](https://www.bitrise.io/integrations/steps/flutter-test)
- [Flutter Build](https://www.bitrise.io/integrations/steps/flutter-build)
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/steps/adding-steps-to-a-workflow.html).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `version` | Specify the Flutter version using a tag, branch, channel name, or Flutter SDK bundle URL.  **Installation priority:** 1. Use FVM/ASDF if available and has the version installed 2. Install via FVM/ASDF if version is available in their releases 3. Manual installation: download from bundle URL or clone from git repository  **Examples:** - Channels: `stable`, `beta`, `dev` - Versions: `3.32.5`, `v1.6.3-beta` - Bundle URL: `https://storage.googleapis.com/flutter_infra/releases/beta/macos/flutter_macos_v1.6.3-beta.zip`  **Resources:** - [Version tags](https://github.com/flutter/flutter/releases) - [Branches](https://github.com/flutter/flutter/branches) - [Bundle URLs](https://flutter.dev/docs/development/tools/sdk/releases) |  | `stable` |
| `is_debug` | If enabled will run flutter doctor and print value of PATH eniroment variable. |  | `false` |
</details>

<details>
<summary>Outputs</summary>
There are no outputs defined in this step
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/bitrise-step-flutter-installer/pulls) and [issues](https://github.com/bitrise-steplib/bitrise-step-flutter-installer/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://docs.bitrise.io/en/bitrise-ci/bitrise-cli/running-your-first-local-build-with-the-cli.html).

Learn more about developing steps:

- [Create your own step](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/developing-your-own-bitrise-step/developing-a-new-step.html)
