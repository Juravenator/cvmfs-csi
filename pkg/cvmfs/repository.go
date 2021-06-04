package cvmfs

import (
	"fmt"
	"path"
)

type Repository string

func RepositoryFrom(s string) (Repository, error) {
	r := Repository(s)
	return r, r.Validate()
}

func RepositoryFromContext(m map[string]string) (Repository, error) {
	r := Repository(m["repository"])
	return r, r.Validate()
}

func (r *Repository) Validate() error {
	if string(*r) == "" {
		return fmt.Errorf("empty repository parameter")
	}
	return nil
}

func (r *Repository) getMountPath() string {
	return path.Join("/cvmfs", string(*r))
}
