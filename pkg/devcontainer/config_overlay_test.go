package devcontainer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devsy-org/devsy/pkg/devcontainer/config"
	"github.com/devsy-org/devsy/pkg/devcontainer/feature"
	"github.com/devsy-org/devsy/pkg/provider"
	"github.com/stretchr/testify/require"
)

const (
	overlayBaseConfigPath    = "base.json"
	overlayFeatureVersionKey = "version"
)

func TestGetSubstitutedConfigOverlayFeatures(t *testing.T) {
	for _, base := range []struct {
		name string
		kind string
	}{
		{name: "image", kind: `"image":"alpine:latest"`},
		{name: "dockerfile", kind: `"build":{"dockerfile":"Dockerfile"}`},
		{name: "compose", kind: `"dockerComposeFile":"compose.yml","service":"development"`},
	} {
		t.Run(base.name, func(t *testing.T) {
			folder := t.TempDir()
			origin := filepath.Join(folder, overlayBaseConfigPath)
			body := `{` + base.kind + `,"features":{"keep":{},"replace":{"version":"base"}}}`
			require.NoError(t, os.WriteFile(origin, []byte(body), 0o600))
			overlay := filepath.Join(folder, "overlay.json")
			require.NoError(t, os.WriteFile(overlay, []byte(`{
				"features":{"added":{},"replace":{"version":"${localEnv:OVERLAY_VERSION}"}}
			}`), 0o600))
			r := newRunnerAt(folder)
			opts := provider.CLIOptions{
				DevContainerPath: overlayBaseConfigPath, ExtraDevContainerPath: overlay,
				InitEnv: []string{"OVERLAY_VERSION=overlay"},
			}

			parsed, _, err := r.getSubstitutedConfig(opts)
			require.NoError(t, err)
			require.Equal(t, map[string]any{
				"keep": map[string]any{}, "added": map[string]any{},
				"replace": map[string]any{overlayFeatureVersionKey: "overlay"},
			}, parsed.Config.Features)
			require.Equal(t, origin, parsed.Config.Origin)
			require.Equal(
				t,
				"${localEnv:OVERLAY_VERSION}",
				parsed.Raw.Features["replace"].(map[string]any)[overlayFeatureVersionKey],
			)

			opts.AdditionalFeatures = `{"replace":{"version":"cli"}}`
			parsed, _, err = r.getSubstitutedConfig(opts)
			require.NoError(t, err)
			require.Len(t, parsed.Config.Features, 3)
			require.Equal(
				t,
				map[string]any{overlayFeatureVersionKey: "cli"},
				parsed.Config.Features["replace"],
			)
		})
	}
}

func TestGetSubstitutedConfigOverlayOnlyFeatures(t *testing.T) {
	folder := t.TempDir()
	seedConfigAt(t, folder, overlayBaseConfigPath)
	overlay := filepath.Join(folder, "overlay.json")
	require.NoError(t, os.WriteFile(overlay, []byte(`{"features":{"added":{}}}`), 0o600))
	parsed, _, err := newRunnerAt(folder).getSubstitutedConfig(provider.CLIOptions{
		DevContainerPath: overlayBaseConfigPath, ExtraDevContainerPath: overlay,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"added": map[string]any{}}, parsed.Config.Features)
	require.Equal(t, parsed.Config.Features, parsed.Raw.Features)
}

func TestGetSubstitutedConfigOverlayLocalFeatures(t *testing.T) {
	folder := t.TempDir()
	baseDir := filepath.Join(folder, "base")
	overlayDir := filepath.Join(folder, "overlay")
	require.NoError(t, os.MkdirAll(baseDir, 0o750))
	require.NoError(t, os.MkdirAll(overlayDir, 0o750))
	basePath := filepath.Join(baseDir, "devcontainer.json")
	require.NoError(t, os.WriteFile(basePath, []byte(`{
		"image":"alpine:latest", "features":{"./base-feature":{}}
	}`), 0o600))
	overlayPath := filepath.Join(overlayDir, "devcontainer.json")
	require.NoError(
		t,
		os.WriteFile(overlayPath, []byte(`{"features":{"./overlay-feature":{}}}`), 0o600),
	)

	parsed, _, err := newRunnerAt(folder).getSubstitutedConfig(provider.CLIOptions{
		DevContainerPath: "base/devcontainer.json", ExtraDevContainerPath: overlayPath,
	})
	require.NoError(t, err)
	require.Len(t, parsed.Config.Features, 2)
	require.Contains(t, parsed.Config.Features, "./base-feature")
	var resolvedPaths []string
	for id := range parsed.Config.Features {
		resolved, err := feature.ProcessFeatureID(id, parsed.Config, false)
		require.NoError(t, err)
		resolvedPaths = append(resolvedPaths, resolved)
	}
	require.ElementsMatch(t, []string{
		filepath.Join(baseDir, "base-feature"), filepath.Join(overlayDir, "overlay-feature"),
	}, resolvedPaths)
	require.Equal(t, basePath, parsed.Config.Origin)
}

func TestGetSubstitutedConfigInvalidOverlay(t *testing.T) {
	for _, invalid := range []struct {
		name string
		body string
	}{
		{name: "missing"},
		{name: "malformed", body: `{invalid`},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			folder := t.TempDir()
			seedConfigAt(t, folder, overlayBaseConfigPath)
			overlay := filepath.Join(folder, "overlay.json")
			if invalid.body != "" {
				require.NoError(t, os.WriteFile(overlay, []byte(invalid.body), 0o600))
			}
			_, _, err := newRunnerAt(folder).getSubstitutedConfig(provider.CLIOptions{
				DevContainerPath: overlayBaseConfigPath, ExtraDevContainerPath: overlay,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "overlay")
		})
	}
}

func TestGetSubstitutedConfigOverlayWithoutFeatures(t *testing.T) {
	folder := t.TempDir()
	seedConfigAt(t, folder, overlayBaseConfigPath)
	overlay := filepath.Join(folder, "overlay.json")
	require.NoError(t, os.WriteFile(overlay, []byte(`{"remoteEnv":{"EXTRA":"value"}}`), 0o600))
	parsed, _, err := newRunnerAt(folder).getSubstitutedConfig(provider.CLIOptions{
		DevContainerPath: overlayBaseConfigPath, ExtraDevContainerPath: overlay,
	})
	require.NoError(t, err)
	require.Nil(t, parsed.Config.Features)
	require.Equal(t, "seed", parsed.Config.Image)
	require.Nil(t, parsed.Config.RemoteEnv)

	merged, err := mergeImageMetadataConfig(
		t.Context(),
		parsed,
		&config.ImageMetadataConfig{},
		overlay,
	)
	require.NoError(t, err)
	require.Equal(t, "value", *merged.RemoteEnv["EXTRA"])
}
