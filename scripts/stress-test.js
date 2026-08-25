// k6 鍘嬫祴鑴氭湰 鈥?chiron 鏍稿績 API
// 杩愯锛歬6 run scripts/stress-test.js
// 瀹夎 k6锛歨ttps://k6.io/docs/getting-started/installation/

import http from 'k6/http'
import { check, sleep, group } from 'k6'
import { Rate, Trend } from 'k6/metrics'

// 鈹€鈹€ 鍘嬫祴閰嶇疆 鈹€鈹€
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080'
const ADMIN_EMAIL = __ENV.ADMIN_EMAIL || 'admin@chiron.local'
const ADMIN_PASS = __ENV.ADMIN_PASS || 'Admin123456'

// 鑷畾涔夋寚鏍?const enterpriseApiErrors = new Rate('enterprise_api_errors')
const loginDuration = new Trend('login_duration', true)

// 鍘嬫祴鍦烘櫙锛氶樁姊紡鍔犲帇
export const options = {
  stages: [
    { duration: '30s', target: 20 },   // 棰勭儹锛?0 VU
    { duration: '1m', target: 50 },   // 姝ｅ父璐熻浇锛?0 VU
    { duration: '30s', target: 100 }, // 宄板€硷細100 VU
    { duration: '30s', target: 0 },   // 闄嶆俯
  ],
  thresholds: {
    // SLO锛?9% 璇锋眰 < 500ms锛岄敊璇巼 < 5%
    http_req_duration: ['p(99)<500'],
    http_req_failed: ['rate<0.05'],
    enterprise_api_errors: ['rate<0.05'],
  },
}

// 鈹€鈹€ 鐧诲綍鑾峰彇 cookie 鈹€鈹€
function login() {
  const res = http.post(
    `${BASE_URL}/v1/auth/login`,
    JSON.stringify({ email: ADMIN_EMAIL, password: ADMIN_PASS }),
    { headers: { 'Content-Type': 'application/json' } }
  )
  loginDuration.add(res.timings.duration)
  check(res, {
    'login 200': (r) => r.status === 200,
  })
  return res.cookies
}

// 鈹€鈹€ 涓绘祴璇曞惊鐜?鈹€鈹€
export default function () {
  const cookies = login()

  group('浼佷笟鍔熻兘 API', () => {
    // 瀹¤鏃ュ織
    const auditRes = http.get(`${BASE_URL}/v1/ent/audit?limit=20`, { cookies })
    enterpriseApiErrors.add(auditRes.status !== 200)
    check(auditRes, { 'audit 200': (r) => r.status === 200 })

    // 瑙掕壊鍒楄〃
    const rolesRes = http.get(`${BASE_URL}/v1/ent/roles`, { cookies })
    enterpriseApiErrors.add(rolesRes.status !== 200)

    // 缇ょ粍鍒楄〃
    const groupsRes = http.get(`${BASE_URL}/v1/ent/groups`, { cookies })
    enterpriseApiErrors.add(groupsRes.status !== 200)

    // 鎴愭湰姹囨€?    const costRes = http.get(`${BASE_URL}/v1/ent/cost/summary`, { cookies })
    enterpriseApiErrors.add(costRes.status !== 200)

    // 閰嶉姹?    const quotaRes = http.get(`${BASE_URL}/v1/ent/quotas`, { cookies })
    enterpriseApiErrors.add(quotaRes.status !== 200)

    // 闅愮閰嶇疆
    const privacyRes = http.get(`${BASE_URL}/v1/ent/privacy`, { cookies })
    enterpriseApiErrors.add(privacyRes.status !== 200)

    // 妯″瀷绛栫暐
    const policyRes = http.get(`${BASE_URL}/v1/ent/model-policies`, { cookies })
    enterpriseApiErrors.add(policyRes.status !== 200)

    // 鑳藉姏甯傚満
    const marketRes = http.get(`${BASE_URL}/v1/ent/market/items`, { cookies })
    enterpriseApiErrors.add(marketRes.status !== 200)
  })

  group('绯荤粺 API', () => {
    const healthRes = http.get(`${BASE_URL}/health`)
    check(healthRes, { 'health 200': (r) => r.status === 200 })

    const metricsRes = http.get(`${BASE_URL}/metrics`)
    check(metricsRes, { 'metrics 200': (r) => r.status === 200 })
  })

  sleep(0.5) // 妯℃嫙鐢ㄦ埛鎬濊€冩椂闂?}

// 鈹€鈹€ 鍘嬫祴缁撴潫鎬荤粨 鈹€鈹€
export function handleSummary(data) {
  return {
    'scripts/stress-test-report.json': JSON.stringify(data, null, 2),
    stdout: `
鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?  chiron 鍘嬫祴鎶ュ憡
鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?  鎬昏姹傛暟: ${data.metrics.http_reqs.count}
  閿欒鐜? ${(data.metrics.http_req_failed.rate * 100).toFixed(2)}%
  骞冲潎鍝嶅簲: ${data.metrics.http_req_duration.avg.toFixed(0)}ms
  P95 鍝嶅簲: ${data.metrics.http_req_duration['p(95)'].toFixed(0)}ms
  P99 鍝嶅簲: ${data.metrics.http_req_duration['p(99)'].toFixed(0)}ms
  浼佷笟 API 閿欒鐜? ${(data.metrics.enterprise_api_errors.rate * 100).toFixed(2)}%
  鐧诲綍骞冲潎鑰楁椂: ${data.metrics.login_duration.avg.toFixed(0)}ms
鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?`,
  }
}

