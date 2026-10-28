package stack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/tweinmann/siding/internal/compose"
)

// OverrideFile is where siding writes the override, relative to the worktree.
const OverrideFile = ".siding/override.yml"

// ProxyNetworkKey is the key of the proxy network in the override. It differs from every
// network name a user is likely to pick, so the override never redefines one of theirs.
const ProxyNetworkKey = "siding_proxy"

// ServiceHeader is the response header every siding router adds, naming <project>/<service>.
// A response that carries it came from the service, not from the proxy (design D9).
const ServiceHeader = "X-Siding-Service"

// Override renders .siding/override.yml: no published ports, siding's labels on every
// container, network and volume of the project, and for every route the proxy network and
// the Traefik labels. network is the proxy's Docker network.
func Override(p *compose.Project, id Identity, routes []Route, network string) []byte {
	var b strings.Builder
	project := id.Project()
	fmt.Fprintf(&b, "# Written by siding for the Compose project %s. Do not edit: siding up rewrites it.\n", project)
	b.WriteString("services:\n")
	for _, name := range p.ServiceNames() {
		s := p.Services[name]
		fmt.Fprintf(&b, "  %s:\n", q(name))
		b.WriteString("    ports: !reset []\n")
		labels := id.ContainerLabels()
		if r, ok := Find(routes, name); ok {
			b.WriteString("    networks:\n")
			for _, n := range s.Networks {
				fmt.Fprintf(&b, "      %s: {}\n", q(n))
			}
			fmt.Fprintf(&b, "      %s: {}\n", ProxyNetworkKey)
			maps.Copy(labels, traefikLabels(id, r, network))
		}
		writeLabels(&b, "    ", labels)
	}

	b.WriteString("networks:\n")
	for _, key := range sortedKeys(p.Networks) {
		if p.Networks[key].External {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", q(key))
		writeLabels(&b, "    ", id.Labels())
	}
	fmt.Fprintf(&b, "  %s:\n    name: %s\n    external: true\n", ProxyNetworkKey, q(network))

	volumes := slices.DeleteFunc(sortedKeys(p.Volumes), func(k string) bool { return p.Volumes[k].External })
	if len(volumes) > 0 {
		b.WriteString("volumes:\n")
		for _, key := range volumes {
			fmt.Fprintf(&b, "  %s:\n", q(key))
			writeLabels(&b, "    ", id.Labels())
		}
	}
	return []byte(b.String())
}

// traefikLabels route the service host to the service and, for a route path, the path on the
// stack's host. Router names are unique per daemon because they start with the project.
func traefikLabels(id Identity, r Route, network string) map[string]string {
	name := id.Project() + "-" + Slug(r.Service)
	middleware := name + "-id"
	l := map[string]string{
		"traefik.enable":         "true",
		"traefik.docker.network": network,
		"traefik.http.services." + name + ".loadbalancer.server.port":                                fmt.Sprint(r.Port),
		"traefik.http.middlewares." + middleware + ".headers.customresponseheaders." + ServiceHeader: id.Project() + "/" + r.Service,
	}
	router := func(router, rule string) {
		prefix := "traefik.http.routers." + router
		l[prefix+".rule"] = rule
		l[prefix+".entrypoints"] = "web"
		l[prefix+".service"] = name
		l[prefix+".middlewares"] = middleware
	}
	router(name, "Host(`"+id.ServiceHost(r.Service)+"`)")
	switch r.Path {
	case "":
	case "/":
		router(name+"-route", "Host(`"+id.Host()+"`)")
	default:
		router(name+"-route", "Host(`"+id.Host()+"`) && PathPrefix(`"+r.Path+"`)")
	}
	return l
}

func writeLabels(b *strings.Builder, indent string, labels map[string]string) {
	b.WriteString(indent + "labels:\n")
	for _, k := range sortedKeys(labels) {
		fmt.Fprintf(b, "%s  %s: %s\n", indent, q(k), q(labels[k]))
	}
}

// q quotes a YAML scalar. A JSON string is a valid YAML double-quoted scalar.
func q(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
