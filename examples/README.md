# Norix example apps

| Example | Build path | Port |
|---|---|---|
| [`hello-go`](./hello-go/) | Dockerfile (classic git deploy) | 8080 |
| [`hello-go-railpack`](./hello-go-railpack/) | **Railpack** (no Dockerfile) | 8080 |
| [`hello-node`](./hello-node/) | **Railpack** | 8080 |
| [`hello-python`](./hello-python/) | **Railpack** | 8080 |
| [`hello-static`](./hello-static/) | **Railpack** | 80 / framework default |

Railpack examples have **no** Dockerfile. Point **Build & deploy from git** at the folder (or a fork) and leave Dockerfile path as `Dockerfile` — Norix falls back to Railpack when markers like `package.json` / `go.mod` / `requirements.txt` / `index.html` are present. Requires `railpack` + BuildKit on the builder node ([docs](../docs/user/deploying.md#railpack-no-dockerfile)).
