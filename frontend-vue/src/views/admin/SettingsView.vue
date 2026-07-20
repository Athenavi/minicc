<template>
  <div class="settings">
    <n-grid :cols="2" :x-gap="16" :y-gap="16">
      <!-- 限流配置 -->
      <n-grid-item>
        <n-card title="限流配置">
          <n-form :model="rateLimitConfig" label-placement="left" label-width="120">
            <n-form-item label="每秒请求数">
              <n-input-number v-model:value="rateLimitConfig.rps" :min="100" :max="100000" />
            </n-form-item>
            <n-form-item label="每分钟请求数">
              <n-input-number v-model:value="rateLimitConfig.rpm" :min="1000" :max="10000000" />
            </n-form-item>
            <n-form-item label="每分钟 Token 数">
              <n-input-number v-model:value="rateLimitConfig.tpm" :min="10000" :max="1000000000" />
            </n-form-item>
            <n-form-item label="单用户并发">
              <n-input-number v-model:value="rateLimitConfig.userConcurrent" :min="1" :max="100" />
            </n-form-item>
            <n-form-item label="单租户并发">
              <n-input-number v-model:value="rateLimitConfig.tenantConcurrent" :min="10" :max="10000" />
            </n-form-item>
          </n-form>
          <n-button type="primary" :loading="saving" @click="saveRateLimit">保存</n-button>
        </n-card>
      </n-grid-item>
      
      <!-- 降级配置 -->
      <n-grid-item>
        <n-card title="降级配置">
          <n-form :model="degradationConfig" label-placement="left" label-width="120">
            <n-form-item label="启用降级">
              <n-switch v-model:value="degradationConfig.enabled" />
            </n-form-item>
            <n-form-item label="轻度过载阈值">
              <n-input-number v-model:value="degradationConfig.lightThreshold" :min="10000" :max="1000000" />
              <template #suffix>并发连接</template>
            </n-form-item>
            <n-form-item label="中度过载阈值">
              <n-input-number v-model:value="degradationConfig.mediumThreshold" :min="50000" :max="1000000" />
              <template #suffix>并发连接</template>
            </n-form-item>
            <n-form-item label="重度过载阈值">
              <n-input-number v-model:value="degradationConfig.heavyThreshold" :min="100000" :max="1000000" />
              <template #suffix>并发连接</template>
            </n-form-item>
            <n-form-item label="VIP 优先">
              <n-switch v-model:value="degradationConfig.vipPriority" />
            </n-form-item>
          </n-form>
          <n-button type="primary" :loading="saving" @click="saveDegradation">保存</n-button>
        </n-card>
      </n-grid-item>
      
      <!-- 缓存配置 -->
      <n-grid-item>
        <n-card title="缓存配置">
          <n-form :model="cacheConfig" label-placement="left" label-width="120">
            <n-form-item label="L1 容量">
              <n-input-number v-model:value="cacheConfig.l1Capacity" :min="100" :max="10000" />
            </n-form-item>
            <n-form-item label="L2 TTL">
              <n-input-number v-model:value="cacheConfig.l2Ttl" :min="60" :max="86400" />
              <template #suffix>秒</template>
            </n-form-item>
            <n-form-item label="语义缓存阈值">
              <n-slider v-model:value="cacheConfig.semanticThreshold" :min="0.5" :max="1" :step="0.01" />
            </n-form-item>
            <n-form-item label="启用预取">
              <n-switch v-model:value="cacheConfig.prefetchEnabled" />
            </n-form-item>
          </n-form>
          <n-button type="primary" :loading="saving" @click="saveCache">保存</n-button>
        </n-card>
      </n-grid-item>
      
      <!-- API Key 配置 -->
      <n-grid-item>
        <n-card title="API Key 配置">
          <n-form :model="apiKeyConfig" label-placement="left" label-width="120">
            <n-form-item label="熔断阈值">
              <n-input-number v-model:value="apiKeyConfig.circuitBreakerThreshold" :min="1" :max="100" />
              <template #suffix>次失败</template>
            </n-form-item>
            <n-form-item label="恢复超时">
              <n-input-number v-model:value="apiKeyConfig.recoveryTimeout" :min="10" :max="3600" />
              <template #suffix>秒</template>
            </n-form-item>
            <n-form-item label="权重衰减">
              <n-slider v-model:value="apiKeyConfig.weightDecay" :min="0.1" :max="1" :step="0.1" />
            </n-form-item>
            <n-form-item label="自动恢复">
              <n-switch v-model:value="apiKeyConfig.autoRecovery" />
            </n-form-item>
          </n-form>
          <n-button type="primary" :loading="saving" @click="saveApiKey">保存</n-button>
        </n-card>
      </n-grid-item>
    </n-grid>
    
    <!-- Nginx 配置 -->
    <n-card title="Nginx 调优配置" style="margin-top: 16px">
      <n-code :code="nginxConfig" language="nginx" />
      <template #header-extra>
        <n-button type="primary" quaternary @click="copyNginx">复制配置</n-button>
      </template>
    </n-card>
    
    <!-- 内核调优 -->
    <n-card title="内核调优配置" style="margin-top: 16px">
      <n-code :code="kernelConfig" language="bash" />
      <template #header-extra>
        <n-button type="primary" quaternary @click="copyKernel">复制配置</n-button>
      </template>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import { saveSettings } from '@/api/admin'

const message = useMessage()
const saving = ref(false)

const rateLimitConfig = ref({
  rps: 10000,
  rpm: 1000000,
  tpm: 100000000,
  userConcurrent: 10,
  tenantConcurrent: 1000,
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

# 文件描述符
fs.file-max = 2097152
fs.nr_open = 2097152

# TCP 连接
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_keepalive_time = 600
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 3

# 端口范围
net.ipv4.ip_local_port_range = 1024 65535

# 内存
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216

# 应用配置
# /etc/security/limits.conf
* soft nofile 2097152
* hard nofile 2097152
* soft nproc 65535
* hard nproc 65535`

async function saveRateLimit() {
  saving.value = true
  try {
    await saveSettings('rate_limit', rateLimitConfig.value)
    message.success('限流配置已保存')
  } catch (err: any) {
    message.error('保存失败: ' + (err.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

async function saveDegradation() {
  saving.value = true
  try {
    await saveSettings('degradation', degradationConfig.value)
    message.success('降级配置已保存')
  } catch (err: any) {
    message.error('保存失败: ' + (err.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

async function saveCache() {
  saving.value = true
  try {
    await saveSettings('cache', cacheConfig.value)
    message.success('缓存配置已保存')
  } catch (err: any) {
    message.error('保存失败: ' + (err.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

async function saveApiKey() {
  saving.value = true
  try {
    await saveSettings('api_key', apiKeyConfig.value)
    message.success('API Key 配置已保存')
  } catch (err: any) {
    message.error('保存失败: ' + (err.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

const copyNginx = () => {
  navigator.clipboard.writeText(nginxConfig)
  message.success('已复制到剪贴板')
}

const copyKernel = () => {
  navigator.clipboard.writeText(kernelConfig)
  message.success('已复制到剪贴板')
}
</script>

<style scoped>
.settings {
  padding: 0;
}
</style>
