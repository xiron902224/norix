package composeparse

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Service is a Coolify-style editable unit from docker-compose.
type Service struct {
	Name            string            `json:"name"`
	Image           string            `json:"image,omitempty"`
	BuildContext    string            `json:"build_context,omitempty"`
	Dockerfile      string            `json:"dockerfile,omitempty"`
	ContainerPorts  []int32           `json:"container_ports"`
	PublishedPorts  []PortMap         `json:"published_ports"`
	DependsOn       []string          `json:"depends_on"`
	Environment     map[string]string `json:"environment,omitempty"`
	Enabled         bool              `json:"enabled"`
	ExposePublic    bool              `json:"expose_public"`
	HostPort        int32             `json:"host_port"`
	ContainerPort   int32             `json:"container_port"`
	HasBuild        bool              `json:"has_build"`
	HasImage        bool              `json:"has_image"`
}

// PortMap is a host→container binding from compose.
type PortMap struct {
	Host      int32  `json:"host"`
	Container int32  `json:"container"`
	Protocol  string `json:"protocol,omitempty"`
}

// Stack is the analyzed compose file.
type Stack struct {
	ComposeFile string    `json:"compose_file"`
	Services    []Service `json:"services"`
}

type rawFile struct {
	Services map[string]rawService `yaml:"services"`
}

type rawService struct {
	Image       string      `yaml:"image"`
	Build       any         `yaml:"build"`
	Ports       []any       `yaml:"ports"`
	Expose      []any       `yaml:"expose"`
	DependsOn   any         `yaml:"depends_on"`
	Environment any         `yaml:"environment"`
}

// ParseYAML analyzes a docker-compose document into editable services.
func ParseYAML(composeFile string, raw []byte) (Stack, error) {
	var doc rawFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Stack{}, fmt.Errorf("parse compose yaml: %w", err)
	}
	if len(doc.Services) == 0 {
		return Stack{}, fmt.Errorf("compose file has no services")
	}
	out := Stack{ComposeFile: composeFile, Services: make([]Service, 0, len(doc.Services))}
	// Stable-ish order: dependencies first when possible, else alphabetical via map iteration then sort by name.
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		svc := doc.Services[name]
		item := Service{
			Name:         name,
			Image:        strings.TrimSpace(svc.Image),
			Enabled:      true,
			ExposePublic: true,
			Environment:  parseEnv(svc.Environment),
			DependsOn:    parseDepends(svc.DependsOn),
			HasImage:     strings.TrimSpace(svc.Image) != "",
		}
		ctx, df := parseBuild(svc.Build)
		item.BuildContext = ctx
		item.Dockerfile = df
		item.HasBuild = ctx != "" || df != "" || svc.Build != nil
		if item.Dockerfile == "" && item.HasBuild {
			item.Dockerfile = "Dockerfile"
		}
		if item.BuildContext == "" && item.HasBuild {
			item.BuildContext = "."
		}
		item.PublishedPorts = parsePorts(svc.Ports)
		for _, p := range parseExpose(svc.Expose) {
			item.ContainerPorts = append(item.ContainerPorts, p)
		}
		for _, pm := range item.PublishedPorts {
			item.ContainerPorts = appendUnique(item.ContainerPorts, pm.Container)
		}
		if len(item.PublishedPorts) > 0 {
			item.HostPort = item.PublishedPorts[0].Host
			item.ContainerPort = item.PublishedPorts[0].Container
			if item.HostPort == 0 {
				item.HostPort = item.ContainerPort
			}
		} else if len(item.ContainerPorts) > 0 {
			item.ContainerPort = item.ContainerPorts[0]
			item.HostPort = item.ContainerPort
		} else {
			item.ContainerPort = 80
			item.HostPort = 0 // assign later
			item.ExposePublic = false
		}
		out.Services = append(out.Services, item)
	}
	return out, nil
}

func parseBuild(raw any) (context, dockerfile string) {
	switch v := raw.(type) {
	case nil:
		return "", ""
	case string:
		return strings.TrimSpace(v), "Dockerfile"
	case map[string]any:
		context, _ = v["context"].(string)
		dockerfile, _ = v["dockerfile"].(string)
		return strings.TrimSpace(context), strings.TrimSpace(dockerfile)
	default:
		return "", ""
	}
}

func parseDepends(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case map[string]any:
		out := make([]string, 0, len(v))
		for name := range v {
			out = append(out, name)
		}
		sortStrings(out)
		return out
	default:
		return nil
	}
}

func parseEnv(raw any) map[string]string {
	out := map[string]string{}
	switch v := raw.(type) {
	case map[string]any:
		for key, val := range v {
			out[key] = fmt.Sprint(val)
		}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			key, value, found := strings.Cut(s, "=")
			if found {
				out[key] = value
			} else {
				out[s] = ""
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parsePorts(raw []any) []PortMap {
	out := make([]PortMap, 0, len(raw))
	for _, item := range raw {
		switch v := item.(type) {
		case string:
			if pm, ok := parsePortString(v); ok {
				out = append(out, pm)
			}
		case int:
			out = append(out, PortMap{Container: int32(v), Protocol: "tcp"})
		case map[string]any:
			pm := PortMap{Protocol: "tcp"}
			if t, ok := v["protocol"].(string); ok {
				pm.Protocol = t
			}
			pm.Container = anyToPort(v["target"])
			if pm.Container == 0 {
				pm.Container = anyToPort(v["published"])
			}
			pm.Host = anyToPort(v["published"])
			if pm.Host == 0 {
				pm.Host = anyToPort(v["host_ip"])
			}
			if pm.Container > 0 {
				out = append(out, pm)
			}
		}
	}
	return out
}

func parseExpose(raw []any) []int32 {
	out := make([]int32, 0, len(raw))
	for _, item := range raw {
		switch v := item.(type) {
		case int:
			out = append(out, int32(v))
		case string:
			if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				out = append(out, int32(p))
			}
		}
	}
	return out
}

func parsePortString(value string) (PortMap, bool) {
	value = strings.TrimSpace(strings.Split(value, "/")[0])
	parts := strings.Split(value, ":")
	pm := PortMap{Protocol: "tcp"}
	switch len(parts) {
	case 1:
		p, err := strconv.Atoi(parts[0])
		if err != nil {
			return PortMap{}, false
		}
		pm.Container = int32(p)
		return pm, true
	case 2:
		h, err1 := strconv.Atoi(parts[0])
		c, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return PortMap{}, false
		}
		pm.Host, pm.Container = int32(h), int32(c)
		return pm, true
	case 3:
		h, err1 := strconv.Atoi(parts[1])
		c, err2 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil {
			return PortMap{}, false
		}
		pm.Host, pm.Container = int32(h), int32(c)
		return pm, true
	default:
		return PortMap{}, false
	}
}

func anyToPort(v any) int32 {
	switch t := v.(type) {
	case int:
		return int32(t)
	case int64:
		return int32(t)
	case float64:
		return int32(t)
	case string:
		p, _ := strconv.Atoi(strings.TrimSpace(t))
		return int32(p)
	default:
		return 0
	}
}

func appendUnique(list []int32, value int32) []int32 {
	if value <= 0 {
		return list
	}
	for _, item := range list {
		if item == value {
			return list
		}
	}
	return append(list, value)
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
