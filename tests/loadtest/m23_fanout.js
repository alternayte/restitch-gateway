// M23 fan-out load scenario.
//
// Drives the /fanout composition, which calls five upstreams per request.
// Used twice by scripts/gates/m23.sh: once against a gateway configured with
// max_idle_conns_per_host: 2 (Go's old default) and once with 100, to prove
// connection pooling reduces TCP connection churn.
//
// GW_URL is injected by the gate via the environment.
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  vus: 50,
  duration: '20s',
  // Thresholds are asserted by the gate from the exported summary, not here,
  // so that the baseline run can complete without a non-zero k6 exit code.
  thresholds: {},
};

const GW_URL = __ENV.GW_URL;

export default function () {
  const res = http.get(`${GW_URL}/fanout`);
  check(res, { 'status is 200': (r) => r.status === 200 });
}
