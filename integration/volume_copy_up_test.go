/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/integration/images"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	containerUserName = "ContainerUser"
	// containerUserSID is a well known SID that is set on the
	// ContainerUser username inside a Windows container.
	containerUserSID = "S-1-5-93-2-2"
)

type volumeFile struct {
	fileName string
	contents string
}

type containerVolume struct {
	containerPath string
	files         []volumeFile
}

func TestVolumeCopyUp(t *testing.T) {
	var (
		testImage   = images.Get(images.VolumeCopyUp)
		execTimeout = time.Minute
	)

	t.Logf("Create a sandbox")
	sb, sbConfig := PodSandboxConfigWithCleanup(t, "sandbox", "volume-copy-up")

	EnsureImageExists(t, testImage)

	t.Logf("Create a container with volume-copy-up test image")
	cnConfig := ContainerConfig(
		"container",
		testImage,
		WithCommand("sleep", "150"),
	)
	cn, err := runtimeService.CreateContainer(sb, cnConfig, sbConfig)
	require.NoError(t, err)

	t.Logf("Start the container")
	require.NoError(t, runtimeService.StartContainer(cn))

	expectedVolumes := []containerVolume{
		{
			containerPath: "/test_dir",
			files: []volumeFile{
				{
					fileName: "test_file",
					contents: "test_content\n",
				},
			},
		},
		{
			containerPath: "/:colon_prefixed",
			files: []volumeFile{
				{
					fileName: "colon_prefixed_file",
					contents: "test_content\n",
				},
			},
		},
		{
			containerPath: "/C:/weird_test_dir",
			files: []volumeFile{
				{
					fileName: "weird_test_file",
					contents: "test_content\n",
				},
			},
		},
	}

	if runtime.GOOS == "windows" {
		expectedVolumes = []containerVolume{
			{
				containerPath: "C:\\test_dir",
				files: []volumeFile{
					{
						fileName: "test_file",
						contents: "test_content\n",
					},
				},
			},
			{
				containerPath: "D:",
				files:         []volumeFile{},
			},
		}
	}

	// ghcr.io/containerd/volume-copy-up:2.2 contains 3 volumes on Linux and 2 volumes on Windows.
	// On linux, each of the volumes contains a single file, all with the same conrent. On Windows,
	// non C volumes defined in the image start out as empty.
	for _, vol := range expectedVolumes {
		for _, file := range vol.files {
			t.Logf("Check whether volume %s contains the test file %s", vol.containerPath, file.fileName)
			stdout, stderr, err := runtimeService.ExecSync(cn, []string{
				"cat",
				filepath.ToSlash(filepath.Join(vol.containerPath, file.fileName)),
			}, execTimeout)
			require.NoError(t, err)
			assert.Empty(t, stderr)
			assert.Equal(t, file.contents, string(stdout))
		}
	}

	volumeMappings, err := getContainerVolumes(t, *criRoot, cn)
	require.NoError(t, err)
	t.Logf("Check host path of the volume")
	assert.Equalf(t, len(expectedVolumes), len(volumeMappings), "expected exactly %d volume(s)", len(expectedVolumes))

	testFilePath := filepath.Join(volumeMappings[expectedVolumes[0].containerPath], expectedVolumes[0].files[0].fileName)
	inContainerPath := filepath.Join(expectedVolumes[0].containerPath, expectedVolumes[0].files[0].fileName)
	contents, err := os.ReadFile(testFilePath)
	require.NoError(t, err)
	assert.Equal(t, "test_content\n", string(contents))

	t.Logf("Update volume from inside the container")
	_, _, err = runtimeService.ExecSync(cn, []string{
		"sh",
		"-c",
		fmt.Sprintf("echo new_content > %s", filepath.ToSlash(inContainerPath)),
	}, execTimeout)
	require.NoError(t, err)

	t.Logf("Check whether host path of the volume is updated")
	contents, err = os.ReadFile(testFilePath)
	require.NoError(t, err)
	assert.Equal(t, "new_content\n", string(contents))
}

