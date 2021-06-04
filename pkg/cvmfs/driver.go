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
	"errors"

	"github.com/cernops/cvmfs-csi/internal"
	"github.com/container-storage-interface/spec/lib/go/csi"
)

/*
Driver handles all relevant CSI gRPC calls for a funcional Identity, Controller, and Node Server
as described in https://github.com/container-storage-interface/spec/blob/master/spec.md
*/
type Driver struct {
	*csi.UnimplementedIdentityServer
	*csi.UnimplementedControllerServer
	*csi.UnimplementedNodeServer

	config                 DriverConfig
	controllerCapabilities []*csi.ControllerServiceCapability
	VolumeCapabilities     []*csi.VolumeCapability
}

const DriverVersion = "1.0.1"

type DriverConfig struct {
	DriverName  string
	NodeID      string
	Endpoint    string
	Proxy       string
	CacheFolder string
}

// NewDriver constructs a new Driver given a valid DriverConfig
func NewDriver(c DriverConfig) (*Driver, error) {
	if c.DriverName == "" {
		return nil, errors.New("Driver name missing")
	}

	if c.NodeID == "" {
		return nil, errors.New("NodeID missing")
	}

	if c.Endpoint == "" {
		return nil, errors.New("Driver endpoint missing")
	}

	log := internal.GetLogger("NewDriver")
	log.Info().Str("driver name", c.DriverName).Str("node ID", c.NodeID).Str("endpoint", c.Endpoint).Msg("new driver")
	driver := &Driver{config: c}
	driver.VolumeCapabilities = []*csi.VolumeCapability{mountVolumeCapability(csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY)}
	driver.controllerCapabilities = []*csi.ControllerServiceCapability{controllerCapability(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME)}
	return driver, nil
}

// Run starts the driver and waits for it to stop.
// If a driver stopped it signifies something went wrong
func (d *Driver) Run() {
	server := &nonBlockingGRPCServer{}
	server.Start(d.config.Endpoint, d, d, d)
	server.Wait()
}

func mountVolumeCapability(mode csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability {
	return &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		AccessMode: &csi.VolumeCapability_AccessMode{Mode: mode},
	}
}

func controllerCapability(c csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: c,
			},
		},
	}
}
