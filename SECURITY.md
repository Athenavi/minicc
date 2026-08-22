# Security Policy

## 报告漏洞(Reporting a Vulnerability)

我们非常重视安全性。如果你发现 MiniCC 存在安全漏洞,请**不要**公开提交 Issue 或 PR——请通过以下任一私有渠道报告:

1. **GitHub Private Vulnerability Reporting(推荐)**:访问 <https://github.com/athenavi/minicc/security/advisories> 提交私有安全公告。
2. **邮件**:发送至 `security@athenavi.com`(占位邮箱,正式开源后请替换为维护者真实邮箱)。

请在报告中包含:

- 漏洞类型与影响范围(组件、接口、版本);
- 复现步骤或最小 PoC(不包含敏感数据);
- 你观察到的实际影响(数据泄露 / 权限绕过 / DoS 等);
- 建议的修复方向(如有)。

**响应承诺**:

- 收到报告后 48 小时内确认收悉;
- 确认有效后尽快修复,严重漏洞通常 90 天内发布修复版本;
- 在修复发布前,我们会与报告者协调披露时间,并(如你同意)在致谢中署名。

## 安全特性

### 多租户与访问控制

- **租户 / 用户两级隔离**:所有业务查询强制携带 `tenant_id` / `user_id` 条件(行级);Redis key、Milvus 向量检索、媒体资源、插件配置均按租户/用户命名空间隔离。
- **认证链路**:JWT(HS256,cookie + bearer)+ API Key + OAuth/OIDC/SMS 多因子入口;`COOKIE_SECURE=true`(生产)为 cookie 追加 `Secure` 标志。
- **网关 → 引擎身份透传 fail-close**:网关转发内部请求时注入 `X-Internal-Token`;Python 引擎仅在令牌匹配时才接受 `tenant_id`/`user_id` query 透传,否则**拒绝**(防止直连引擎绕过网关鉴权)。
- **RBAC**:管理接口按权限位(`RequirePermission`)校验,企业版支持角色 / 群组 / 操作审计。

### 注入与命令执行防护

- **Prompt 注入检测**:网关 `InputSanitizer` 对常见注入模式(忽略历史指令、越权提示、系统提示覆盖等)正则检测并拒绝;合法输入以 `<user_input>` 标签包裹隔离。
- **SSRF 防护**:引擎工具层对目标地址做端口白名单校验,拦截内网/元数据地址与未允许端口。
- **插件命令白名单**:`PLUGIN_COMMAND_ALLOWLIST`(逗号分隔的 basename)控制 MCP 插件可拉起的可执行文件;**留空即禁止所有自定义插件命令**(安全默认)。
- **输出路径脱敏**:工具输出与文件读写路径经过校验,防止路径穿越。

### 媒体与数据保护

- **媒体签名 URL**:资源不公开直出;`POST /v1/media/{id}/sign` 校验归属(资产必须属于当前租户与用户)后签发 `HMAC-SHA256(JWT_SECRET, assetID|exp)` 签名,15 分钟有效期,`GET /media/s/{id}?exp=&sig=` 验签后流式返回。
- **存储型 XSS 防护**:媒体上传支持分片与内容校验,`/media/` 静态服务经净化处理。

### 可用性防护

- **分布式限流**:每用户 RPM + 每租户 RPS + 全局限额三层;`TRUSTED_PROXY_CIDRS` 防止伪造 `X-Forwarded-For` 绕过限流/验证码。
- **Redis 必需化 fail-fast**:Redis 作为队列、语义缓存与分布式限流的核心依赖,不可用时快速失败而非静默降级(避免"看似可用实则失效"的降级攻击面)。
- **指标鉴权**:`/metrics` 需要 `METRICS_TOKEN`(Bearer)或 JWT 管理员权限。
- **Fail-fast 依赖**:关键配置缺失(如 `JWT_SECRET`)时网关拒绝启动,不提供弱默认值。

## 已知边界(Known Limitations)

- **技能沙箱为进程级隔离**:Python 引擎对技能/工具的执行采用进程级沙箱与文件系统限制,而非 VM / 容器级隔离。**不要**在不可信环境下对不可信代码授予自定义命令执行权限;`PLUGIN_COMMAND_ALLOWLIST` 请按最小权限原则配置。
- **SSRF 防护基于白名单**:端口白名单可能误伤合法内网工具,也可能被高级绕过技术规避;内网部署请结合网络层防火墙。
- **限流为应用层机制**:不是 WAF/网关级防护,建议在生产入口叠加 CDN / WAF。
- **AI 输出本身不可信**:LLM 生成的文本可能包含注入或误导性内容,渲染端应继续使用 DOMPurify 等净化(前端已内置),并保留人类确认环节。

## 生产部署安全清单

1. `JWT_SECRET` 使用 `openssl rand -base64 48` 生成且唯一;
2. 多租户生产环境务必配置 `INTERNAL_TOKEN`;
3. `COOKIE_SECURE=true`,`DISABLE_REGISTRATION=true`(如需封闭注册);
4. 设置 `TRUSTED_PROXY_CIDRS` 与 `METRICS_TOKEN`;
5. 按最小权限配置 `PLUGIN_COMMAND_ALLOWLIST`;
6. PostgreSQL / Redis / MinIO 使用强口令,Redis 开启 `requirepass`;
7. 前端 `CORS_ORIGINS` 与 `CSP_CONNECT_SRC` 收敛到实际域名,禁止 `*`;
8. 定期更新依赖并关注 GitHub Security Advisories。
