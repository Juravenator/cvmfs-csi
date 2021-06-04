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
	"fmt"
	"os/exec"
	"time"

	"github.com/cernops/cvmfs-csi/internal"
)

func execCommand(program string, args ...string) ([]byte, error) {
	log := internal.GetLogger("execCommand").With().Str("program", program).Strs("args", args).Logger()
	log.Info().Msg("executing command")

	cmd := exec.Command(program, args[:]...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	timeout := time.After(30 * time.Second)
	done := make(chan error)
	go func() { done <- cmd.Wait() }()
	select {
	case <-timeout:
		cmd.Process.Kill()
		err = fmt.Errorf("command timed out")
	case err = <-done:
		if err == nil && cmd.ProcessState.ExitCode() > 0 {
			err = fmt.Errorf("non-zero exit code: %d", cmd.ProcessState.ExitCode())
		}
	}

	log.Debug().Err(err).Msg("command finished")
	b := buf.Bytes()
	log.Trace().Bytes("output", b).Msg("command finished")
	return b, err
}
