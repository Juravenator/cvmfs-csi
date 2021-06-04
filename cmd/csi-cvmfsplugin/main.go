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

package main

import (
	"flag"

	"github.com/cernops/cvmfs-csi/internal"
	"github.com/cernops/cvmfs-csi/pkg/cvmfs"
)

var (
	config   = cvmfs.DriverConfig{}
	logLevel = flag.String("log.level", "info", "log level")
	logMode  = flag.String("log.mode", "plain", "log mode (plain|json)")
)

func main() {
	flag.StringVar(&config.Endpoint, "csi-address", "unix:///csi/csi.sock", "CSI socket address to share with helper sidecar containers (e.g. csi-attacher)")
	flag.StringVar(&config.DriverName, "drivername", "cvmfs.csi.cern.ch", "name of the driver. To be used as 'provisioner' for K8S StorageClasses")
	flag.StringVar(&config.Proxy, "cvmfs-proxy", "http://ca-proxy.cern.ch:3128", "proxy to use for CVMFS mounts")
	flag.StringVar(&config.CacheFolder, "cache-folder", "/var/cache/cvmfs", "cache location to use for CVMFS mounts")
	flag.StringVar(&config.NodeID, "nodeid", "", "name of the node this runs on (recommended to use spec.nodeName in your statefulset/deployment)")
	flag.Parse()
	internal.InitLogging(*logLevel, *logMode)

	log := internal.GetLogger("")
	log.Info().Msg("starting")

	driver, err := cvmfs.NewDriver(config)
	if err != nil {
		log.Fatal().Err(err).Msg("driver start failed")
	}

	driver.Run()
	log.Warn().Msg("finished")
}
