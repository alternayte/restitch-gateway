// M24 baseline load scenario.
//
// PLAN.md phrases the target as "~1000 RPS (50 VUs)". Those are different
// things in k6: 50 VUs in an open loop produce whatever throughput the system
// allows, so if the gateway slows down the offered load drops with it and the
// test quietly stops measuring what it was built to measure.
// constant-arrival-rate holds the request rate fixed and treats VUs as a pool.
//
// Every knob is an env var so one script serves both the gate (full profile)
// and CI (reduced profile on a shared runner).
import http from 'k6/http';
import { check } from 'k6';

const GW_URL = __ENV.GW_URL;
const STUDIO_URL = __ENV.STUDIO_URL;
const DURATION = __ENV.DURATION || '60s';

if (!GW_URL) {
  throw new Error('GW_URL is required');
}

export const options = {
  scenarios: {
    compositions: {
      executor: 'constant-arrival-rate',
      exec: 'composition',
      rate: Number(__ENV.RATE || 1000),
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: Number(__ENV.VUS || 50),
      maxVUs: Number(__ENV.MAX_VUS || 200),
    },
  },
  thresholds: {
    // Sub-metric thresholds also CREATE the sub-metrics, which is what makes
    // the per-scenario keys available in handleSummary below.
    'http_req_duration{scenario:compositions}': [`p(95)<${__ENV.P95_MS || 1000}`],
    'http_req_failed{scenario:compositions}': [`rate<${__ENV.ERR_RATE || 0.01}`],
    'http_reqs{scenario:compositions}': ['count>0'],
  },
};

// Studio is the CONTROL plane — config CRUD and registry bundles. Driving it
// at 1000 RPS would measure SQLite write contention rather than anything an
// operator experiences, and would dominate the failure signal. 5 req/s proves
// it stays responsive while the data plane is saturated.
//
// The scenario is conditional because constant-arrival-rate rejects rate: 0,
// so it cannot be switched off by zeroing the rate. The gate sets STUDIO_URL;
// the gateway-only CI job leaves it unset.
if (STUDIO_URL) {
  options.scenarios.studio_api = {
    executor: 'constant-arrival-rate',
    exec: 'studio',
    rate: Number(__ENV.STUDIO_RATE || 5),
    timeUnit: '1s',
    duration: DURATION,
    preAllocatedVUs: 2,
    maxVUs: 10,
  };
  // These thresholds exist primarily to MATERIALISE the studio sub-metrics.
  // k6 only creates a sub-metric when a threshold references it, so without
  // these the handleSummary lookups below fall back to -1/0 and the evidence
  // reads as though the studio scenario served nothing. The p95 bound is
  // deliberately generous — this is a liveness check on the control plane,
  // not a latency SLO, and a tight bound here would be a flake source.
  options.thresholds['http_reqs{scenario:studio_api}'] = ['count>0'];
  options.thresholds['http_req_duration{scenario:studio_api}'] = [
    `p(95)<${__ENV.STUDIO_P95_MS || 5000}`,
  ];
}

export function composition() {
  const res = http.get(GW_URL);
  check(res, { 'composition 200': (r) => r.status === 200 });
}

export function studio() {
  const res = http.get(STUDIO_URL);
  check(res, { 'studio 200': (r) => r.status === 200 });
}

// Write a normalised summary the gate parses by key name. Deriving this
// ourselves rather than using --summary-export means the gate never has to
// guess how k6 spells its per-scenario sub-metric keys.
export function handleSummary(data) {
  const m = data.metrics;
  // pick() has TWO jobs, not one. Its fallback only fires when the
  // sub-metric key is entirely absent (e.g. STUDIO_URL unset, so the
  // threshold that materialises it was never registered). It does NOT
  // detect a scenario that ran but sampled nothing: thresholds materialise
  // http_req_duration{...} and http_req_failed{...} with a value of 0
  // regardless of sample count, so a zero-iteration run reads as
  // p95_ms: 0 and error_rate: 0 — indistinguishable from "everything was
  // instant and nothing failed". reqCountFailingValue() closes that gap by
  // checking the corresponding reqs count and forcing the FAILING value
  // (-1 / 1) whenever it is 0, so p95_ms and error_rate are guaranteed to
  // be genuine measurements whenever they are not the documented failing
  // sentinel.
  const pick = (name, field, fallback) => {
    const metric = m[name];
    if (!metric || !metric.values || metric.values[field] === undefined) {
      return fallback;
    }
    return metric.values[field];
  };
  const reqCountFailingValue = (reqsName, name, field, fallback, failing) => {
    const reqs = pick(reqsName, 'count', 0);
    if (reqs === 0) {
      return failing;
    }
    return pick(name, field, fallback);
  };

  const out = {
    compositions: {
      p95_ms: reqCountFailingValue(
        'http_reqs{scenario:compositions}',
        'http_req_duration{scenario:compositions}',
        'p(95)',
        -1,
        -1
      ),
      error_rate: reqCountFailingValue(
        'http_reqs{scenario:compositions}',
        'http_req_failed{scenario:compositions}',
        'rate',
        1,
        1
      ),
      reqs: pick('http_reqs{scenario:compositions}', 'count', 0),
    },
  };

  if (STUDIO_URL) {
    out.studio = {
      p95_ms: reqCountFailingValue(
        'http_reqs{scenario:studio_api}',
        'http_req_duration{scenario:studio_api}',
        'p(95)',
        -1,
        -1
      ),
      reqs: pick('http_reqs{scenario:studio_api}', 'count', 0),
    };
  }

  const path = __ENV.SUMMARY_OUT || 'm24_summary.json';
  const result = {};
  result[path] = JSON.stringify(out, null, 2);
  result.stdout = `\nM24 summary: ${JSON.stringify(out)}\n`;
  return result;
}
