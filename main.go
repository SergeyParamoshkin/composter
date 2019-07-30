package main

// TODO:

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/filters"

	"github.com/docker/go-connections/nat"

	"flag"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

type Serivce struct {
	Version  string                   `yaml:"version"`
	Networks map[string]NetworkConfig `yaml:"networks"`
	Serivce  map[string]ServiceConfig `yaml:"services"`
	Volumes  map[string]VolumeConfig  `yaml:"volumes"`
}

type VolumeConfig struct {
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}
type NetworkConfig struct {
	Name     string `yaml:"name"`
	External bool   `yaml:"external,omitempty"`
}

type ServiceConfig struct {
	ContainerName string             `yaml:"container_name"`
	Image         string             `yaml:"image"`
	Restart       string             `yaml:"restart,omitempty"`
	WorkingDir    string             `yaml:"working_dir,omitempty"`
	Command       string             `yaml:"command,omitempty"`
	Volumes       []string           `yaml:"volumes,omitempty"`
	Ports         []string           `yaml:"ports,omitempty"`
	Env           []string           `yaml:"environment,omitempty"`
	Networks      map[string]Network `yaml:"networks,omitempty"`
	Logging       LogConfig          `yaml:"logging,omitempty"`
}

// Custom type for yaml tag contol
type LogConfig struct {
	Type   string            `yaml:"type,omitempty"`
	Config map[string]string `yaml:"options,omitempty"`
}

type Network struct {
	Aliases []string `yaml:"aliases,omitempty"`
}

func main() {
	var (
		fileName string
	)
	flag.StringVar(&fileName, "file", "docker-compose.yml", "file name for docker-compose")
	flag.Parse()
	cli, err := client.NewClient(client.DefaultDockerHost, client.DefaultVersion, nil, nil)
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
		All: true,
	})
	if err != nil {
		panic(err)
	}

	s := Serivce{}
	v, err := cli.ServerVersion(context.Background())
	if err != nil {
		log.Println(err)
	}
	s.Version = makeVersion(v)
	s.Networks = make(map[string]NetworkConfig)
	s.Volumes = make(map[string]VolumeConfig)
	s.Serivce = make(map[string]ServiceConfig)

	networks, err := cli.NetworkList(context.Background(), types.NetworkListOptions{
		Filters: filters.Args{},
	})
	if err != nil {
		log.Println(err)
	}
	for _, network := range networks {
		nj, err := cli.NetworkInspect(context.Background(), network.ID)
		if err != nil {
			log.Println(err)
		}

		s.Networks[network.Name] = NetworkConfig{
			Name:     network.Name,
			External: nj.Internal,
		}
	}

	volumes, err := cli.VolumeList(context.Background(), filters.Args{})
	if err != nil {
		log.Println(err)
	}
	for _, volume := range volumes.Volumes {
		s.Volumes[volume.Name] = VolumeConfig{
			Driver:     volume.Driver,
			DriverOpts: volume.Options,
		}
	}
	//
	for _, container := range containers {
		cj, err := cli.ContainerInspect(context.Background(), container.ID)
		if err != nil {
			log.Println(err)
		}

		s.Serivce[makeContainerName(cj.Name)] = ServiceConfig{
			ContainerName: makeContainerName(cj.Name),
			Image:         cj.Config.Image,
			Restart:       makeRestartPolicy(cj.HostConfig.RestartPolicy),
			WorkingDir:    cj.Config.WorkingDir,
			Command:       makeCommand(cj.Config.Cmd),
			Volumes:       cj.HostConfig.Binds,
			Networks:      makeNetworks(cj.NetworkSettings.Networks, cj.Config.Hostname),
			Ports:         makePorts(cj.NetworkSettings.Ports),
			Env:           cj.Config.Env,
			Logging:       makeLogConfig(cj.HostConfig.LogConfig),
		}
	}

	// s.Serivce = services
	b, err := yaml.Marshal(s)
	if err != nil {
		log.Println(err)
	}
	if len(fileName) > 0 {
		err = ioutil.WriteFile("docker-compose.yml", b, 0644)
		if err != nil {
			log.Println(err)
		}
	}
	fmt.Println(string(b))
}

func makeLogConfig(lg container.LogConfig) LogConfig {
	if lg.Type == "json-file" {
		return LogConfig{
			Config: lg.Config,
		}
	}
	return LogConfig{
		Type:   lg.Type,
		Config: lg.Config,
	}
}

func cleanUpAliases(aliases []string, hostname string) []string {
	var as []string
	for _, a := range aliases {
		if a != hostname {
			as = append(as, a)
		}
	}
	return as
}

func makeNetworks(ns map[string]*network.EndpointSettings, hostname string) map[string]Network {
	networks := map[string]Network{}
	for network, v := range ns {
		networks[network] = Network{
			Aliases: cleanUpAliases(v.Aliases, hostname),
		}
	}
	return networks
}

func makePorts(ps nat.PortMap) []string {
	ports := make([]string, 0, len(ps))
	for k, v := range ps {
		for _, p := range v {
			port := p.HostIP + ":" + p.HostPort + ":" + k.Port() + "/" + k.Proto()
			ports = append(ports, port)
		}
	}
	return ports
}

func makeContainerName(cn string) string {
	return strings.ReplaceAll(cn, "/", "")
}

func makeCommand(cmd strslice.StrSlice) string {
	return strings.Join(cmd, " ")
}

func makeRestartPolicy(rp container.RestartPolicy) string {
	if rp.IsNone() {
		return ""
	}
	return rp.Name
}

func makeVersion(v types.Version) string {
	nv, err := strconv.Atoi(strings.ReplaceAll(v.Version, ".", ""))
	if err != nil {
		log.Println(err)
	}

	if nv > 18060 {
		return "3.7"
	}
	return v.Version
}
