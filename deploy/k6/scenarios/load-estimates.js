import { check, sleep } from 'k6';
import http from 'k6/http';
import { baseURL, defaultThresholds } from '../lib/config.js';
import {
  login,
  fetchEstimatesSummary,
  fetchEstimateDetail,
  pickEstimate,
  buildEstimateBody,
} from '../lib/auth.js';

export const options = {
  scenarios: {
    estimate_save: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: Number(__ENV.K6_VUS || 5) },
        { duration: '30s', target: Number(__ENV.K6_VUS || 5) },
        { duration: '10s', target: 0 },
      ],
      gracefulRampDown: '5s',
    },
  },
  thresholds: {
    ...defaultThresholds,
    'http_req_duration{name:estimate_put}': ['p(95)<8000'],
  },
};

export function setup() {
  login();
  const listRes = fetchEstimatesSummary();
  if (listRes.status !== 200) {
    throw new Error(`setup: estimates list failed with status ${listRes.status}`);
  }
  const summary = pickEstimate(listRes.json());
  if (!summary) {
    throw new Error('setup: no estimates found for load test');
  }

  const detailRes = fetchEstimateDetail(summary.id);
  if (detailRes.status !== 200) {
    throw new Error(`setup: estimate detail failed with status ${detailRes.status}`);
  }
  const estimate = detailRes.json();
  return { estimateId: estimate.id, body: buildEstimateBody(estimate) };
}

export default function (data) {
  const res = http.put(`${baseURL}/api/estimates/${data.estimateId}`, data.body, {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'estimate_put' },
  });

  check(res, {
    'estimate put status 200': (r) => r.status === 200,
  });

  sleep(Number(__ENV.K6_SLEEP || 1));
}

export function teardown() {
  sleep(5);
  const health = http.get(`${baseURL}/api/healthz`, {
    tags: { name: 'healthz_teardown' },
  });
  check(health, {
    'pipeline ready after load': (r) => {
      try {
        return r.json('pipelineReady') === true;
      } catch (_) {
        return false;
      }
    },
  });
}
