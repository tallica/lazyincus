package incustest

import (
	"time"

	"github.com/lxc/incus/v7/shared/api"
)

// Fixture is the daemon the golden screens in pkg/gui/testdata were drawn from.
func Fixture() *Server {
	running := func(project, name, ipv4 string, snapshots int) api.InstanceFull {
		instance := api.InstanceFull{
			Instance: api.Instance{
				Name: name, Project: project, Status: "Running", Type: "container",
				InstancePut: api.InstancePut{
					Architecture: "x86_64", Profiles: []string{"default"},
					Config: map[string]string{"volatile.base_image": "0123456789abcdef0123456789abcdef"},
				},
				ExpandedDevices: map[string]map[string]string{"eth0": {"type": "nic", "network": "incusbr0"}},
				ExpandedConfig: map[string]string{
					"image.description": "Alpine 3.22 amd64",
				},
			},
			State: &api.InstanceState{
				Status:    "Running",
				Processes: 12,
				Network: map[string]api.InstanceStateNetwork{
					"eth0": {HostName: "veth" + ipv4[len("192.0.2."):], Addresses: []api.InstanceStateNetworkAddress{
						{Family: "inet", Address: ipv4, Scope: "global"},
					}},
				},
			},
		}

		for i := range snapshots {
			instance.Snapshots = append(instance.Snapshots, api.InstanceSnapshot{
				Name:      name + "/daily-" + string(rune('a'+i)),
				CreatedAt: time.Date(2026, 9, 20+i, 6, 0, 0, 0, time.UTC),
			})
		}

		return instance
	}

	web := running("default", "web", "192.0.2.10", 2)
	web.ExpandedDevices["eth0"]["security.acls"] = "web-only"

	stopped := api.InstanceFull{Instance: api.Instance{
		Name: "db", Project: "default", Status: "Stopped", Type: "container",
		InstancePut: api.InstancePut{Architecture: "x86_64"},
	}}

	return New(Server{
		Version: "7.4",
		Instances: []api.InstanceFull{
			web,
			stopped,
			running("default", "a-name-long-enough-to-be-cut-off", "192.0.2.144", 0),
		},
		Images: []api.Image{
			{
				Fingerprint: "0123456789abcdef0123456789abcdef", Project: "default", Type: "container",
				Properties: map[string]string{"description": "Alpine 3.22 amd64"},
				Size:       3 << 20, LastUsedAt: time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC),
			},
			{
				Fingerprint: "fedcba9876543210fedcba9876543210", Project: "default", Type: "container",
				Properties: map[string]string{"description": "Debian 13 amd64"},
				Size:       90 << 20, Cached: true,
			},
		},
		Networks: []api.Network{
			{Name: "incusbr0", Type: "bridge", Managed: true, Project: "default", NetworkPut: api.NetworkPut{
				Config: map[string]string{"security.acls": "isolate", "security.acls.default.egress.action": "allow"},
			}},
			{Name: "eth0", Type: "physical", Project: "default"},
		},
		Profiles: []api.Profile{{
			Name: "default", Project: "default", UsedBy: []string{"/1.0/instances/web", "/1.0/instances/db"},
			ProfilePut: api.ProfilePut{
				Description: "Default Incus profile",
				Devices: map[string]map[string]string{
					"eth0": {"type": "nic", "network": "incusbr0", "name": "eth0"},
					"root": {"type": "disk", "pool": "default", "path": "/"},
				},
			},
		}},
		NetworkLeases: map[string]map[string][]api.NetworkLease{
			"incusbr0": {"default": {
				{Hostname: "incusbr0.gw", Address: "192.0.2.1", Type: "GATEWAY"},
				{Hostname: "web", Hwaddr: "10:66:6a:00:00:10", Address: "192.0.2.10", Type: "DYNAMIC"},
				{Hostname: "web", Hwaddr: "10:66:6a:00:00:10", Address: "2001:db8::10", Type: "DYNAMIC"},
			}},
		},
		NetworkACLs: map[string]api.NetworkACL{
			"isolate": {NetworkACLPost: api.NetworkACLPost{Name: "isolate"}, NetworkACLPut: api.NetworkACLPut{
				Description: "Keep the stack to itself",
				Ingress: []api.NetworkACLRule{
					{Action: "reject", Source: "10.0.0.0/8", State: "enabled"},
					{Action: "allow", Protocol: "tcp", DestinationPort: "80,443", State: "logged"},
				},
				Egress: []api.NetworkACLRule{{Action: "drop", Destination: "192.0.2.53", Protocol: "udp", DestinationPort: "53", State: "disabled"}},
			}},
			"web-only": {NetworkACLPost: api.NetworkACLPost{Name: "web-only"}, NetworkACLPut: api.NetworkACLPut{
				Ingress: []api.NetworkACLRule{{Action: "allow", Protocol: "tcp", DestinationPort: "80", State: "enabled"}},
			}},
		},
		NetworkForwards: map[string][]api.NetworkForward{
			"incusbr0": {{ListenAddress: "198.51.100.7", NetworkForwardPut: api.NetworkForwardPut{
				Ports: []api.NetworkForwardPort{
					{Protocol: "tcp", ListenPort: "443", TargetAddress: "192.0.2.10", Description: "https"},
				},
			}}},
		},
		NetworkStates: map[string]api.NetworkState{
			"incusbr0": {
				State: "up", Type: "broadcast", Hwaddr: "10:66:6a:00:00:01", Mtu: 1500,
				Addresses: []api.NetworkStateAddress{
					{Family: "inet", Address: "192.0.2.1", Netmask: "24", Scope: "global"},
				},
				Counters: &api.NetworkStateCounters{BytesReceived: 2048, PacketsReceived: 20, BytesSent: 1024, PacketsSent: 10},
				Bridge:   &api.NetworkStateBridge{ID: "8000.10666a000001", UpperDevices: []string{"veth10", "veth99"}},
			},
		},
		Volumes: map[string][]api.StorageVolume{
			"default": {
				{Name: "data", Type: "custom", Project: "default"},
				{Name: "web", Type: "container", Project: "default", UsedBy: []string{"/1.0/instances/web"}},
			},
		},
		PoolSpace:   map[string]api.ResourcesStoragePoolSpace{"default": {Used: 5 << 30, Total: 50 << 30}},
		VolumeUsage: map[string]uint64{"data": 512 << 20},
		VolumeSnapshots: map[string][]api.StorageVolumeSnapshot{"data": {{
			Name: "before-migration", CreatedAt: time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC),
		}}},
	})
}
