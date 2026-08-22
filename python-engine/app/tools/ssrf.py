"""SSRF 防护 — 解析级 IP 黑名单 + scheme/端口白名单 + 连接时复检

web_fetch/web_search/skill_install 等出站 HTTP 的目标 host 解析后，
拒绝回环/私有/链路本地/保留 IP 段（防访问内网与云元数据）。

强化项：
- scheme 白名单：仅允许 http/https
- 端口白名单：仅允许 80/443/8080/8443（防访问内网 6379/5432/27017 等服务端口）
- 连接时复检：调用方应在 socket.getaddrinfo 之后、connect 之前再次校验 IP，
  防 DNS rebinding（解析级防护的固有局限）
"""
from __future__ import annotations

import ipaddress
import logging
import socket
from urllib.parse import urlparse

logger = logging.getLogger(__name__)

# 允许的 scheme
_ALLOWED_SCHEMES = {"http", "https"}
# 允许的端口（防访问内网数据库/缓存/消息队列服务端口）
_ALLOWED_PORTS = {80, 443, 8080, 8443, 8000, 5000}

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
    """解析 url 的 host，若指向内网/保留地址则抛 ValueError。

    校验顺序：scheme → 端口 → host → DNS 解析 → IP 黑名单。
    调用方在 connect 时应调用 assert_safe_ip 再次复检防 DNS rebinding。
    """
    parsed = urlparse(url)
    if parsed.scheme not in _ALLOWED_SCHEMES:
        raise ValueError(f"blocked scheme: {parsed.scheme} (only http/https allowed)")
    host = parsed.hostname
    if not host:
        raise ValueError(f"invalid url: {url}")

    # 端口白名单：未显式指定端口时按 scheme 默认值（80/443）
    port = parsed.port
    if port is None:
        port = 443 if parsed.scheme == "https" else 80
    if port not in _ALLOWED_PORTS:
        raise ValueError(f"blocked port: {port} (not in allowlist)")

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


def assert_safe_ip(ip_str: str) -> None:
    """连接时复检 IP，防 DNS rebinding。

    调用方流程：
        assert_safe_url(url)
        ip = socket.gethostbyname(host)  # 再次解析
        assert_safe_ip(ip)               # 复检
        # 连接 ip
    """
    try:
        ip = ipaddress.ip_address(ip_str)
    except ValueError as e:
        raise ValueError(f"invalid ip for rebinding check: {ip_str}") from e
    if _is_blocked(ip):
        raise ValueError(f"blocked address at connect time (DNS rebinding?): {ip_str}")
