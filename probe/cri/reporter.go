package cri

import (
	"context"

	"github.com/weaveworks/scope/probe/docker"
	"github.com/weaveworks/scope/report"
	criClient "github.com/weaveworks/scope/runtime"
)

// Reporter generate Reports containing Container and ContainerImage topologies
type Reporter struct {
	cri criClient.RuntimeServiceClient
}

// NewReporter makes a new Reporter
func NewReporter(cri criClient.RuntimeServiceClient) *Reporter {
	reporter := &Reporter{
		cri: cri,
	}

	return reporter
}

// Name of this reporter, for metrics gathering
func (Reporter) Name() string { return "CRI" }

// Report generates a Report containing Container and ContainerImage topologies
func (r *Reporter) Report() (report.Report, error) {
	result := report.MakeReport()
	imageTopology, err := r.containerImageTopology()
	if err != nil {
		return report.MakeReport(), err
	}

	result.ContainerImage = result.ContainerImage.Merge(imageTopology)
	return result, nil
}

func (r *Reporter) containerImageTopology() (report.Topology, error) {
	result := report.MakeTopology().
		WithMetadataTemplates(docker.ContainerImageMetadataTemplates).
		WithTableTemplates(docker.ContainerImageTableTemplates)

	// walk the line...
	resp, err := r.cri.ListContainers(context.TODO(), nil)
	if err != nil {
		return result, err
	}

	for _, _ = range resp.Containers {
		latests := map[string]string{
			docker.ImageID:          c.ImageRef,
			docker.ImageSize:        "10MB",
			docker.ImageVirtualSize: "10MB",
			docker.ImageName:        c.Image.Image,
		}
		nodeID := report.MakeContainerImageNodeID(latests[docker.ImageID])
		node := report.MakeNodeWith(nodeID, latests)
		result.AddNode(node)
	}
	return result, nil
}
