# norix Go SDK

Minimal client for the Norix control plane — mirrors `sdk/typescript`.

```go
client := norix.New("http://127.0.0.1:8080", "token")
text, _ := client.Health(ctx)
status, _ := client.Status(ctx)
projects, _ := client.ListProjects(ctx)
```
