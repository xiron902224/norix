"""Minimal Norix Python SDK — mirrors sdk/typescript baseline."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from typing import Any, Mapping, Optional


class NorixError(RuntimeError):
    pass


class NorixClient:
    def __init__(self, base_url: str, token: Optional[str] = None, timeout: float = 30.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout

    def health(self) -> str:
        """GET /healthz — returns body text (usually 'ok')."""
        return self._get_text("/healthz")

    def status(self) -> dict[str, Any]:
        """GET /status — public JSON component health."""
        return self._get_json("/status")

    def list_projects(self) -> Any:
        return self._connect("v1.ProjectService/ListProjects", {})

    def list_workloads(self, project_id: str) -> Any:
        return self._connect("v1.WorkloadService/ListWorkloads", {"projectId": project_id})

    def list_sandboxes(self, project_id: str = "") -> Any:
        return self._connect("v1.SandboxService/ListSandboxes", {"projectId": project_id})

    def create_sandbox(
        self,
        project_id: str,
        name: str,
        template: str = "blank",
        ttl_seconds: int = 3600,
    ) -> Any:
        return self._connect(
            "v1.SandboxService/CreateSandbox",
            {
                "projectId": project_id,
                "name": name,
                "template": template,
                "ttlSeconds": ttl_seconds,
            },
        )

    def query_logs(self, query: str = "", limit: int = 100) -> Any:
        return self._connect("v1.MetricsService/QueryLogs", {"query": query, "limit": limit})

    def _headers(self, content_type: Optional[str] = None) -> dict[str, str]:
        headers: dict[str, str] = {}
        if content_type:
            headers["Content-Type"] = content_type
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def _get_text(self, path: str) -> str:
        req = urllib.request.Request(
            f"{self.base_url}{path}",
            headers=self._headers(),
            method="GET",
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return resp.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            raise NorixError(f"GET {path} failed: {exc.code}") from exc

    def _get_json(self, path: str) -> dict[str, Any]:
        req = urllib.request.Request(
            f"{self.base_url}{path}",
            headers=self._headers(),
            method="GET",
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as exc:
            raise NorixError(f"GET {path} failed: {exc.code}") from exc

    def _connect(self, procedure: str, body: Mapping[str, Any]) -> Any:
        url = f"{self.base_url}/api/{procedure}"
        headers = self._headers("application/json")
        headers["Connect-Protocol-Version"] = "1"
        data = json.dumps(body).encode("utf-8")
        req = urllib.request.Request(url, data=data, headers=headers, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as exc:
            raise NorixError(f"Norix API {procedure} failed: {exc.code}") from exc


__all__ = ["NorixClient", "NorixError"]
