# Troubleshooting

| Symptom | Likely cause | What to do |
|---|---|---|
| Deploy / DeployFromGit fails with `no online node with Docker or containerd` | Agents online but no Docker/containerd socket | Install Docker on a worker; restart agent; confirm Nodes → Docker = yes |
| Opaque “internal error” on deploy | Older CP hid scheduler reasons | Upgrade control plane; check node runtimes and cordon/drain |
| Agent offline | Process dead, wrong `NORIX_CONTROL_PLANE_URL`, firewall | `norix-agent check`; inspect agent logs; allow worker → CP |
| UI version badge missing (local Vite) | `/status` not proxied | Use Norix ≥ this guide’s release; Vite must proxy `/status` |
| Agents on different versions | Partial upgrades | Nodes → **Update agent**, or `norix-agent update --version …` |
| Dashboard login loop | Wrong `NORIX_WEB_ORIGIN` / cookies / HTTPS mismatch | Align public URL and CORS origins with how you open the UI |

## CLI checks

```bash
# Control plane
norix-control-plane check
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/status | jq .

# Agent (no join token required for check)
NORIX_CONTROL_PLANE_URL=https://YOUR_CP norix-agent check
```

## Get help

- GitHub Issues: https://github.com/xiron902224/norix/issues  
- Security: [`SECURITY.md`](../../SECURITY.md) (private reports only)
