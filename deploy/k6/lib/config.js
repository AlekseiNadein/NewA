export const baseURL = __ENV.K6_BASE_URL || 'http://localhost:8080';

export const credentials = {
  companyName: __ENV.K6_COMPANY_NAME || 'Система',
  name: __ENV.K6_USER_NAME || 'nadein.av@yandex.ru',
  password: __ENV.K6_PASSWORD || 'admin123',
};

export const prometheusRW =
  __ENV.K6_PROMETHEUS_RW_URL ||
  'http://host.docker.internal:9093/api/v1/write';

export const defaultThresholds = {
  http_req_failed: ['rate<0.05'],
  http_req_duration: ['p(95)<5000'],
  checks: ['rate>0.95'],
};
