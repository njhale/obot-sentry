package scan

import "github.com/obot-platform/obot/apiclient/types"

// observations groups what sources and skill discovery emit. Slices
// accumulate in the orchestrator; the shared file/client tables live on
// state instead.
type observations struct {
	servers []types.DeviceScanMCPServer
	skills  []skill
	plugins []types.DeviceScanPlugin
}

func (o *observations) add(other observations) {
	o.servers = append(o.servers, other.servers...)
	o.skills = append(o.skills, other.skills...)
	o.plugins = append(o.plugins, other.plugins...)
}
