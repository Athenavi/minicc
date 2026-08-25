<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Card, Row, Col, Form, FormItem, InputNumber, Input, InputPassword, Select, Button, Switch, Slider, message } from 'ant-design-vue'
import { saveSettings, getSettings } from '@/api/admin'

const saving = ref(false)
const loading = ref(false)

const rateLimitConfig = ref({
  global: 1000,
  tenant: 500,
  user: 100,
})

const agentConfig = ref({
  max_turns: 10,
  max_tokens: 4096,
  context_limit: 20,
})

const llmConfig = ref({
  provider: 'openai',
  model: 'gpt-4o',
})

const storageConfig = ref({
  backend: 'local',
  root: './workspace',
})

const paymentConfig = ref({
  public_base_url: '',
  alipay_gateway: '',
})

// 杩炴帴涓庡瘑閽ラ厤缃紙鏁忔劅鍊肩敱 APP_SECRET 娲剧敓瀵嗛挜鍔犲瘑鍏ュ簱锛?
const redisConfig = ref({
  addr: 'localhost:6379',
  password: '',
  db: 0,
})

const postgresConfig = ref({
  dsn: '',
})

const corsConfig = ref({
  origins: '',
})

const s3Config = ref({
  endpoint: '',
  bucket: '',
  access_key: '',
  secret_key: '',
  use_ssl: false,
})

// Python AI 寮曟搸閰嶇疆锛堜笅鍙戝埌寮曟搸锛沘pi_key 绛夋晱鎰熷€煎姞瀵嗗叆搴擄級
const pythonConfig = ref({
  llm_provider: 'openai',
  llm_model: '',
  llm_api_key: '',
  llm_base_url: '',
  embedding_model: '',
  max_turns: 10,
  queue_worker_concurrency: 10,
  cache_l1_capacity: 2048,
})

const degradationConfig = ref({
  enabled: true,
  lightThreshold: 500000,
  mediumThreshold: 700000,
  heavyThreshold: 900000,
  vipPriority: true,
})

const cacheConfig = ref({
  l1Capacity: 2048,
  l2Ttl: 3600,
  semanticThreshold: 0.95,
  prefetchEnabled: true,
})

const apiKeyConfig = ref({
  circuitBreakerThreshold: 5,
  recoveryTimeout: 60,
  weightDecay: 0.5,
  autoRecovery: true,
})

const nginxConfig = `# /etc/nginx/nginx.conf
user nginx;
worker_processes auto;
worker_rlimit_nofile 2097152;

events {
    worker_connections 1048576;
    use epoll;
    multi_accept on;
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    keepalive_requests 1000;
    
    client_body_buffer_size 16K;
    client_header_buffer_size 1k;
    client_max_body_size 8m;
    large_client_header_buffers 4 8k;
    
    client_body_timeout 12;
    client_header_timeout 12;
    send_timeout 10;
    
    upstream go_gateway {
        least_conn;
        server 127.0.0.1:8080;
        server 127.0.0.1:8081;
        server 127.0.0.1:8082;
        server 127.0.0.1:8083;
        keepalive 1000;
    }
    
    limit_req_zone $binary_remote_addr zone=api:10m rate=100r/s;
    limit_conn_zone $binary_remote_addr zone=conn:10m;
    
    server {
        listen 80;
        listen 443 ssl http2;
        
        ssl_certificate /etc/nginx/ssl/cert.pem;
        ssl_certificate_key /etc/nginx/ssl/key.pem;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 10m;
        
        limit_req zone=api burst=200 nodelay;
        limit_conn conn 100;
        
        location /v1/agent/stream {
            proxy_pass http://go_gateway;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_buffering off;
            proxy_cache off;
            proxy_read_timeout 86400s;
            proxy_send_timeout 86400s;
        }
        
        location /v1/ {
            proxy_pass http://go_gateway;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_connect_timeout 5s;
            proxy_read_timeout 30s;
            proxy_send_timeout 30s;
        }
    }
}`

