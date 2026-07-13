import { check, sleep } from 'k6';
import { baseURL, defaultThresholds } from '../lib/config.js';
import { login, fetchEstimatesSummary } from '../lib/auth.js';
import http from 'k6/http';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: defaultThresholds,
};

export default function () {
  const health = http.get(`${baseURL}/api/healthz`, {
    tags: { name: 'healthz' },
  });
  check(health, {
    'healthz status 200': (r) => r.status === 200,
    'healthz has queue': (r) => {
      try {
        return Boolean(r.json('queue'));
      } catch (_) {
        return false;
      }
    },
  });

  login();

  const estimatesRes = fetchEstimatesSummary();
  check(estimatesRes, {
    'estimates status 200': (r) => r.status === 200,
    'estimates not empty': (r) => {
      try {
        const list = r.json();
        return Array.isArray(list) && list.length > 0;
      } catch (_) {
        return false;
      }
    },
  });

  sleep(1);
}