func TestVolumeOwnership(t *testing.T) {
	var (
		testImage   = images.Get(images.VolumeOwnership)
		execTimeout = time.Minute
	)

	t.Logf("Create a sandbox")
	sb, sbConfig := PodSandboxConfigWithCleanup(t, "sandbox", "volume-ownership")

	EnsureImageExists(t, testImage)

	t.Logf("Create a container with volume-ownership test image")
	cnConfig := ContainerConfig(
		"container",
		testImage,
		WithCommand("sleep", "150"),
	)
	cn, err := runtimeService.CreateContainer(sb, cnConfig, sbConfig)
	require.NoError(t, err)

	t.Logf("Start the container")
	require.NoError(t, runtimeService.StartContainer(cn))

	// ghcr.io/containerd/volume-ownership:2.1 contains a test_dir
	// volume, which is owned by 65534:65534 (nobody:nogroup, or nobody:nobody).
	// On Windows, the folder is situated in C:\volumes\test_dir and is owned
	// by ContainerUser (SID: S-1-5-93-2-2). A helper tool get_owner.exe should
	// exist inside the container that returns the owner in the form of USERNAME:SID.
	t.Logf("Check ownership of test directory inside container")

	cmd := []string{
		"stat", "-c", "%u:%g", "/test_dir",
	}
	expectedContainerOutput := "65534:65534\n"
	expectedHostOutput := "65534:65534\n"
	if runtime.GOOS == "windows" {
		cmd = []string{
			"C:\\bin\\get_owner.exe",
			"C:\\volumes\\test_dir",
		}
		expectedContainerOutput = fmt.Sprintf("%s:%s", containerUserName, containerUserSID)
		// The username is unknown on the host, but we can still get the SID.
		expectedHostOutput = containerUserSID
	}
	stdout, stderr, err := runtimeService.ExecSync(cn, cmd, execTimeout)
	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.Equal(t, expectedContainerOutput, string(stdout))

	t.Logf("Check ownership of test directory on the host")
	volumePaths, err := getHostPathForVolumes(*criRoot, cn)
	require.NoError(t, err)
	assert.Equal(t, len(volumePaths), 1, "expected exactly 1 volume")

	output, err := getOwnership(volumePaths[0])
	require.NoError(t, err)
	assert.Equal(t, expectedHostOutput, output)
}

func getContainerVolumes(t *testing.T, criRoot, containerID string) (map[string]string, error) {
	client, err := RawRuntimeClient()
	require.NoError(t, err, "failed to get raw grpc runtime service client")
	request := &v1.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	}
	response, err := client.ContainerStatus(context.TODO(), request)
	require.NoError(t, err)
	ret := make(map[string]string)

	mounts := struct {
		RuntimeSpec struct {
			Mounts []specs.Mount `json:"mounts"`
		} `json:"runtimeSpec"`
	}{}

	info := response.Info["info"]
	err = json.Unmarshal([]byte(info), &mounts)
	require.NoError(t, err)
	containerVolumesHostPath := filepath.Join(criRoot, "containers", containerID, "volumes")
	for _, mount := range mounts.RuntimeSpec.Mounts {
		norm, err := getFinalPath(mount.Source)
		require.NoError(t, err)
		if strings.HasPrefix(norm, containerVolumesHostPath) {
			ret[mount.Destination] = norm
		}
	}
	return ret, nil
}

func getHostPathForVolumes(criRoot, containerID string) ([]string, error) {
	hostPath := filepath.Join(criRoot, "containers", containerID, "volumes")
	if _, err := os.Stat(hostPath); err != nil {
		return nil, err
	}

	volumes, err := os.ReadDir(hostPath)
	if err != nil {
		return nil, err
	}

	if len(volumes) == 0 {
		return []string{}, nil
	}

	volumePaths := make([]string, len(volumes))
	for idx, volume := range volumes {
		volumePaths[idx] = filepath.Join(hostPath, volume.Name())
	}

	return volumePaths, nil
}
