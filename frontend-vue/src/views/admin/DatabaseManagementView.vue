<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Card, Table, Button, Space, Tag, Input, Modal, Form, InputNumber, Alert, Progress, Descriptions } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, DatabaseOutlined, BackupOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { api } from '../../api'

// State
const loading = ref(true)
const dbConfigs = ref<any[]>([])
const selectedDB = ref<any>(null)
const statusData = ref<any>({})
const backups = ref<any[]>([])
const queryResult = ref<any[]>([])
const modalVisible = ref(false)
const queryModalVisible = ref(false)
const queryText = ref('SELECT * FROM information_schema.tables LIMIT 10;')
const currentForm = ref({
  name: '',
  host: 'localhost',
  port: 5432,
  dbname: 'minicc',
  username: 'postgres',
  password_hash: '',
  max_open_connections: 25,
  max_idle_connections: 5,
  conn_max_lifetime: '30m'
})

// Load DB configs
async function loadDBConfigs() {
  try {
    loading.value = true
    const response = await api.get('/admin/database/configs')
    dbConfigs.value = response.data || []
  } catch (error: any) {
    console.error('Failed to load DB configs:', error)
  } finally {
    loading.value = false
  }
}

// Get status
async function getStatus(id: string) {
  try {
    const response = await api.get(`/admin/database/${id}/status`)
    statusData.value = response.data
    selectedDB.value = dbConfigs.value.find(d => d.id === id)
  } catch (error: any) {
    alert(error.response?.data?.error || '获取状态失败')
  }
}

// Create backup
async function createBackup(description: string) {
  try {
    await api.post('/admin/database/backups', { description })
    alert('备份已开始,请稍后查看状态')
  } catch (error: any) {
    alert(error.response?.data?.error || '创建备份失败')
  }
}

// List backups
async function listBackups() {
  try {
    const response = await api.get('/admin/database/backups')
    backups.value = response.data || []
  } catch (error: any) {
    console.error('Failed to list backups:', error)
  }
}

// Restore backup
async function restoreBackup(backupId: string) {
  if (!confirm('警告: 确定要从该备份恢复吗?当前数据将被覆盖!')) return
  
  try {
    await api.post(`/admin/database/backups/${backupId}/restore`)
    alert('恢复已启动,请稍后查看状态')
  } catch (error: any) {
    alert(error.response?.data?.error || '恢复失败')
  }
}

// Execute query
async function executeQuery() {
  if (!queryText.value.trim()) {
    alert('请输入 SQL 查询')
    return
  }
  
  try {
    const response = await api.post('/admin/database/query', {
      query: queryText.value
    })
    queryResult.value = response.data || []
  } catch (error: any) {
    alert(error.response?.data?.error || '查询执行失败')
  }
}

// Optimize database
async function optimize(action: string) {
  const actionNames: Record<string, string> = {
    vacuum: 'VACUUM ANALYZE',
    analyze: 'ANALYZE',
    reindex: 'REINDEX'
  }
  
  if (!confirm(`确定要执行 ${actionNames[action]} 吗?`)) return
  
  try {
    await api.post(`/admin/database/optimize/${action}`)
    alert('优化操作已完成')
  } catch (error: any) {
    alert(error.response?.data?.error || '优化失败')
  }
}

// Helper functions
function getStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'inactive': return 'red'
    default: return 'default'
  }
}

function getCacheHitRateColor(rate: number): string {
  if (rate >= 99) return 'green'
  if (rate >= 95) return 'orange'
  return 'red'
}

// Initialize
onMounted(() => {
  loadDBConfigs()
  listBackups()
})
</script>

