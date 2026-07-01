"""邮件收发工具 — SMTP/IMAP。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class EmailSendInput(BaseModel):
    to: str = Field(description="Recipient email address")
    subject: str = Field(description="Email subject")
    body: str = Field(description="Email body text")
    cc: Optional[str] = Field(default=None, description="CC recipients (comma-separated)")
    attachments: Optional[list[str]] = Field(default=None, description="File paths to attach")


class EmailSendTool(BaseTool):
    name = "email_send"
    description = "Send an email via SMTP. Requires SMTP_* env vars."
    input_schema = EmailSendInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.WEB

    async def execute(self, input_data: EmailSendInput, context: ToolUseContext | None = None) -> ToolResult:
        import os
        smtp_host = os.environ.get("SMTP_HOST", "")
        smtp_port = int(os.environ.get("SMTP_PORT", "587"))
        smtp_user = os.environ.get("SMTP_USER", "")
        smtp_pass = os.environ.get("SMTP_PASS", "")
        from_addr = os.environ.get("SMTP_FROM", smtp_user)

        if not smtp_host or not smtp_user:
            return ToolResult(
                tool_call_id="",
                output="[email] SMTP not configured. Set SMTP_HOST, SMTP_USER, SMTP_PASS env vars.\n\n"
                       f"Queued: To={input_data.to}, Subject={input_data.subject}",
                is_error=False,
            )

        try:
            import aiosmtplib
            from email.mime.text import MIMEText
            from email.mime.multipart import MIMEMultipart
            from email.mime.base import MIMEBase
            from email import encoders

            msg = MIMEMultipart()
            msg["From"] = from_addr
            msg["To"] = input_data.to
            msg["Subject"] = input_data.subject
            if input_data.cc:
                msg["Cc"] = input_data.cc
            msg.attach(MIMEText(input_data.body, "plain"))

            if input_data.attachments:
                for fpath in input_data.attachments:
                    import os as _os
                    if _os.path.exists(fpath):
                        with open(fpath, "rb") as f:
                            part = MIMEBase("application", "octet-stream")
                            part.set_payload(f.read())
                            encoders.encode_base64(part)
                            part.add_header("Content-Disposition", f'attachment; filename="{_os.path.basename(fpath)}"')
                            msg.attach(part)

            await aiosmtplib.send(msg, hostname=smtp_host, port=smtp_port,
                                  username=smtp_user, password=smtp_pass, use_tls=True)
            return ToolResult(tool_call_id="", output=f"[email] Sent to {input_data.to}: {input_data.subject}")
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"[email] Send failed: {exc}", is_error=True)


def register_email_tools(registry) -> None:
    registry.register(EmailSendTool())
