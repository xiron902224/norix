// Full API load: login/bootstrap, project + workload create (deploy path),
// authenticated reads, and heartbeat-style fan-out (concurrent healthz + ListProjects).
// Requires a running control plane. Gate with NORIX_LOAD_FULL=1.
//
// Real AgentService/Heartbeat needs a node token after EnrollNode — that path is
// documented in docs/user/load-tests.md. Here we simulate agent fan-out pressure
// with concurrent /healthz + authenticated ListProjects (same read path agents
// and the UI hammer under many nodes).
import { check, fail, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { baseURL, connectPost, healthz, parseJSON } from './lib/connect.js';

const loginLatency = new Trend('norix_login_ms');
const listLatency = new Trend('norix_list_projects_ms');
const createWorkloadOk = new Counter('norix_create_workload_ok');

if (__ENV.NORIX_LOAD_FULL !== '1') {
  fail('Set NORIX_LOAD_FULL=1 and point NORIX_API at a running control plane. See docs/user/load-tests.md');
}

const email = __ENV.NORIX_LOAD_EMAIL || `load-${Date.now()}@example.com`;
const password = __ENV.NORIX_LOAD_PASSWORD || 'load-test-password-change-me';
const orgName = __ENV.NORIX_LOAD_ORG || 'Norix Load';

export const options = {
  scenarios: {
    health: {
      executor: 'constant-vus',
      vus: Number(__ENV.NORIX_LOAD_HEALTH_VUS || 10),
      duration: __ENV.NORIX_LOAD_DURATION || '30s',
      exec: 'healthScenario',
    },
    api_reads: {
      executor: 'constant-vus',
      vus: Number(__ENV.NORIX_LOAD_READ_VUS || 5),
      duration: __ENV.NORIX_LOAD_DURATION || '30s',
      startTime: '2s',
      exec: 'apiReadScenario',
    },
    deploy_create: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.NORIX_LOAD_DEPLOY_RATE || 2),
      timeUnit: '1s',
      duration: __ENV.NORIX_LOAD_DURATION || '30s',
      preAllocatedVUs: 5,
      maxVUs: 20,
      startTime: '4s',
      exec: 'deployScenario',
    },
    heartbeat_fanout: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '5s', target: Number(__ENV.NORIX_LOAD_HB_VUS || 25) },
        { duration: __ENV.NORIX_LOAD_DURATION || '30s', target: Number(__ENV.NORIX_LOAD_HB_VUS || 25) },
        { duration: '5s', target: 0 },
      ],
      startTime: '2s',
      exec: 'heartbeatFanoutScenario',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.1'],
    http_req_duration: ['p(95)<3000'],
    norix_login_ms: ['p(95)<2000'],
    norix_list_projects_ms: ['p(95)<2000'],
  },
};

export function setup() {
  const hz = healthz();
  if (hz.status !== 200) {
    fail(`control plane not healthy at ${baseURL()}/healthz (status ${hz.status})`);
  }

  const statusRes = connectPost('/v1.AuthService/GetBootstrapStatus', {});
  const statusBody = parseJSON(statusRes);
  check(statusRes, { 'bootstrap status 200': (r) => r.status === 200 });

  let token = __ENV.NORIX_LOAD_TOKEN || '';

  if (!token) {
    if (statusBody && statusBody.configured === false) {
      const boot = connectPost('/v1.AuthService/CreateInitialAdmin', {
        organizationName: orgName,
        adminName: 'Load Admin',
        adminEmail: email,
        adminPassword: password,
      });
      const bootBody = parseJSON(boot);
      check(boot, { 'bootstrap 200': (r) => r.status === 200 });
      token = (bootBody && bootBody.token) || '';
    } else {
      const loginEmail = __ENV.NORIX_LOAD_EXISTING_EMAIL || email;
      const loginPassword = __ENV.NORIX_LOAD_EXISTING_PASSWORD || password;
      const started = Date.now();
      const login = connectPost('/v1.AuthService/Login', {
        email: loginEmail,
        password: loginPassword,
      });
      loginLatency.add(Date.now() - started);
      const loginBody = parseJSON(login);
      check(login, { 'login 200': (r) => r.status === 200 });
      token = (loginBody && loginBody.token) || '';
    }
  }

  if (!token) {
    fail('no session token — set NORIX_LOAD_TOKEN or NORIX_LOAD_EXISTING_EMAIL/PASSWORD');
  }

  const projectName = `load-proj-${Date.now()}`;
  const create = connectPost(
    '/v1.ProjectService/CreateProject',
    { name: projectName },
    token,
  );
  const createBody = parseJSON(create);
  check(create, { 'create project 200': (r) => r.status === 200 });
  const projectId = createBody && createBody.project && createBody.project.id;
  if (!projectId) {
    fail('CreateProject did not return project.id');
  }

  return { token, projectId, projectName };
}

export function healthScenario() {
  const res = healthz();
  check(res, { 'health 200': (r) => r.status === 200 });
  sleep(0.2);
}

export function apiReadScenario(data) {
  const started = Date.now();
  const res = connectPost('/v1.ProjectService/ListProjects', {}, data.token);
  listLatency.add(Date.now() - started);
  check(res, {
    'list projects 200': (r) => r.status === 200,
    'list has projects array': (r) => {
      const body = parseJSON(r);
      return body && Array.isArray(body.projects);
    },
  });
  sleep(0.3);
}

export function deployScenario(data) {
  const name = `w-${__VU}-${__ITER}-${Date.now()}`;
  const create = connectPost(
    '/v1.WorkloadService/CreateWorkload',
    {
      workload: {
        projectId: data.projectId,
        name,
        runtime: 'RUNTIME_TYPE_CONTAINER',
        imageOrSource: 'nginx:alpine',
        containerPort: 80,
        sourceKind: 'image',
        cpuShares: 1024,
        memoryLimitMb: '512',
        replicaCount: 1,
        scheduleStrategy: 'spread',
        preferredZone: 'default',
      },
    },
    data.token,
  );
  const created = parseJSON(create);
  const ok = create.status === 200 && created && created.workload && created.workload.id;
  check(create, { 'create workload 200': (r) => r.status === 200 });
  if (ok) {
    createWorkloadOk.add(1);
    // Best-effort Deploy — may return FailedPrecondition without an agent.
    // Still exercises the scheduling path under load.
    const deploy = connectPost(
      '/v1.WorkloadService/Deploy',
      {
        workload: { id: created.workload.id },
        containerPort: 80,
      },
      data.token,
    );
    check(deploy, {
      'deploy accepted or precondition': (r) =>
        r.status === 200 || r.status === 400 || r.status === 409 || r.status === 503,
    });
  }
  sleep(0.2);
}

export function heartbeatFanoutScenario(data) {
  // Simulated agents: concurrent health probes + authenticated inventory reads.
  const hz = healthz();
  check(hz, { 'fan-out health 200': (r) => r.status === 200 });
  const list = connectPost('/v1.ProjectService/ListProjects', {}, data.token);
  check(list, { 'fan-out list 200': (r) => r.status === 200 });
  sleep(0.1);
}