<template>
  <div class="database-management">
    <!-- Header -->
    <div class="page-header">
      <h1>🗄️ 数据库管理</h1>
      <Space>
        <Button @click="listBackups">
          <ReloadOutlined /> 刷新备份
        </Button>
        <Button type="primary" @click="modalVisible = true">
          <PlusOutlined /> 添加数据库实例
        </Button>
      </Space>
    </div>

    <!-- DB Instance List -->
    <Card style="margin-bottom: 16px">
      <Table
        :columns="[
          { title: '名称', dataIndex: 'name', key: 'name', width: 150 },
          { title: '地址', dataIndex: 'host', key: 'host', width: 200,
            customRender: ({ record }: any) => `${record.host}:${record.port}/${record.dbname}` },
          { title: '最大连接', dataIndex: 'max_open_connections', key: 'max_open_connections', width: 100 },
          { title: '状态', key: 'status', width: 100,
            customRender: ({ record }: any) => (
              <Tag color={getStatusColor(record.status)}>{record.status}</Tag>
            )
          },
          { title: '数据库大小', dataIndex: 'database_size_mb', key: 'database_size_mb', width: 120,
            customRender: ({ record }: any) => `${record.database_size_mb?.toFixed(2)} MB` },
          { title: '表数量', dataIndex: 'total_tables', key: 'total_tables', width: 80 },
          { title: '操作', key: 'actions', width: 300, fixed: 'right',
            customRender: ({ record }: any) => (
              <Space>
                <Button size="small" icon={<MonitorOutlined />} onClick={() => getStatus(record.id)}>监控</Button>
                <Button size="small" icon={<SearchOutlined />} onClick={() => { selectedDB.value = record; queryModalVisible.value = true; }}>查询</Button>
                <Button size="small" icon={<BackupOutlined />} onClick={() => createBackup(`手动备份 - ${record.name}`)}>备份</Button>
              </Space>
            )
          }
        ]"
        :data-source="dbConfigs"
        :loading="loading"
        row-key="id"
      />
    </Card>

    <!-- Backups List -->
    <Card style="margin-bottom: 16px">
      <h3 style="margin-top: 0">💾 备份记录</h3>
      <Table
        :columns="[
          { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
          { title: '类型', dataIndex: 'backup_type', key: 'backup_type', width: 100,
            customRender: ({ record }: any) => (
              <Tag>{record.backup_type === 'manual' ? '手动' : '自动'}</Tag>
            )
          },
          { title: '状态', key: 'status', width: 100,
            customRender: ({ record }: any) => (
              <Tag color={record.status === 'completed' ? 'green' : 'orange'}>{record.status}</Tag>
            )
          },
          { title: '大小', dataIndex: 'size_mb', key: 'size_mb', width: 100,
            customRender: ({ record }: any) => `${record.size_mb?.toFixed(2)} MB` },
          { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 180 },
          { title: '操作', key: 'actions', width: 120,
            customRender: ({ record }: any) => (
              <Button size="small" danger disabled={record.status !== 'completed'} onClick={() => restoreBackup(record.id)}>
                恢复
              </Button>
            )
          }
        ]"
        :data-source="backups"
        row-key="id"
        :pagination="{ pageSize: 10 }"
      />
    </Card>

    <!-- Status Detail Modal -->
    <Modal
      v-model:open="selectedDB !== null"
      title={`📊 ${selectedDB?.name} - 实时监控`}
      footer={null}
      width={900}
      onClose={() => selectedDB.value = null}
    >
      <Tabs>
        <!-- Overview Tab -->
        <TabPane tab="概览" key="overview">
          <Space direction="vertical" style="width: 100%" size="large">
            <Card>
              <Descriptions column="2" bordered>
                <Descriptions.Item label="版本">{statusData.version || 'N/A'}</Descriptions.Item>
                <Descriptions.Item label="运行状态">
                  <Tag color={getStatusColor(statusData.status)}>{statusData.status || 'unknown'}</Tag>
                </Descriptions.Item>
                <Descriptions.Item label="活跃连接数">{statusData.active_connections || 0}</Descriptions.Item>
                <Descriptions.Item label="最大连接数">{statusData.max_connections || 100}</Descriptions.Item>
                <Descriptions.Item label="数据库大小">{statusData.disk_usage?.total_size_mb || 0} MB</Descriptions.Item>
                <Descriptions.Item label="总表数">{statusData.disk_usage?.table_count || 0}</Descriptions.Item>
              </Descriptions>
            </Card>

            <Card title="磁盘使用情况">
              <Space direction="vertical" style="width: 100%">
                <div>
                  <p>数据大小: {{ statusData.disk_usage?.data_size_mb || 0 }} MB</p>
                  <Progress percent={(statusData.disk_usage?.data_size_mb / statusData.disk_usage?.total_size_mb * 100).toFixed(1)} />
                </div>
                <div>
                  <p>索引大小: {{ statusData.disk_usage?.index_size_mb || 0 }} MB</p>
                  <Progress percent={(statusData.disk_usage?.index_size_mb / statusData.disk_usage?.total_size_mb * 100).toFixed(1)} />
                </div>
                <div>
                  <p>TOAST 大小: {{ statusData.disk_usage?.toast_size_mb || 0 }} MB</p>
                  <Progress percent={(statusData.disk_usage?.toast_size_mb / statusData.disk_usage?.total_size_mb * 100).toFixed(1)} />
                </div>
              </Space>
            </Card>
          </Space>
        </TabPane>

        <!-- Performance Tab -->
        <TabPane tab="性能指标" key="performance">
          <Space direction="vertical" style="width: 100%" size="large">
            <Card title="查询统计">
              <Descriptions column="2" bordered>
                <Descriptions.Item label="平均查询时间">{statusData.query_stats?.avg_query_time_ms || 0} ms</Descriptions.Item>
                <Descriptions.Item label="慢查询数">{statusData.query_stats?.slow_queries || 0}</Descriptions.Item>
                <Descriptions.Item label="总查询数">{statusData.query_stats?.total_queries || 0}</Descriptions.Item>
                <Descriptions.Item label="查询失败数">{statusData.query_stats?.failures || 0}</Descriptions.Item>
              </Descriptions>
            </Card>

            <Card title="缓存命中率">
              <Space direction="vertical" style="width: 100%">
                <Progress
                  type="circle"
                  :percent="(statusData.performance?.cache_hit_rate || 0).toFixed(2)"
                  format={(percent: number) => `${percent}%`}
                />
                <p style="text-align: center; color: getCacheHitRateColor(statusData.performance?.cache_hit_rate) ">
                  缓存命中率
                </p>
              </Space>
            </Card>

            <Card title="操作速率">
              <Descriptions column="3" bordered>
                <Descriptions.Item label="元组读取/秒">{statusData.performance?.tuple_read_per_sec || 0}</Descriptions.Item>
                <Descriptions.Item label="元组插入/秒">{statusData.performance?.tuple_insert_per_sec || 0}</Descriptions.Item>
                <Descriptions.Item label="缓冲区命中率">{statusData.performance?.buffer_hit_rate || 0}%</Descriptions.Item>
              </Descriptions>
            </Card>
          </Space>
        </TabPane>

        <!-- Optimization Tab -->
        <TabPane tab="优化操作" key="optimization">
          <Alert message="以下操作将直接影响数据库性能,请谨慎使用" type="warning" showIcon style="margin-bottom: 16px" />
          <Space direction="vertical" style="width: 100%">
            <Button block @click="optimize('vacuum')">
              VACUUM ANALYZE - 回收存储空间并更新统计信息
            </Button>
            <Button block @click="optimize('analyze')">
              ANALYZE - 更新查询优化器统计信息
            </Button>
            <Button block @click="optimize('reindex')">
              REINDEX - 重建所有索引
            </Button>
          </Space>
        </TabPane>
      </Tabs>
    </Modal>

    <!-- Query Modal -->
    <Modal
      v-model:open="queryModalVisible"
      title="🔍 SQL 查询 (只读)"
      footer={null}
      width={1000}
      onCancel={() => { queryModalVisible.value = false; queryResult.value = [] }}
    >
      <Space direction="vertical" style="width: 100%">
        <TextArea
          v-model:value="queryText"
          :rows="4"
          placeholder="输入 SELECT 查询语句..."
          style="font-family: monospace"
        />
        <Button type="primary" @click="executeQuery">
          <SearchOutlined /> 执行查询
        </Button>
        
        <Table
          :columns="[
            { title: '列名', dataIndex: 'key', key: 'key', width: 200 },
            { title: '值', dataIndex: 'value', key: 'value' }
          ]"
          :data-source="queryResult.flatMap((row: any) => Object.entries(row).map(([key, value]) => ({ key, value })))"
          row-key="key"
          :scroll="{ x: 800 }"
          :pagination="{ pageSize: 20 }"
        />
      </Space>
    </Modal>

    <!-- Add/Edit Modal -->
    <Modal
      v-model:open="modalVisible"
      title="添加数据库实例"
      ok-text="创建"
      cancel-text="取消"
      @ok="loadDBConfigs"
    >
      <Form layout="vertical">
        <Form.Item label="实例名称" required>
          <Input v-model:value="currentForm.name" placeholder="例如: Production PostgreSQL" />
        </Form.Item>
        <Form.Item label="主机地址" required>
          <Input v-model:value="currentForm.host" placeholder="localhost" />
        </Form.Item>
        <Form.Item label="端口" required>
          <InputNumber v-model:value="currentForm.port" :min="1" :max="65535" style="width: 100%" />
        </Form.Item>
        <Form.Item label="数据库名" required>
          <Input v-model:value="currentForm.dbname" placeholder="minicc" />
        </Form.Item>
        <Form.Item label="用户名" required>
          <Input v-model:value="currentForm.username" placeholder="postgres" />
        </Form.Item>
        <Form.Item label="密码哈希">
          <Input v-model:value="currentForm.password_hash" placeholder="SHA-256 哈希值" />
        </Form.Item>
        <Form.Item label="最大打开连接">
          <InputNumber v-model:value="currentForm.max_open_connections" :min="1" :max="100" style="width: 100%" />
        </Form.Item>
        <Form.Item label="最大空闲连接">
          <InputNumber v-model:value="currentForm.max_idle_connections" :min="0" :max="50" style="width: 100%" />
        </Form.Item>
        <Form.Item label="连接生命周期">
          <Input v-model:value="currentForm.conn_max_lifetime" placeholder="30m" />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.database-management {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}
</style>
