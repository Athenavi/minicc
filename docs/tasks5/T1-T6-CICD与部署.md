# T1-T6: CI/CD 与自动部署

> **总预估**：4.5 周 | **前置**：Phase S

## T1: 自动 CI 配置（1 周）

`backend/app/devops/ci_generator.py`：

```python class CIGenerator:
    """自动生成 CI/CD 配置文件。"""

    async def generate(self, repo_analysis: RepoAnalysis) -> CIYAML:
        """分析项目：
        - 语言/框架检测（Python/Node/Go/Rust）
        - 测试框架检测（pytest/jest/go test）
        - 构建工具检测（poetry/npm/cargo）
        → 生成 .github/workflows/ci.yml
        """
```

支持：GitHub Actions、GitLab CI、Jenkinsfile

## T2: 构建管理（1 周）

`backend/app/devops/build_manager.py`：
- Docker 镜像构建（多阶段构建优化）
- 版本号管理（SemVer 自动递增）
- 制品存储（Docker Registry / S3）
- 构建缓存优化

## T3: 自动部署（1 周）

`backend/app/devops/deployer.py`：

```python
class Deployer:
    async def deploy(self, env: str, artifact: str) -> DeployResult:
        """支持：
        - Docker Compose（V0.4 Q1）
        - Kubernetes（自动生成 manifests）
        - VPS（SSH + systemd）
        - Serverless（AWS Lambda / Cloudflare Workers）
        """
```

## T4: 环境管理（0.5 周）

- dev/staging/prod 环境自动创建
- 环境变量管理（.env 模板 + 密钥注入）
- 数据库自动创建

## T5: 数据库 Migration（0.5 周）

`backend/app/devops/migration_manager.py`：
- 分析模型变更 → 生成 Alembic 迁移脚本
- 自动执行迁移（dev → staging → prod）
- 回滚支持

## T6: 域名/DNS/SSL（0.5 周）

- 自动获取 SSL 证书（Let's Encrypt）
- DNS 配置（Cloudflare API）
- Nginx/Caddy 反向代理配置

### 验收标准
- [ ] `git push` 自动触发 CI
- [ ] Docker 镜像自动构建
- [ ] 一键部署到 VPS/K8s
- [ ] 多环境隔离
- [ ] 数据库迁移自动执行
- [ ] SSL 证书自动续期
- [ ] 120 测试通过
