// k6 压测脚本 — MiniCC 核心 API
// 运行：k6 run scripts/stress-test.js
// 安装 k6：https://k6.io/docs/getting-started/installation/

import http from 'k6/http'
import { check, sleep, group } from 'k6'
import { Rate, Trend } from 'k6/metrics'

// ── 压测配置 ──
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080'
const ADMIN_EMAIL = __ENV.ADMIN_EMAIL || 'admin@minicc.local'
const ADMIN_PASS = __ENV.ADMIN_PASS || 'Admin123456'

// 自定义指标
const enterpriseApiErrors = new Rate('enterprise_api_errors')
const loginDuration = new Trend('login_duration', true)

// 压测场景：阶梯式加压
export const options = {
  stages: [
    { duration: '30s', target: 20 },   // 预热：20 VU
    { duration: '1m', target: 50 },   // 正常负载：50 VU
    { duration: '30s', target: 100 }, // 峰值：100 VU
    { duration: '30s', target: 0 },   // 降温
  ],
  thresholds: {
    // SLO：99% 请求 < 500ms，错误率 < 5%
    http_req_duration: ['p(99)<500'],
    http_req_failed: ['rate<0.05'],
    enterprise_api_errors: ['rate<0.05'],
  },
}

// ── 登录获取 cookie ──
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

// ── 主测试循环 ──
export default function () {
  const cookies = login()

  group('企业功能 API', () => {
    // 审计日志
    const auditRes = http.get(`${BASE_URL}/v1/ent/audit?limit=20`, { cookies })
    enterpriseApiErrors.add(auditRes.status !== 200)
    check(auditRes, { 'audit 200': (r) => r.status === 200 })

    // 角色列表
    const rolesRes = http.get(`${BASE_URL}/v1/ent/roles`, { cookies })
    enterpriseApiErrors.add(rolesRes.status !== 200)

    // 群组列表
    const groupsRes = http.get(`${BASE_URL}/v1/ent/groups`, { cookies })
    enterpriseApiErrors.add(groupsRes.status !== 200)

    // 成本汇总
    const costRes = http.get(`${BASE_URL}/v1/ent/cost/summary`, { cookies })
    enterpriseApiErrors.add(costRes.status !== 200)

    // 配额池
    const quotaRes = http.get(`${BASE_URL}/v1/ent/quotas`, { cookies })
    enterpriseApiErrors.add(quotaRes.status !== 200)

    // 隐私配置
    const privacyRes = http.get(`${BASE_URL}/v1/ent/privacy`, { cookies })
    enterpriseApiErrors.add(privacyRes.status !== 200)

    // 模型策略
    const policyRes = http.get(`${BASE_URL}/v1/ent/model-policies`, { cookies })
    enterpriseApiErrors.add(policyRes.status !== 200)

    // 能力市场
    const marketRes = http.get(`${BASE_URL}/v1/ent/market/items`, { cookies })
    enterpriseApiErrors.add(marketRes.status !== 200)
  })

  group('系统 API', () => {
    const healthRes = http.get(`${BASE_URL}/health`)
    check(healthRes, { 'health 200': (r) => r.status === 200 })

    const metricsRes = http.get(`${BASE_URL}/metrics`)
    check(metricsRes, { 'metrics 200': (r) => r.status === 200 })
  })

  sleep(0.5) // 模拟用户思考时间
}

// ── 压测结束总结 ──
export function handleSummary(data) {
  return {
    'scripts/stress-test-report.json': JSON.stringify(data, null, 2),
    stdout: `
═══════════════════════════════════════════════════════════
  MiniCC 压测报告
═══════════════════════════════════════════════════════════
  总请求数: ${data.metrics.http_reqs.count}
  错误率: ${(data.metrics.http_req_failed.rate * 100).toFixed(2)}%
  平均响应: ${data.metrics.http_req_duration.avg.toFixed(0)}ms
  P95 响应: ${data.metrics.http_req_duration['p(95)'].toFixed(0)}ms
  P99 响应: ${data.metrics.http_req_duration['p(99)'].toFixed(0)}ms
  企业 API 错误率: ${(data.metrics.enterprise_api_errors.rate * 100).toFixed(2)}%
  登录平均耗时: ${data.metrics.login_duration.avg.toFixed(0)}ms
═══════════════════════════════════════════════════════════
`,
  }
}
