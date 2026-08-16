<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Card, Button, Spin, Empty, Tag, message } from 'ant-design-vue'
import { ThunderboltOutlined, DownloadOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import { api } from '../api'

interface Plugin {
  name: string
  description: string
  version: string
  status: string
}

const loading = ref(true)
const plugins = ref<Plugin[]>([])

onMounted(async () => {
  await loadPlugins()
})

async function loadPlugins() {
  try {
    loading.value = true
    const response = await api.get('/v1/plugins')
    plugins.value = response.data?.data || []
  } catch (error) {
    message.error('获取插件列表失败，请检查网络连接')
  } finally {
    loading.value = false
  }
}

async function handleInstall(name: string) {
  try {
    await api.post(`/v1/plugins/${name}/install`)
    message.success(`${name} 已安装`)
    await loadPlugins()
  } catch (error: any) {
    message.error(error.message || '安装失败')
  }
}

async function handleUninstall(name: string) {
  try {
    await api.delete(`/v1/plugins/${name}`)
    message.success(`${name} 已卸载`)
    await loadPlugins()
  } catch (error: any) {
    message.error(error.message || '卸载失败')
  }
}
</script>

<template>
  <div class="plugins-container">
    <div class="plugins-header">
      <ThunderboltOutlined style="font-size: 24px; color: #8b5cf6" />
      <h1>插件管理</h1>
    </div>

    <Spin :spinning="loading">
      <Empty v-if="plugins.length === 0 && !loading" description="暂无插件">
        <template #image>
          <ThunderboltOutlined style="font-size: 48px; color: #8b5cf6" />
        </template>
      </Empty>

      <div v-else class="plugins-grid">
        <Card v-for="plugin in plugins" :key="plugin.name" class="plugin-card">
          <div class="plugin-header">
            <span class="plugin-name">{{ plugin.name }}</span>
            <Tag :color="plugin.status === 'active' ? 'success' : 'default'">
              {{ plugin.status }}
            </Tag>
          </div>
          <p class="plugin-description">{{ plugin.description }}</p>
          <div class="plugin-footer">
            <span class="plugin-version">v{{ plugin.version }}</span>
            <div class="plugin-actions">
              <Button
                v-if="plugin.status !== 'active'"
                type="primary"
                size="small"
                @click="handleInstall(plugin.name)"
              >
                <template #icon><DownloadOutlined /></template>
                安装
              </Button>
              <Button
                v-else
                danger
                size="small"
                @click="handleUninstall(plugin.name)"
              >
                <template #icon><DeleteOutlined /></template>
                卸载
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </Spin>
  </div>
</template>

<style scoped>
.plugins-container { padding: 24px; }
.plugins-header { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.plugins-header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.plugins-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; }
.plugin-card { transition: box-shadow 0.2s; }
.plugin-card:hover { box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1); }
.plugin-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.plugin-name { font-weight: 600; font-size: 16px; }
.plugin-description { color: #6b7280; font-size: 14px; margin: 0 0 12px; line-height: 1.5; }
.plugin-footer { display: flex; align-items: center; justify-content: space-between; }
.plugin-version { color: #9ca3af; font-size: 12px; }
.plugin-actions { display: flex; gap: 8px; }
</style>
