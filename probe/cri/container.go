package cri

import (
	"sync"

	"github.com/weaveworks/scope/report"
	runtime "github.com/weaveworks/scope/runtime"
)

// Container represents a CRI container
type Container interface {
	ID() string
	Image() string
	PID() int
}

type container struct {
	sync.RWMutex
	container              *runtime.Container
	stopStats              chan<- bool
	latestStats            runtime.ContainerStats
	pendingStats           [60]runtime.ContainerStats
	numPending             int
	hostID                 string
	baseNode               report.Node
	noCommandLineArguments bool
	noEnvironmentVariables bool
}

// StatsGatherer gathers container stats
type StatsGatherer interface {
	Stats(runtime.ContainerStatsRequest) error
}

// NewContainer creates a new Container
func NewContainer(c *runtime.Container, hostID string, noCommandLineArguments bool, noEnvironmentVariables bool) Container {
	result := &container{
		container:              c,
		hostID:                 hostID,
		noCommandLineArguments: noCommandLineArguments,
		noEnvironmentVariables: noEnvironmentVariables,
	}
	return result
}

func (c *container) ID() string {
	// runtime.CreateContainerRequest.PodSandboxId
	return c.container.Id
}

func (c *container) Image() string {

	return c.container.Image.Image
}

// TODO: no PID
func (c *container) PID() int {
	return 1
}
