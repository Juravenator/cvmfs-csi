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

// see doc.go for a more holistic view of what the methods in this file do

import (
	"context"
	"fmt"

	"github.com/cernops/cvmfs-csi/internal"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type volumeID string

func newVolumeID() volumeID {
	return volumeID("csi-cvmfs-" + uuid.New().String())
}

// CreateVolume is called in response to the creation of a PersistentVolumeClaim
// For CVMFS, this is a NOP, and the volume only serves an administrative purpose
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log := zerolog.Ctx(ctx)
	err := d.validateCreateVolumeRequest(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Errorf("invalid CreateVolumeRequest: %w", err).Error())
	}

	volId := newVolumeID()

	log.Info().Str("volumeid", string(volId)).Msg("new volume created")

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      string(volId),
			VolumeContext: req.GetParameters(),
			CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
		},
	}, nil
}

// DeleteVolume is called in response to the deletion of the PersistentVolumeClaim
// responsible for the creation of this Volume
func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("volumeid", req.GetVolumeId()).Logger()
	if err := d.validateDeleteVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Errorf("cannot validate DeleteVolumeRequest: %w", err).Error())
	}

	log.Info().Msg("deleted volume")

	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerGetCapabilities is used to communicate driver capabilities
// This CVMFS driver supports only mounting, and not other features
// like snapshots or clones.
func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {

	return &csi.ControllerGetCapabilitiesResponse{Capabilities: d.controllerCapabilities}, nil
}

// ValidateVolumeCapabilities is used to verify that Kubernetes is creating
// volumes that this driver actually understands
// For this CVMFS driver, this means that we want to deal with volumes
// that represent readonly block storage
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if err := d.validateVolumeCapabilities(req.VolumeCapabilities); err != nil {
		return nil, status.Error(codes.Unimplemented, err.Error())
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: d.VolumeCapabilities,
		},
	}, nil
}

func (d *Driver) validateVolumeCapabilities(capabilities []*csi.VolumeCapability) error {
	log := internal.GetLogger("validateVolumeCapabilities")
	if capabilities == nil {
		return fmt.Errorf("volume capabilities cannot be empty")
	}
	for _, c := range capabilities {
		log.Trace().Interface("requestedVolumeCapability", c).Msg("checking if we can serve this volume capability")
		match := false
		for _, volumeCapability := range d.VolumeCapabilities {
			modeMatch := c.AccessMode.GetMode() == volumeCapability.AccessMode.GetMode()

			// validating a typematch is a bit icky given the wacko way it's implemented
			typeMatch := false
			ct := c.GetAccessType()
			vt := volumeCapability.GetAccessType()
			if ct == nil || vt == nil {
				typeMatch = ct == vt
			} else if _, ok := ct.(*csi.VolumeCapability_Mount); ok {
				_, ok = vt.(*csi.VolumeCapability_Mount)
				typeMatch = ok
			} else if _, ok := ct.(*csi.VolumeCapability_Block); ok {
				_, ok = vt.(*csi.VolumeCapability_Block)
				typeMatch = ok
			}
			log.Trace().Interface("driverVolumeCapability", volumeCapability).Bool("modematch", modeMatch).Bool("typematch", typeMatch).Msg("checking with this supported capability")
			if modeMatch && typeMatch {
				match = true
			}
		}
		if !match {
			return fmt.Errorf("invalid volume access mode '%s' type '%s'", c.AccessMode, c.AccessType)
		}
	}
	return nil
}

func (d *Driver) validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if req.GetName() == "" {
		return fmt.Errorf("volume name cannot be empty")
	}

	if err := d.validateVolumeCapabilities(req.VolumeCapabilities); err != nil {
		return err
	}
	return nil
}

func (d *Driver) validateDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	if req.VolumeId == "" {
		return fmt.Errorf("volume ID cannot be empty")
	}
	return nil
}
