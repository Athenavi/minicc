"""SSRF 防护 — 解析级 IP 黑名单（S4 安全修复）

web_fetch/web_search/skill_install 等出站 HTTP 的目标 host 解析后，
拒绝回环/私有/链路本地/保留 IP 段（防访问内网与云元数据）。

局限（文档标注）：解析级防护存在 DNS rebinding 理论风险
（解析后域名重绑），生产环境应配合出站代理白名单。
"""
from __future__ import annotations

import ipaddress
import logging
import socket
from urllib.parse import urlparse

logger = logging.getLogger(__name__)

# 被禁止的 IP 段（IPv4）
_BLOCKED_IPV4 = [
    ipaddress.ip_network("0.0.0.0/8"),
    ipaddress.ip_network("10.0.0.0/8"),
    ipaddress.ip_network("127.0.0.0/8"),
    ipaddress.ip_network("169.254.0.0/16"),   # 云元数据/链路本地
    ipaddress.ip_network("172.16.0.0/12"),
    ipaddress.ip_network("192.168.0.0/16"),
    ipaddress.ip_network("224.0.0.0/4"),
    ipaddress.ip_network("240.0.0.0/4"),
]
# 被禁止的 IPv6 段
_BLOCKED_IPV6 = [
    ipaddress.ip_network("::1/128"),
    ipaddress.ip_network("::/128"),
    ipaddress.ip_network("fc00::/7"),        # ULA
    ipaddress.ip_network("fe80::/10"),       # 链路本地
]


def _is_blocked(ip: ipaddress.IPv4Address | ipaddress.IPv6Address) -> bool:
    nets = _BLOCKED_IPV4 if isinstance(ip, ipaddress.IPv4Address) else _BLOCKED_IPV6
    return any(ip in net for net in nets)


def assert_safe_url(url: str) -> None:
    """解析 url 的 host，若指向内网/保留地址则抛 ValueError。"""
    parsed = urlparse(url)
    host = parsed.hostname
    if not host:
        raise ValueError(f"invalid url: {url}")

    # 已是 IP 字面量 → 直接检查
    try:
        ip = ipaddress.ip_address(host)
        if _is_blocked(ip):
            raise ValueError(f"blocked address (internal/private): {host}")
        return
    except ValueError as e:
        if isinstance(e, ValueError) and str(e).startswith("blocked"):
            raise
        # 非 IP 字面量 → 继续 DNS 解析

    # DNS 解析，检查所有结果
    try:
        infos = socket.getaddrinfo(host, None)
    except socket.gaierror as e:
        raise ValueError(f"cannot resolve host: {host}") from e

    for info in infos:
        ip = ipaddress.ip_address(info[4][0])
        if _is_blocked(ip):
            raise ValueError(f"blocked address (internal/private): {host} -> {ip}")
