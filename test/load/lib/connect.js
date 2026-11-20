// Shared Connect JSON helpers for Norix k6 scripts.
// Unary Connect over HTTP: POST /api/v1.Service/Method with application/json.
import http from 'k6/http';

export function baseURL() {
  return (__ENV.NORIX_API || 'http://127.0.0.1:8080').replace(/\/$/, '');
}

export function connectHeaders(token) {
  const headers = {
    'Content-Type': 'application/json',
    'Connect-Protocol-Version': '1',
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

export function connectPost(path, body, token) {
  return http.post(`${baseURL()}/api${path}`, JSON.stringify(body || {}), {
    headers: connectHeaders(token),
    tags: { name: path },
  });
}

export function healthz() {
  return http.get(`${baseURL()}/healthz`, { tags: { name: 'GET /healthz' } });
}

export function parseJSON(res) {
  try {
    return res.json();
  } catch (_) {
    return null;
  }
}
