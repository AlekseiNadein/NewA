import http from 'k6/http';
import { check } from 'k6';
import { baseURL, credentials } from './config.js';

export function login() {
  const res = http.post(
    `${baseURL}/api/auth/login`,
    JSON.stringify({
      companyName: credentials.companyName,
      name: credentials.name,
      password: credentials.password,
    }),
    { headers: { 'Content-Type': 'application/json' }, tags: { name: 'auth_login' } },
  );

  check(res, {
    'login status 200': (r) => r.status === 200,
    'login returns user': (r) => {
      try {
        return Boolean(r.json('user'));
      } catch (_) {
        return false;
      }
    },
  });

  return res;
}

export function fetchEstimatesSummary() {
  return http.get(`${baseURL}/api/estimates?summary=1`, {
    tags: { name: 'estimates_list_summary' },
  });
}

export function fetchEstimateDetail(estimateId) {
  return http.get(`${baseURL}/api/estimates/${estimateId}`, {
    tags: { name: 'estimate_get' },
  });
}

export function pickEstimate(estimates) {
  if (!Array.isArray(estimates) || estimates.length === 0) {
    return null;
  }
  const withCode = estimates.find((item) => item && item.code);
  return withCode || estimates[0];
}

export function buildEstimateBody(estimate) {
  return JSON.stringify({
    objectId: estimate.objectId,
    code: estimate.code,
    title: estimate.title,
    description: estimate.description,
    district: estimate.district,
    fgisSetId: estimate.fgisSetId,
    status: estimate.status,
    items: estimate.items,
  });
}
