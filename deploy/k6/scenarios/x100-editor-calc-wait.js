import { check, sleep } from 'k6';
import http from 'k6/http';
import { Trend } from 'k6/metrics';
import { baseURL, defaultThresholds } from '../lib/config.js';
import { login, fetchEstimatesSummary, fetchEstimateDetail } from '../lib/auth.js';

const scenarioDuration = new Trend('x100_editor_calc_duration', true);

export const options = {
  scenarios: {
    x100_editor_calc_wait: {
      executor: 'per-vu-iterations',
      vus: Number(__ENV.K6_VUS || 5),
      iterations: 1,
      maxDuration: __ENV.K6_MAX_DURATION || '45m',
    },
  },
  thresholds: {
    ...defaultThresholds,
    x100_editor_calc_duration: ['max<2700000'],
  },
};

function pickX100Estimates(list) {
  if (!Array.isArray(list)) {
    return [];
  }
  return list
    .filter((item) => item && typeof item.code === 'string' && item.code.toUpperCase().includes('X100'))
    .sort((a, b) => String(a.code).localeCompare(String(b.code), 'ru'));
}

function fetchCalcSummary(estimateId) {
  return http.get(`${baseURL}/api/estimates/${estimateId}/calc-status?summary=1`, {
    tags: { name: 'calc_status_summary' },
  });
}

function acquireEstimateLock(estimateId) {
  return http.put(`${baseURL}/api/estimates/${estimateId}/lock`, null, {
    tags: { name: 'estimate_lock_put' },
  });
}

function cancelEstimateCalc(estimateId) {
  return http.post(`${baseURL}/api/estimates/${estimateId}/calc/cancel`, null, {
    tags: { name: 'estimate_calc_cancel' },
  });
}

function startEstimateCalc(estimateId) {
  return http.post(`${baseURL}/api/estimates/${estimateId}/calc`, null, {
    tags: { name: 'estimate_calc_start' },
  });
}

function waitForCalcCompletion(estimateId, requireRestart) {
  const pollSec = Number(__ENV.K6_CALC_POLL_SEC || 2);
  const timeoutSec = Number(__ENV.K6_CALC_TIMEOUT_SEC || 2700);
  const started = Date.now();

  if (requireRestart) {
    const restartDeadline = started + 60000;
    while (Date.now() < restartDeadline) {
      const res = fetchCalcSummary(estimateId);
      if (res.status === 200) {
        try {
          const summary = res.json();
          const total = Number(summary.total) || 0;
          const processed = Number(summary.processed) || 0;
          if (total > 0 && processed < total) {
            break;
          }
        } catch (_) {
          // ignore parse errors while restart is propagating
        }
      }
      sleep(pollSec);
    }
  }

  while ((Date.now() - started) / 1000 < timeoutSec) {
    const res = fetchCalcSummary(estimateId);
    if (res.status !== 200) {
      sleep(pollSec);
      continue;
    }

    let summary;
    try {
      summary = res.json();
    } catch (_) {
      sleep(pollSec);
      continue;
    }

    const total = Number(summary.total) || 0;
    const processed = Number(summary.processed) || 0;
    const errors = Number(summary.errors) || 0;
    const grandTotal = Number(summary.grandTotal) || 0;

    if (total > 0 && processed >= total) {
      return {
        done: true,
        total,
        processed,
        errors,
        grandTotal,
        waitMs: Date.now() - started,
      };
    }

    sleep(pollSec);
  }

  const finalRes = fetchCalcSummary(estimateId);
  let summary = {};
  try {
    summary = finalRes.json();
  } catch (_) {
    summary = {};
  }

  return {
    done: false,
    total: Number(summary.total) || 0,
    processed: Number(summary.processed) || 0,
    errors: Number(summary.errors) || 0,
    grandTotal: Number(summary.grandTotal) || 0,
    waitMs: Date.now() - started,
  };
}

export default function () {
  const startedAt = Date.now();

  login();

  const listRes = fetchEstimatesSummary();
  check(listRes, { 'estimates summary status 200': (r) => r.status === 200 });
  const x100 = pickX100Estimates(listRes.status === 200 ? listRes.json() : []);
  check(x100, { 'x100 estimates exist': (items) => items.length >= Number(__ENV.K6_VUS || 5) });
  if (x100.length === 0) {
    return;
  }

  const idx = Math.min(__VU - 1, x100.length - 1);
  const estimate = x100[idx];

  check(http.get(`${baseURL}/api/constructions`, { tags: { name: 'constructions_list' } }), {
    'constructions status 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  check(http.get(`${baseURL}/api/objects`, { tags: { name: 'objects_list' } }), {
    'objects status 2xx': (r) => r.status >= 200 && r.status < 300,
  });

  const detailRes = fetchEstimateDetail(estimate.id);
  check(detailRes, {
    'estimate detail status 200': (r) => r.status === 200,
    'estimate detail has items': (r) => {
      try {
        return Array.isArray(r.json('items'));
      } catch (_) {
        return false;
      }
    },
  });

  const lockRes = acquireEstimateLock(estimate.id);
  check(lockRes, {
    'estimate lock acquired': (r) => r.status === 200 || r.status === 409,
  });

  if (__ENV.K6_FORCE_RECALC === '1') {
    const cancelRes = cancelEstimateCalc(estimate.id);
    check(cancelRes, {
      'estimate calc cancelled': (r) => r.status === 200,
    });
    sleep(2);
  }

  const calcStartedAt = Date.now();
  const calcRes = startEstimateCalc(estimate.id);
  check(calcRes, {
    'estimate calc started': (r) => r.status === 202 || r.status === 200,
  });

  const calcResult = waitForCalcCompletion(estimate.id, __ENV.K6_FORCE_RECALC === '1');
  const calcWaitMs = Date.now() - calcStartedAt;
  const totalMs = Date.now() - startedAt;
  scenarioDuration.add(totalMs);

  check(calcResult, {
    'calc completed': (r) => r.done === true,
  });

  // Single-line key=value format: survives k6 log wrapping (unlike pretty JSON).
  console.log(
    [
      'X100_RESULT',
      `vu=${__VU}`,
      `code=${estimate.code}`,
      `id=${estimate.id}`,
      `lines=${calcResult.processed}/${calcResult.total}`,
      `errors=${calcResult.errors}`,
      `grandTotal=${calcResult.grandTotal}`,
      `calcWaitMs=${calcWaitMs}`,
      `durationMs=${totalMs}`,
      `done=${calcResult.done}`,
    ].join(' '),
  );
}
