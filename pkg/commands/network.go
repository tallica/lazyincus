package commands

import (
	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// Network is a network as listed by `incus network list`, managed by Incus
// or merely detected on the host.
type Network struct {
	Name string

	Network   api.Network
	Client    incus.InstanceServer
	OSCommand *OSCommand
	Log       *logrus.Entry
	Tr        *i18n.TranslationSet
}

func (n *Network) IsManaged() bool {
	return n.Network.Managed
}

// UsedByCount is how many profiles and instances reference the network.
func (n *Network) UsedByCount() int {
	return len(n.Network.UsedBy)
}

// Delete removes the network. Incus only allows this for managed networks
// that nothing is using; it rejects the rest itself.
func (n *Network) Delete() error {
	return n.Client.DeleteNetwork(n.Name)
}
