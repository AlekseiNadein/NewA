import { check } from 'k6';
import http from 'k6/http';
import { baseURL, defaultThresholds } from '../lib/config.js';
import { login, fetchEstimatesSummary, fetchEstimateDetail } from '../lib/auth.js';

export const options = {
  scenarios: {
    open_x100_editor: {
      executor: 'per-vu-iterations',
      vus: Number(__ENV.K6_VUS || 5),
      iterations: 1,
      maxDuration: '2m',
    },
  },
  thresholds: {
    ...defaultThresholds,
    'http_req_duration{name:estimates_list_summary}': ['p(95)<3000'],
    'http_req_duration{name:estimate_get}': ['p(95)<8000'],
  },
};

export default function () {
  login();
  const listRes = fetchEstimatesSummary();
  check(listRes, {
    'estimates summary status 200': (r) => r.status === 200,
  });
  const list = listRes.status === 200 ? listRes.json() : [];
  const x100 = Array.isArray(list)
    ? list.filter((item) => item && typeof item.code === 'string' && item.code.toUpperCase().includes('X100'))
    : [];
  check(x100, {
    'x100 estimates exist': (items) => Array.isArray(items) && items.length > 0,
  });
  if (x100.length === 0) {
    return;
  }
  const idx = ((__VU - 1) % x100.length + x100.length) % x100.length;
  const estimate = x100[idx];

  const constructionsRes = http.get(`${baseURL}/api/constructions`, { tags: { name: 'constructions_list' } });
  check(constructionsRes, {
    'constructions status 2xx': (r) => r.status >= 200 && r.status < 300,
  });

  const objectsRes = http.get(`${baseURL}/api/objects`, { tags: { name: 'objects_list' } });
  check(objectsRes, {
    'objects status 2xx': (r) => r.status >= 200 && r.status < 300,
  });

  const detailRes = fetchEstimateDetail(estimate.id);
  check(detailRes, {
    'estimate detail status 200': (r) => r.status === 200,
    'estimate detail has items': (r) => {
      try {
        const items = r.json('items');
        return Array.isArray(items);
      } catch (_) {
        return false;
      }
    },
  });
}
