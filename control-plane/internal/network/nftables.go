package network

import (
	"fmt"
	"strings"
)

type ProjectIsolation struct {
	ProjectID   string
	ProjectName string
	Mark        int
	NodeAddrs   []string
}

type EgressRule struct {
	WorkloadID   string
	WorkloadName string
	Mode         string // allow | deny | allowlist
	Allowlist    []string
}

// RenderNftables builds a declarative ruleset isolating projects and applying egress policy.
func RenderNftables(projects []ProjectIsolation, egress []EgressRule) string {
	var b strings.Builder
	b.WriteString("#!/usr/sbin/nft -f\n")
	b.WriteString("flush table inet norix\n")
	b.WriteString("table inet norix {\n")
	b.WriteString("  set project_marks {\n    type mark\n    flags interval\n  }\n")
	b.WriteString("  chain forward {\n    type filter hook forward priority 0; policy accept;\n")
	b.WriteString("    # Cross-project traffic on the overlay is dropped unless marks match.\n")
	b.WriteString("    meta mark != 0 ct mark set meta mark\n")
	for _, project := range projects {
		fmt.Fprintf(&b, "    # project %s mark %d\n", project.ProjectName, project.Mark)
		for _, addr := range project.NodeAddrs {
			fmt.Fprintf(&b, "    ip saddr %s meta mark set %d\n", strings.TrimSuffix(addr, "/32"), project.Mark)
		}
	}
	b.WriteString("    meta mark != 0 ct mark != 0 meta mark != ct mark drop comment \"norix-project-isolation\"\n")
	b.WriteString("  }\n")
	b.WriteString("  chain output {\n    type filter hook output priority 0; policy accept;\n")
	for _, rule := range egress {
		switch strings.ToLower(rule.Mode) {
		case "deny":
			fmt.Fprintf(&b, "    # deny all egress for workload %s\n", rule.WorkloadName)
			b.WriteString("    # placeholder: container-specific egress enforced via cgroup mark in future\n")
		case "allowlist":
			fmt.Fprintf(&b, "    # allowlist egress for workload %s\n", rule.WorkloadName)
			for _, cidr := range rule.Allowlist {
				fmt.Fprintf(&b, "    ip daddr %s accept comment \"norix-allow-%s\"\n", cidr, sanitizeDNS(rule.WorkloadName))
			}
			fmt.Fprintf(&b, "    # unmatched egress for %s would drop once cgroup marks land\n", rule.WorkloadName)
		default:
			fmt.Fprintf(&b, "    # allow egress for workload %s\n", rule.WorkloadName)
		}
	}
	b.WriteString("  }\n}\n")
	return b.String()
}
