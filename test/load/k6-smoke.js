// Lightweight health smoke — safe for CI without a real control plane
// (point NORIX_API at any HTTP server that returns 200 on GET /healthz).
// Full API scenarios: k6-api.js with NORIX_LOAD_FULL=1.
import { check, sleep } from 'k6';
import { healthz } from './lib/connect.js';

export const options = {
  vus: Number(__ENV.NORIX_LOAD_VUS || 5),
  duration: __ENV.NORIX_LOAD_DURATION || '15s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

export default function () {
  const res = healthz();
  check(res, { 'health 200': (r) => r.status === 200 });
  sleep(0.5);
}
