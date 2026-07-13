import { check, sleep } from 'k6';
import http from 'k6/http';
import { baseURL, defaultThresholds } from '../lib/config.js';
import { login } from '../lib/auth.js';

export const options = {
  scenarios: {
    read_mix: {
      executor: 'constant-vus',
      vus: Number(__ENV.K6_VUS || 3),
      duration: __ENV.K6_DURATION || '1m',
    },
  },
  thresholds: defaultThresholds,
};

const endpoints = [
  { path: '/api/estimates?summary=1', name: 'estimates_list' },
  { path: '/api/objects', name: 'objects_list' },
  { path: '/api/constructions', name: 'constructions_list' },
  { path: '/api/healthz', name: 'healthz' },
];

export function setup() {
  login();
}

export default function () {
  const target = endpoints[Math.floor(Math.random() * endpoints.length)];
  const res = http.get(`${baseURL}${target.path}`, {
    tags: { name: target.name },
  });

  check(res, {
    [`${target.name} status 2xx`]: (r) => r.status >= 200 && r.status < 300,
  });

  sleep(Number(__ENV.K6_SLEEP || 0.5));
}