const kernelConfig = `# /etc/sysctl.conf

# 鏂囦欢鎻忚堪绗?
fs.file-max = 2097152
fs.nr_open = 2097152

# TCP 杩炴帴
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_keepalive_time = 600
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 3

# 绔彛鑼冨洿
net.ipv4.ip_local_port_range = 1024 65535

# 鍐呭瓨
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216

# 搴旂敤閰嶇疆
# /etc/security/limits.conf
* soft nofile 2097152
* hard nofile 2097152
* soft nproc 65535
* hard nproc 65535`

async function saveRateLimit() {
  saving.value = true
  try {
    await saveSettings('rate_limit', rateLimitConfig.value)
    message.success('闄愭祦閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveDegradation() {
  saving.value = true
  try {
    await saveSettings('degradation', degradationConfig.value)
    message.success('闄嶇骇閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveCache() {
  saving.value = true
  try {
    await saveSettings('cache', cacheConfig.value)
    message.success('缂撳瓨閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveApiKey() {
  saving.value = true
  try {
    await saveSettings('api_key', apiKeyConfig.value)
    message.success('API Key 閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveAgent() {
  saving.value = true
  try {
    await saveSettings('agent', agentConfig.value)
    message.success('Agent 閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveLlm() {
  saving.value = true
  try {
    await saveSettings('llm', llmConfig.value)
    message.success('妯″瀷閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveStorage() {
  saving.value = true
  try {
    await saveSettings('storage', storageConfig.value)
    message.success('瀛樺偍閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function savePayment() {
  saving.value = true
  try {
    await saveSettings('payment', paymentConfig.value)
    message.success('鏀粯閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveRedis() {
  saving.value = true
  try {
    await saveSettings('redis', redisConfig.value)
    message.success('Redis 閰嶇疆宸蹭繚瀛樺苟鐑洿鏂拌繛鎺?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function savePostgres() {
  saving.value = true
  try {
    await saveSettings('postgres', postgresConfig.value)
    message.success('鏁版嵁搴撻厤缃凡淇濆瓨锛堥噸鍚悗鐢熸晥锛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveCors() {
  saving.value = true
  try {
    await saveSettings('cors', corsConfig.value)
    message.success('CORS 閰嶇疆宸蹭繚瀛橈紙閲嶅惎鍚庣敓鏁堬級')
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function saveS3() {
  saving.value = true
  try {
    await saveSettings('s3', s3Config.value)
    message.success('瀵硅薄瀛樺偍閰嶇疆宸蹭繚瀛?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

async function savePython() {
  saving.value = true
  try {
    await saveSettings('python', pythonConfig.value)
    message.success('Python 寮曟搸閰嶇疆宸蹭繚瀛橈紝寮曟搸閲嶅惎鍚庣敓鏁?)
  } catch (err: any) {
    message.error('淇濆瓨澶辫触: ' + (err.message || '鏈煡閿欒'))
  } finally {
    saving.value = false
  }
}

const copyNginx = () => {
  navigator.clipboard.writeText(nginxConfig)
  message.success('宸插鍒跺埌鍓创鏉?)
}

const copyKernel = () => {
  navigator.clipboard.writeText(kernelConfig)
  message.success('宸插鍒跺埌鍓创鏉?)
}

// S 淇锛氬姞杞藉凡鎸佷箙鍖栫殑鐪熷疄閰嶇疆锛堜笉鍐嶄互鍐欐鐨勭ず渚嬮粯璁ゅ€艰鐩栫嚎涓婇厤缃級銆?
// 鎸夎繑鍥炵殑 key 瑕嗙洊榛樿鍊硷紱鍚庣鏃犺褰曟椂淇濈暀榛樿锛堟湰鍦颁负绌烘€侊級銆?
function mergeConfig(target: { value: Record<string, any> }, saved: Record<string, any>) {
  for (const k of Object.keys(target.value)) {
    if (saved[k] !== undefined) target.value[k] = saved[k]
  }
}

onMounted(async () => {
  loading.value = true
  try {
    mergeConfig(rateLimitConfig, await getSettings('rate_limit'))
    mergeConfig(degradationConfig, await getSettings('degradation'))
    mergeConfig(cacheConfig, await getSettings('cache'))
    mergeConfig(apiKeyConfig, await getSettings('api_key'))
    mergeConfig(agentConfig, await getSettings('agent'))
    mergeConfig(llmConfig, await getSettings('llm'))
    mergeConfig(storageConfig, await getSettings('storage'))
    mergeConfig(paymentConfig, await getSettings('payment'))
    mergeConfig(redisConfig, await getSettings('redis'))
    mergeConfig(postgresConfig, await getSettings('postgres'))
    mergeConfig(corsConfig, await getSettings('cors'))
    mergeConfig(s3Config, await getSettings('s3'))
    mergeConfig(pythonConfig, await getSettings('python'))
  } catch {
    // 鎷夊彇澶辫触淇濈暀榛樿鍊硷紝涓嶉樆鏂〉闈?
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="settings">
    <Row :gutter="16">
      <!-- 闄愭祦閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="闄愭祦閰嶇疆">
          <Form :model="rateLimitConfig" layout="vertical">
            <FormItem label="鍏ㄥ眬锛堟瘡鍒嗛挓锛?>
              <InputNumber v-model:value="rateLimitConfig.global" :min="100" :max="1000000" style="width: 100%" />
            </FormItem>
            <FormItem label="鍗曠鎴凤紙姣忓垎閽燂級">
              <InputNumber v-model:value="rateLimitConfig.tenant" :min="10" :max="100000" style="width: 100%" />
            </FormItem>
            <FormItem label="鍗曠敤鎴凤紙姣忓垎閽燂級">
              <InputNumber v-model:value="rateLimitConfig.user" :min="1" :max="10000" style="width: 100%" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveRateLimit">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍚庡嵆鍒荤敓鏁堬紙鐑洿鏂帮級銆?/div>
        </Card>
      </Col>

      <!-- 闄嶇骇閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="闄嶇骇閰嶇疆">
          <Form :model="degradationConfig" layout="vertical">
            <FormItem label="鍚敤闄嶇骇">
              <Switch v-model:checked="degradationConfig.enabled" />
            </FormItem>
            <FormItem label="杞诲害杩囪浇闃堝€?>
              <InputNumber v-model:value="degradationConfig.lightThreshold" :min="10000" :max="1000000" style="width: 100%" />
            </FormItem>
            <FormItem label="涓害杩囪浇闃堝€?>
              <InputNumber v-model:value="degradationConfig.mediumThreshold" :min="50000" :max="1000000" style="width: 100%" />
            </FormItem>
            <FormItem label="閲嶅害杩囪浇闃堝€?>
              <InputNumber v-model:value="degradationConfig.heavyThreshold" :min="100000" :max="1000000" style="width: 100%" />
            </FormItem>
            <FormItem label="VIP 浼樺厛">
              <Switch v-model:checked="degradationConfig.vipPriority" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveDegradation">淇濆瓨</Button>
        </Card>
      </Col>

      <!-- 缂撳瓨閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="缂撳瓨閰嶇疆">
          <Form :model="cacheConfig" layout="vertical">
            <FormItem label="L1 瀹归噺">
              <InputNumber v-model:value="cacheConfig.l1Capacity" :min="100" :max="10000" style="width: 100%" />
            </FormItem>
            <FormItem label="L2 TTL">
              <InputNumber v-model:value="cacheConfig.l2Ttl" :min="60" :max="86400" style="width: 100%" />
            </FormItem>
            <FormItem label="璇箟缂撳瓨闃堝€?>
              <Slider v-model:value="cacheConfig.semanticThreshold" :min="0.5" :max="1" :step="0.01" />
            </FormItem>
            <FormItem label="鍚敤棰勫彇">
              <Switch v-model:checked="cacheConfig.prefetchEnabled" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveCache">淇濆瓨</Button>
        </Card>
      </Col>

      <!-- API Key 閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="API Key 閰嶇疆">
          <Form :model="apiKeyConfig" layout="vertical">
            <FormItem label="鐔旀柇闃堝€?>
              <InputNumber v-model:value="apiKeyConfig.circuitBreakerThreshold" :min="1" :max="100" style="width: 100%" />
            </FormItem>
            <FormItem label="鎭㈠瓒呮椂">
              <InputNumber v-model:value="apiKeyConfig.recoveryTimeout" :min="10" :max="3600" style="width: 100%" />
            </FormItem>
            <FormItem label="鏉冮噸琛板噺">
              <Slider v-model:value="apiKeyConfig.weightDecay" :min="0.1" :max="1" :step="0.1" />
            </FormItem>
            <FormItem label="鑷姩鎭㈠">
              <Switch v-model:checked="apiKeyConfig.autoRecovery" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveApiKey">淇濆瓨</Button>
        </Card>
      </Col>
    </Row>

    <!-- 杩佺Щ鑷?.env 鐨勪笟鍔￠厤缃紙鎸佷箙鍖栧埌 DB system_settings锛?-->
    <Row :gutter="16" class="config-row">
      <!-- Agent 閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="Agent 閰嶇疆">
          <Form :model="agentConfig" layout="vertical">
            <FormItem label="鏈€澶ф帹鐞嗚疆鏁?>
              <InputNumber v-model:value="agentConfig.max_turns" :min="1" :max="100" style="width: 100%" />
            </FormItem>
            <FormItem label="姣忔璋冪敤鏈€澶?Token">
              <InputNumber v-model:value="agentConfig.max_tokens" :min="256" :max="32768" style="width: 100%" />
            </FormItem>
            <FormItem label="涓婁笅鏂囨秷鎭暟闄愬埗">
              <InputNumber v-model:value="agentConfig.context_limit" :min="1" :max="100" style="width: 100%" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveAgent">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍒?DB锛岃繍琛屾椂娑堣垂椤归噸鍚悗鐢熸晥銆?/div>
        </Card>
      </Col>

      <!-- LLM / 妯″瀷閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="妯″瀷閰嶇疆">
          <Form :model="llmConfig" layout="vertical">
            <FormItem label="Provider">
              <Select v-model:value="llmConfig.provider" style="width: 100%">
                <Select.Option value="openai">OpenAI</Select.Option>
                <Select.Option value="anthropic">Anthropic</Select.Option>
                <Select.Option value="deepseek">DeepSeek</Select.Option>
              </Select>
            </FormItem>
            <FormItem label="榛樿妯″瀷">
              <Input v-model:value="llmConfig.model" placeholder="gpt-4o" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveLlm">淇濆瓨</Button>
          <div class="config-note">Provider/Model 宸叉寔涔呭寲鍒?DB锛岄噸鍚敓鏁堬紱瀵嗛挜绫绘晱鎰熷€煎姞瀵嗗叆搴撱€?/div>
        </Card>
      </Col>

      <!-- 瀛樺偍閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="瀛樺偍閰嶇疆">
          <Form :model="storageConfig" layout="vertical">
            <FormItem label="鍚庣绫诲瀷">
              <Select v-model:value="storageConfig.backend" style="width: 100%">
                <Select.Option value="local">鏈湴纾佺洏</Select.Option>
                <Select.Option value="s3">S3 / MinIO</Select.Option>
              </Select>
            </FormItem>
            <FormItem label="瀛樺偍鏍圭洰褰?>
              <Input v-model:value="storageConfig.root" placeholder="./workspace" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveStorage">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍒?DB锛岃繍琛屾椂娑堣垂椤归噸鍚悗鐢熸晥銆?/div>
        </Card>
      </Col>

      <!-- 鏀粯閰嶇疆锛堥潪鏁忔劅椤癸級 -->
      <Col :xs="24" :sm="12">
        <Card title="鏀粯閰嶇疆">
          <Form :model="paymentConfig" layout="vertical">
            <FormItem label="鍏綉鍩虹 URL">
              <Input v-model:value="paymentConfig.public_base_url" placeholder="https://api.example.com" />
            </FormItem>
            <FormItem label="鏀粯瀹濈綉鍏?>
              <Input v-model:value="paymentConfig.alipay_gateway" placeholder="https://openapi.alipay.com/gateway.do" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="savePayment">淇濆瓨</Button>
          <div class="config-note">瀵嗛挜绫诲嚟鎹笉鍏ュ簱锛岀敱鐜鍙橀噺娉ㄥ叆銆?/div>
        </Card>
      </Col>
    </Row>

    <!-- 杩炴帴涓庡瘑閽ラ厤缃紙鏁忔劅鍊?AES-GCM 鍔犲瘑鍏ュ簱锛?-->
    <Row :gutter="16" class="config-row">
      <!-- Redis 閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="Redis 閰嶇疆">
          <Form :model="redisConfig" layout="vertical">
            <FormItem label="鍦板潃">
              <Input v-model:value="redisConfig.addr" placeholder="localhost:6379" />
            </FormItem>
            <FormItem label="瀵嗙爜锛堝姞瀵嗗叆搴擄級">
              <InputPassword v-model:value="redisConfig.password" placeholder="绌哄垯鏃犲瘑鐮? />
            </FormItem>
            <FormItem label="DB">
              <InputNumber v-model:value="redisConfig.db" :min="0" :max="15" style="width: 100%" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveRedis">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍚庣儹鏇存柊杩炴帴锛涘彲鍒囨崲 Redis 闆嗙兢銆?/div>
        </Card>
      </Col>

      <!-- PostgreSQL 閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="鏁版嵁搴擄紙PostgreSQL锛夐厤缃?>
          <Form :model="postgresConfig" layout="vertical">
            <FormItem label="DSN锛堝姞瀵嗗叆搴擄級">
              <InputPassword v-model:value="postgresConfig.dsn" placeholder="postgres://user:pass@host:5432/chiron" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="savePostgres">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍒?DB锛屽垏鎹㈡暟鎹簱闆嗙兢闇€閲嶅惎鐢熸晥銆?/div>
        </Card>
      </Col>

      <!-- CORS 閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="CORS 閰嶇疆">
          <Form :model="corsConfig" layout="vertical">
            <FormItem label="鍏佽鏉ユ簮锛堥€楀彿鍒嗛殧锛?>
              <Input v-model:value="corsConfig.origins" placeholder="http://localhost:5173,https://app.example.com" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveCors">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍚庨噸鍚敓鏁堛€?/div>
        </Card>
      </Col>

      <!-- S3 / MinIO 閰嶇疆 -->
      <Col :xs="24" :sm="12">
        <Card title="瀵硅薄瀛樺偍锛圫3/MinIO锛夐厤缃?>
          <Form :model="s3Config" layout="vertical">
            <FormItem label="Endpoint">
              <Input v-model:value="s3Config.endpoint" placeholder="localhost:9000" />
            </FormItem>
            <FormItem label="Bucket">
              <Input v-model:value="s3Config.bucket" placeholder="chiron-media" />
            </FormItem>
            <FormItem label="Access Key">
              <Input v-model:value="s3Config.access_key" />
            </FormItem>
            <FormItem label="Secret Key锛堝姞瀵嗗叆搴擄級">
              <InputPassword v-model:value="s3Config.secret_key" />
            </FormItem>
            <FormItem label="鍚敤 SSL">
              <Switch v-model:checked="s3Config.use_ssl" />
            </FormItem>
          </Form>
          <Button type="primary" :loading="saving" @click="saveS3">淇濆瓨</Button>
          <div class="config-note">淇濆瓨鍚庨噸鍚敓鏁堛€?/div>
        </Card>
      </Col>

      <!-- Python AI 寮曟搸閰嶇疆 -->
      <Col :xs="24" :sm="24">
        <Card title="Python AI 寮曟搸閰嶇疆">
          <Row :gutter="16">
            <Col :xs="24" :sm="12">
              <Form :model="pythonConfig" layout="vertical">
                <FormItem label="LLM Provider">
                  <Select v-model:value="pythonConfig.llm_provider" style="width: 100%">
                    <Select.Option value="openai">OpenAI</Select.Option>
                    <Select.Option value="anthropic">Anthropic</Select.Option>
                    <Select.Option value="deepseek">DeepSeek</Select.Option>
                  </Select>
                </FormItem>
                <FormItem label="榛樿妯″瀷">
                  <Input v-model:value="pythonConfig.llm_model" placeholder="deepseek-v4-flash" />
                </FormItem>
                <FormItem label="LLM API Key锛堝姞瀵嗗叆搴擄級">
                  <InputPassword v-model:value="pythonConfig.llm_api_key" />
                </FormItem>
                <FormItem label="LLM Base URL">
                  <Input v-model:value="pythonConfig.llm_base_url" placeholder="https://api.deepseek.com" />
                </FormItem>
              </Form>
            </Col>
            <Col :xs="24" :sm="12">
              <Form :model="pythonConfig" layout="vertical">
                <FormItem label="Embedding 妯″瀷">
                  <Input v-model:value="pythonConfig.embedding_model" placeholder="text-embedding-3-small" />
                </FormItem>
                <FormItem label="Agent 鏈€澶ц疆鏁?>
                  <InputNumber v-model:value="pythonConfig.max_turns" :min="1" :max="100" style="width: 100%" />
                </FormItem>
                <FormItem label="闃熷垪骞跺彂鏁?>
                  <InputNumber v-model:value="pythonConfig.queue_worker_concurrency" :min="1" :max="100" style="width: 100%" />
                </FormItem>
                <FormItem label="L1 缂撳瓨瀹归噺">
                  <InputNumber v-model:value="pythonConfig.cache_l1_capacity" :min="128" :max="100000" style="width: 100%" />
                </FormItem>
              </Form>
            </Col>
          </Row>
          <Button type="primary" :loading="saving" @click="savePython">淇濆瓨</Button>
          <div class="config-note">寮曟搸鍚姩鏃剁粡鍐呴儴绔偣鎷夊彇锛汚PI Key 鍔犲瘑鍏ュ簱銆?/div>
        </Card>
      </Col>
    </Row>

    <!-- Nginx 閰嶇疆 -->
    <Card title="Nginx 璋冧紭閰嶇疆" class="config-card">
      <template #extra>
        <Button type="primary" ghost @click="copyNginx">澶嶅埗閰嶇疆</Button>
      </template>
      <pre class="code-block">{{ nginxConfig }}</pre>
    </Card>

    <!-- 鍐呮牳璋冧紭 -->
    <Card title="鍐呮牳璋冧紭閰嶇疆" class="config-card">
      <template #extra>
        <Button type="primary" ghost @click="copyKernel">澶嶅埗閰嶇疆</Button>
      </template>
      <pre class="code-block">{{ kernelConfig }}</pre>
    </Card>
  </div>
</template>

<style scoped>
.settings { padding: 0; }
.config-card { margin-top: 16px; }

.config-row {
  margin-top: 16px;
}

.config-note {
  margin-top: 8px;
  color: var(--text-tertiary);
  font-size: 12px;
  line-height: 1.5;
}

.code-block {
  background: var(--bg-code);
  border: 1px solid var(--border-card);
  border-radius: var(--radius-md);
  padding: 16px;
  font-family: var(--font-mono, 'JetBrains Mono', 'Cascadia Code', 'Fira Code', monospace);
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  white-space: pre;
  color: var(--text-code, var(--text-primary));
}

/* 绉诲姩绔?*/
@media (max-width: 640px) {
  .code-block { padding: 12px; font-size: 11px; }
}

/* 绐勫睆锛氭寜閽彁楂樿Е鎺ч珮搴?*/
@media (max-width: 576px) {
  .settings :deep(.ant-btn:not(.ant-btn-sm):not(.ant-btn-link)) { min-height: 40px; }
}

@media (prefers-reduced-motion: reduce) {
  .code-block { transition: none; }
}
</style>

