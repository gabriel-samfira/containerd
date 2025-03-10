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

package plugin

import (
	"errors"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/snapshots/blockfile"
)

// Config represents configuration for the native plugin.
type Config struct {
	// Root directory for the plugin
	RootPath string `toml:"root_path"`

	// ScratchFile is the scratch block file to use as an empty block
	ScratchFile string `toml:"scratch_file"`

	// FSType is the filesystem type for the mount
	FSType string `toml:"fs_type"`
}

func init() {
	plugin.Register(&plugin.Registration{
		Type:   plugin.SnapshotPlugin,
		ID:     "blockfile",
		Config: &Config{},
		InitFn: func(ic *plugin.InitContext) (interface{}, error) {
			ic.Meta.Platforms = append(ic.Meta.Platforms, platforms.DefaultSpec())

			config, ok := ic.Config.(*Config)
			if !ok {
				return nil, errors.New("invalid blockfile configuration")
			}

			var opts []blockfile.Opt
			root := ic.Root
			if len(config.RootPath) != 0 {
				root = config.RootPath
			}
			if config.ScratchFile != "" {
				opts = append(opts, blockfile.WithScratchFile(config.ScratchFile))
			}
			if config.FSType != "" {
				opts = append(opts, blockfile.WithFSType(config.FSType))
			}

			return blockfile.NewSnapshotter(root, opts...)
		},
	})
}
