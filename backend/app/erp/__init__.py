"""ERP 工具 — 企业资源规划（X1-X6）。"""

from __future__ import annotations

import sqlite3
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

_DB = Path("minicc_erp.db")


class _Empty(BaseModel):
    pass


def _db() -> sqlite3.Connection:
    db = sqlite3.connect(str(_DB))
    db.execute("CREATE TABLE IF NOT EXISTS suppliers (id TEXT PRIMARY KEY, name TEXT, contact TEXT, rating REAL, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS purchase_orders (id TEXT PRIMARY KEY, supplier_id TEXT, items TEXT, total REAL, status TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS inventory (id TEXT PRIMARY KEY, sku TEXT, name TEXT, quantity INTEGER, warehouse TEXT, min_level INTEGER, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS sales_orders (id TEXT PRIMARY KEY, customer TEXT, items TEXT, total REAL, status TEXT, created_at TEXT)")
    db.execute("CREATE TABLE IF NOT EXISTS invoices (id TEXT PRIMARY KEY, customer TEXT, amount REAL, status TEXT, due_date TEXT, created_at TEXT)")
    db.commit()
    return db


class SupplierCreateInput(BaseModel):
    name: str = Field(description="Supplier name")
    contact: str = Field(default="", description="Contact info")


class InventoryInput(BaseModel):
    sku: str = Field(description="SKU code")
    name: str = Field(description="Product name")
    quantity: int = Field(default=0, description="Quantity")
    warehouse: str = Field(default="main", description="Warehouse")
    min_level: int = Field(default=10, description="Minimum stock level")


class OrderInput(BaseModel):
    customer: str = Field(description="Customer name")
    items: str = Field(description="JSON array of items")
    total: float = Field(description="Order total")


class SupplierCreateTool(BaseTool):
    name = "erp_supplier_create"
    description = "Register a new supplier."
    input_schema = SupplierCreateInput

    async def execute(self, input_data: SupplierCreateInput, context=None) -> ToolResult:
        import uuid, datetime
        sid = uuid.uuid4().hex[:12]
        db = _db()
        db.execute("INSERT INTO suppliers VALUES (?,?,?,?,?)", (sid, input_data.name, input_data.contact, 100.0, datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[erp] Supplier created: {input_data.name}")


class InventoryAddTool(BaseTool):
    name = "erp_inventory_add"
    description = "Add product to inventory."
    input_schema = InventoryInput

    async def execute(self, input_data: InventoryInput, context=None) -> ToolResult:
        import uuid, datetime
        iid = uuid.uuid4().hex[:12]
        db = _db()
        db.execute("INSERT INTO inventory VALUES (?,?,?,?,?,?,?)",
                   (iid, input_data.sku, input_data.name, input_data.quantity, input_data.warehouse, input_data.min_level, datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[erp] Inventory added: {input_data.sku} ({input_data.quantity} in {input_data.warehouse})")


class InventoryCheckTool(BaseTool):
    name = "erp_inventory_check"
    description = "Check inventory levels and get low-stock alerts."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        db = _db()
        rows = db.execute("SELECT sku, name, quantity, warehouse, min_level FROM inventory").fetchall()
        low = [r for r in rows if r[2] < r[4]]
        lines = [f"[erp] Total products: {len(rows)}"]
        if low:
            lines.append(f"\n⚠ Low Stock Alerts ({len(low)}):")
            for r in low:
                lines.append(f"  • {r[0]} {r[1]}: {r[2]} (min: {r[4]}) in {r[3]}")
        else:
            lines.append("\n✅ All inventory levels OK")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class OrderCreateTool(BaseTool):
    name = "erp_order_create"
    description = "Create a sales order."
    input_schema = OrderInput

    async def execute(self, input_data: OrderInput, context=None) -> ToolResult:
        import uuid, datetime
        oid = uuid.uuid4().hex[:12]
        db = _db()
        db.execute("INSERT INTO sales_orders VALUES (?,?,?,?,?,?)",
                   (oid, input_data.customer, input_data.items, input_data.total, "pending", datetime.datetime.now().isoformat()))
        db.commit()
        return ToolResult(tool_call_id="", output=f"[erp] Order created: {oid} for {input_data.customer} (${input_data.total:.2f})")


class InvoiceCreateTool(BaseTool):
    name = "erp_invoice_create"
    description = "Create an invoice for a customer."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[erp] Use: erp_order_create first, then this to generate invoice.")


def register_erp_tools(registry) -> None:
    registry.register(SupplierCreateTool())
    registry.register(InventoryAddTool())
    registry.register(InventoryCheckTool())
    registry.register(OrderCreateTool())
    registry.register(InvoiceCreateTool())
