// Copyright CERN.
//
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package cvmfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"text/template"

	_ "embed"

	"github.com/cernops/cvmfs-csi/internal"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:embed cvmfsconf.go.tpl
var localConfTemplateStr string
var localConfTemplate = template.Must(template.New("default.local").Parse(localConfTemplateStr))

func (d *Driver) BasicSetup() error {
	log := internal.GetLogger("BasicSetup")

	// we need the cache folder before we can mount anything
	if err := mkdir(d.config.CacheFolder); err != nil {
		return fmt.Errorf("cannot create cache root folder %s: %w", d.config.CacheFolder, err)
	}

	// The config repository needs to be mounted before any other
	configPath := CVMFSConfigRepo.getMountPath()
	if err := mkdir(configPath); err != nil {
		return fmt.Errorf("cannot create config repository folder %s: %w", CVMFSConfigRepo, err)
	}

	mounted, err := folderIsMounted(configPath)
	if err != nil {
		return fmt.Errorf("cannot check if config folder is mounted: %w", err)
	}

	if mounted {
		log.Debug().Str("path", configPath).Msg("config repository already mounted")
		return nil
	} else {
		// delete default.local
		// it contains config that will mess up our bootstrap mount
		log.Debug().Str("path", CVMFSLocalConfigFile).Msg("deleting local config file")
		os.Remove(CVMFSLocalConfigFile)

		if err := MountCVMFS(CVMFSConfigRepo); err != nil {
			return err
		}
	}

	// create default.local
	if _, err := os.Stat(CVMFSLocalConfigFile); os.IsNotExist(err) {
		log.Debug().Str("path", CVMFSLocalConfigFile).Msg("creating local config file")
		f, err := os.Create(CVMFSLocalConfigFile)
		if err != nil {
			return fmt.Errorf("cannot create local config file %s: %w", CVMFSLocalConfigFile, err)
		}
		var tpl bytes.Buffer
		wr := io.MultiWriter(f, &tpl)
		if err := localConfTemplate.Execute(wr, d.config); err != nil {
			log.Error().Err(err).Str("path", f.Name()).Msg("config file generation failed")
			return fmt.Errorf("unable to write local config file %s: %w", CVMFSLocalConfigFile, err)
		}
		log.Debug().Str("path", f.Name()).Bytes("content", tpl.Bytes()).Msg("config file written")
	}

	return nil
}

// NodeStageVolume is called to mount a volume in a 'staging' location, a folder somewhere on the node.
// This staging folder can be used by many pods simultaneously, since we mount readonly.
// This driver creates one cvmfs mount per StorageClass, which represents a unique configuration of
// repository and tag/hash.
func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("volumeid", req.GetVolumeId()).Logger()
	log.Trace().Interface("req", req).Msg("NodeStageVolume")

	// check if this request is valid for this driver
	if err := validateNodeStageVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to validate NodeStageVolumeRequest: %v", err))
	}

	if err := d.BasicSetup(); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to perform basic setup: %v", err))
	}

	/*
	 * get parameters
	 */
	log.Trace().Interface("volumecontext", req.GetVolumeContext()).Msg("parsing volumecontext")
	repository, err := RepositoryFromContext(req.GetVolumeContext())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("cannot parse volume options: %v", err))
	}
	to := repository.getMountPath()
	log = log.With().Str("to", to).Str("repository", string(repository)).Logger()

	/*
	 * mount cvmfs folder if needed
	 */
	if err := mkdir(to); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot create CVMFS folder %s: %v", to, err))
	}

	log.Trace().Msg("checking if volume is already mounted")
	mounted, err := folderIsMounted(to)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot probe if folder is already mounted %s: %v", to, err))
	}

	if mounted {
		log.Debug().Msg("volume already mounted")
	} else {
		log.Debug().Msg("mounting volume")
		err = MountCVMFS(repository)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("cannot mount volume: %v", err))
		}

		log.Info().Msg("volume mounted")
	}

	/*
	 * bind mount to requested folder
	 */
	stagingTargetPath := req.GetStagingTargetPath()
	log = log.With().Str("stagingpath", stagingTargetPath).Logger()

	err = mkdir(stagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot create staging folder %s: %v", stagingTargetPath, err))
	}

	log.Trace().Msg("checking if staging path is already mounted")
	mounted, err = folderIsMounted(stagingTargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot probe if staging folder is already mounted %s: %v", stagingTargetPath, err))
	}

	if mounted {
		log.Debug().Msg("staging path already mounted, skipping")
	} else {
		log.Debug().Msg("mounting staging path")
		err = bindMount(to, stagingTargetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("cannot mount staging path %s: %v", stagingTargetPath, err))
		}

		log.Info().Msg("volume mounted")
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	stagingTargetPath := req.GetStagingTargetPath()
	log := zerolog.Ctx(ctx).With().Str("targetpath", stagingTargetPath).Logger()
	log.Trace().Msg("NodeUnstageVolume")

	if err := validateNodeUnstageVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Errorf("failed to validate NodeUnstageVolumeRequest: %w", err).Error())
	}

	if err := Unmount(stagingTargetPath); err != nil {
		log.Warn().Err(err).Str("path", stagingTargetPath).Msg("failed to unmount")
	}

	if err := os.Remove(stagingTargetPath); err != nil {
		log.Error().Err(err).Msg("cannot delete staging target path for volume")
	}

	log.Info().Msg("unmounted volume")

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume is called after NodeStageVolume and used to bind mount a volume
// from the staging folder into a pod-specific folder
func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log := *zerolog.Ctx(ctx)
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Errorf("failed to validate NodePublishVolumeRequest: %w", err).Error())
	}

	// Configuration

	targetPath := req.GetTargetPath()
	volId := volumeID(req.GetVolumeId())
	log = log.With().Str("volumeid", string(volId)).Str("targetpath", targetPath).Logger()

	if err := mkdir(targetPath); err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to create mount point for volume: %w", err).Error())
	}

	// Check if the volume is already mounted

	isMnt, err := folderIsMounted(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("stat failed: %w", err).Error())
	}

	if isMnt {
		log.Info().Msg("volume is already bind-mounted")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, bind-mount now

	if err = bindMount(req.GetStagingTargetPath(), targetPath); err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to bind-mount volume: %w", err).Error())
	}

	log.Info().Msg("bind-mounted volume")

	return &csi.NodePublishVolumeResponse{}, nil
}
func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log := *zerolog.Ctx(ctx)
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Errorf("failed to validate NodeUnpublishVolumeRequest: %w", err).Error())
	}

	targetPath := req.GetTargetPath()
	volId := volumeID(req.GetVolumeId())
	log = log.With().Str("volumeid", string(volId)).Str("targetpath", targetPath).Logger()

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", targetPath).Msg("unpublish called on non-existing directory")
	} else {
		if err := Unmount(targetPath); err != nil {
			log.Warn().Err(err).Str("path", targetPath).Msg("failed to unmount")
		}
		if err := os.Remove(targetPath); err != nil {
			log.Error().Err(err).Msg("cannot delete target path for volume")
		}
	}

	log.Info().Msg("volume unpublished")

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.config.NodeID,
	}, nil
}

func validateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return fmt.Errorf("volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return fmt.Errorf("staging target path missing in request")
	}

	return nil
}

func validateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return fmt.Errorf("staging target path missing in request")
	}

	return nil
}

func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return fmt.Errorf("volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return fmt.Errorf("target path missing in request")
	}

	return nil
}

func validateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return fmt.Errorf("target path missing in request")
	}

	return nil
}
