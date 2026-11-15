# norix (Python SDK)

Minimal client for the Norix control plane — mirrors `sdk/typescript`.

```bash
pip install -e .
```

```python
from norix import NorixClient

client = NorixClient("http://127.0.0.1:8080", token="...")
print(client.health())
print(client.status())
print(client.list_projects())
```
