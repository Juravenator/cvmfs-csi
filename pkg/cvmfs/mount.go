package cvmfs

import (
	"fmt"
	"os"

	"github.com/cernops/cvmfs-csi/internal"
	"k8s.io/mount-utils"

	_ "embed"
)

var CVMFSConfigRepo = Repository("cvmfs-config.cern.ch")

const (
	CVMFSLocalConfigFile = "/etc/cvmfs/default.local"
)

// MountCVMFS mounts a given repository name to the /cvmfs/<repository> folder
func MountCVMFS(r Repository) error {
	to := r.getMountPath()
	log := internal.GetLogger("MountCVMFS").With().Str("to", to).Str("repository", string(r)).Logger()
	log.Debug().Msg("mounting repository")

	if err := mkdir(to); err != nil {
		return fmt.Errorf("cannot create target folder: %w", err)
	}

	log = log.With().Str("path", r.getMountPath()).Str("repository", string(r)).Logger()
	if _, err := execCommand("/usr/bin/mount", "-t", "cvmfs", string(r), r.getMountPath()); err != nil {
		log.Error().Err(err).Msg("mount failed")
		return err
	}

	log.Info().Msg("mounted")
	return nil
}

// Unmount unmounts the given path
func Unmount(mountpath string) error {
	_, err := execCommand("umount", mountpath)
	return err
}

func mkdir(path string) error {
	return os.MkdirAll(path, 0755)
}

func folderIsMounted(path string) (bool, error) {
	not, err := mount.IsNotMountPoint(mount.New(""), path)
	return !not, err
}

func bindMount(from, to string) error {
	if _, err := execCommand("mount", "--bind", from, to); err != nil {
		return fmt.Errorf("failed bind-mount of %s to %s: %v", from, to, err)
	}

	_, err := execCommand("mount", "-o", "remount,ro,bind", to)
	return err
}
