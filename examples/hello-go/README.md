# hello-go

Minimal Go HTTP app used to validate Norix S5 (git → BuildKit/docker build → run).

```bash
# from a machine with docker
docker build -t hello-go .
docker run --rm -p 8080:8080 hello-go
```

In the Norix dashboard: create a workload, then **Build & deploy from git** with this repo URL
(or a fork), ref `main`, Dockerfile path `Dockerfile`, container port `8080`.
