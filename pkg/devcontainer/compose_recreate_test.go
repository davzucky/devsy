package devcontainer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/devsy-org/devsy/pkg/compose"
	"github.com/devsy-org/devsy/pkg/devcontainer/config"
	"github.com/devsy-org/devsy/pkg/driver"
	"github.com/devsy-org/devsy/pkg/provider"
	"github.com/stretchr/testify/require"
)

type composeRecreateImageBackend struct {
	driver.ImageBackend
	inspectErr error
}

func (b composeRecreateImageBackend) InspectImage(
	context.Context,
	string,
) (*config.ImageDetails, error) {
	return nil, b.inspectErr
}

func TestStartComposeContainerRecreateRefreshesOverrides(t *testing.T) {
	const serviceName = "development"
	for _, reset := range []bool{false, true} {
		name := "recreate"
		if reset {
			name = "reset and recreate"
		}
		t.Run(name, func(t *testing.T) {
			stale := filepath.Join(t.TempDir(), FeaturesStartOverrideFilePrefix+"-stale.yml")
			require.NoError(t, os.WriteFile(stale, []byte("stale lifecycle metadata"), 0o600))
			d := &mockDriver{stopErr: errors.New("stale overrides reused")}
			r := newTestRunner(d)
			inspectErr := errors.New("fresh build selected")
			r.imageBackend = composeRecreateImageBackend{inspectErr: inspectErr}

			_, err := r.startContainer(context.Background(), &startContainerParams{
				parsedConfig: &config.SubstitutedConfig{Config: &config.DevContainerConfig{
					ComposeContainer: config.ComposeContainer{Service: serviceName},
				}},
				project: &composetypes.Project{
					Name: "recreate-test",
					Services: composetypes.Services{
						serviceName: {Name: serviceName, Image: testImage},
					},
				},
				composeHelper: &compose.ComposeHelper{},
				container: &config.ContainerDetails{
					ID: testContainerID,
					Config: config.ContainerDetailsConfig{
						Labels: map[string]string{ConfigFilesLabel: stale},
					},
				},
				options: UpOptions{
					CLIOptions: provider.CLIOptions{Recreate: true, Reset: reset, Pull: true},
				},
			})

			require.ErrorIs(t, err, inspectErr)
			require.False(t, d.stopCalled, "a failed rebuild must preserve the existing container")
			require.False(t, d.deleteCalled)
		})
	}
}
