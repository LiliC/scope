package cri

import (
	"fmt"
	"net"
	"sync"

	"github.com/weaveworks/scope/report"
	runtime "github.com/weaveworks/scope/runtime"
)

// Container represents a CRI container
type Container interface {
	UpdateState(*runtime.Container)

	ID() string
	Image() string
	PID() int
	Hostname() string
	GetNode() report.Node
	State() string
	StateString() string
	HasTTY() bool
	Container() *runtime.Container
	StartGatheringStats(StatsGatherer) error
	StopGatheringStats()
	NetworkMode() (string, bool)
	NetworkInfo([]net.IP) report.Sets
}

type container struct {
	sync.RWMutex
	container              *Container
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
	result.baseNode = result.getBaseNode()
	return result
}

func (c *container) ID() string {
	return c.container.ID
}

func (c *container) Image() string {
	return trimImageID(c.container.Image)
}

func (c *container) PID() int {
	return c.container.State.Pid
}

func (c *container) Hostname() string {
	if c.container.Config.Domainname == "" {
		return c.container.Config.Hostname
	}

	return fmt.Sprintf("%s.%s", c.container.Config.Hostname,
		c.container.Config.Domainname)
}

func (c *container) HasTTY() bool {
	return c.container.Config.Tty
}

func (c *container) State() string {
	return c.container.State.String()
}

func (c *container) StateString() string {
	return c.container.State.StateString()
}

func (c *container) Container() *runtime.Container {
	return c.container
}
