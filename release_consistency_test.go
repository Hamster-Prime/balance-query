package main

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
)

const releasePluginID = "balance-query"

type releaseRegistry struct {
	Plugins []releaseRegistryPlugin `json:"plugins"`
}

type releaseRegistryPlugin struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Install struct {
		Artifacts []releaseArtifact `json:"artifacts"`
	} `json:"install"`
}

type releaseArtifact struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func TestReleaseVersionConsistency(t *testing.T) {
	if strings.TrimSpace(pluginVersion) == "" {
		t.Fatal("pluginVersion must not be empty")
	}
	if expected := strings.TrimSpace(os.Getenv("BALANCE_QUERY_EXPECT_VERSION")); expected != "" && pluginVersion != expected {
		t.Fatalf("pluginVersion = %q, want release tag version %q", pluginVersion, expected)
	}

	legacyPlugin := readReleaseRegistryPlugin(t, "registry.json")
	plugin := readReleaseRegistryPlugin(t, "registry-v2.json")
	if legacyPlugin.Version != plugin.Version {
		t.Fatalf("registry versions differ: registry.json=%q registry-v2.json=%q", legacyPlugin.Version, plugin.Version)
	}
	expectedPlatforms := map[string]bool{
		"darwin/amd64":  false,
		"darwin/arm64":  false,
		"linux/amd64":   false,
		"linux/arm64":   false,
		"windows/amd64": false,
		"windows/arm64": false,
	}
	if len(plugin.Install.Artifacts) != len(expectedPlatforms) {
		t.Fatalf("registry-v2.json contains %d artifacts, want %d", len(plugin.Install.Artifacts), len(expectedPlatforms))
	}
	for _, artifact := range plugin.Install.Artifacts {
		platform := artifact.GOOS + "/" + artifact.GOARCH
		t.Run(platform, func(t *testing.T) {
			seen, expected := expectedPlatforms[platform]
			if !expected {
				t.Fatalf("unexpected release platform %q", platform)
			}
			if seen {
				t.Fatalf("duplicate release platform %q", platform)
			}
			expectedPlatforms[platform] = true
			assertArtifactURLVersion(t, artifact, plugin.Version)
			if artifact.Size <= 0 {
				t.Errorf("artifact size = %d, want a positive value", artifact.Size)
			}
			checksum, err := hex.DecodeString(artifact.SHA256)
			if err != nil || len(checksum) != 32 {
				t.Errorf("artifact sha256 = %q, want 64 hexadecimal characters", artifact.SHA256)
			}
		})
	}
}

func readReleaseRegistryPlugin(t *testing.T, filename string) releaseRegistryPlugin {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	var registry releaseRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatalf("decode %s: %v", filename, err)
	}

	var matches []releaseRegistryPlugin
	for _, plugin := range registry.Plugins {
		if plugin.ID == releasePluginID {
			matches = append(matches, plugin)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("%s contains %d plugins with id %q, want exactly 1", filename, len(matches), releasePluginID)
	}
	return matches[0]
}

func assertArtifactURLVersion(t *testing.T, artifact releaseArtifact, version string) {
	t.Helper()
	if artifact.GOOS == "" || artifact.GOARCH == "" {
		t.Fatalf("artifact platform is incomplete: goos=%q goarch=%q", artifact.GOOS, artifact.GOARCH)
	}
	parsed, err := url.Parse(artifact.URL)
	if err != nil {
		t.Fatalf("parse artifact URL %q: %v", artifact.URL, err)
	}

	tagPath := "/releases/download/v" + version + "/"
	if !strings.Contains(parsed.Path, tagPath) {
		t.Errorf("artifact URL path %q must contain release tag path %q", parsed.Path, tagPath)
	}
	expectedArchive := releasePluginID + "_" + version + "_" + artifact.GOOS + "_" + artifact.GOARCH + ".zip"
	if archive := path.Base(parsed.Path); archive != expectedArchive {
		t.Errorf("artifact archive = %q, want %q", archive, expectedArchive)
	}
}
