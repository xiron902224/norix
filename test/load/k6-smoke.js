// S20: k6 smoke against control-plane health (extend for auth/deploy load).
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 5,
  duration: '15s',
};

const BASE = __ENV.NORIX_API || 'http://127.0.0.1:8080';

export default function () {
  const res = http.get(`${BASE}/healthz`);
  check(res, { 'health 200': (r) => r.status === 200 });
  sleep(0.5);
}
