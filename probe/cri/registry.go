package cri

import (
	"fmt"
)

// Registry keeps track of running docker containers and their images
type Registry interface {
	Stop()
	LockedPIDLookup(f func(func(int) Container))
	WalkContainers(f func(Container))
	WalkImages(f func(runtime.Image))
	WalkNetworks(f func(docker_client.Network))
	WatchContainerUpdates(ContainerUpdateWatcher)
	GetContainer(string) (Container, bool)
	GetContainerByPrefix(string) (Container, bool)
	GetContainerImage(string) (runtime.Image, bool)
}

// WalkContainers runs f on every running containers the registry knows of.
func (r *registry) WalkContainers(f func(Container)) {
	r.RLock()
	defer r.RUnlock()

	r.containers.Walk(func(_ string, c interface{}) bool {
		f(c.(Container))
		return false
	})
}
