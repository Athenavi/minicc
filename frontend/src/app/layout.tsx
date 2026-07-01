import Link from "next/link";

const navItems = [
  { href: "/", label: "💬 Chat", desc: "AI Chat" },
  { href: "/editor", label: "✏️ Editor", desc: "AI Code Editor" },
  { href: "/workflow", label: "🔀 Workflow", desc: "Visual Graph" },
  { href: "/rpa", label: "🤖 RPA", desc: "Automation" },
  { href: "/enterprise", label: "🏢 Enterprise", desc: "CRM/ERP" },
  { href: "/devops", label: "🚀 DevOps", desc: "CI/CD" },
  { href: "/system", label: "🔬 System", desc: "Self-Monitor" },
];

import ClientLayout from "./client-layout";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{
          __html: `
            try {
              let theme = localStorage.getItem('theme');
              if (!theme) theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
              if (theme === 'dark') document.documentElement.classList.add('dark');
            } catch(e) {}
          `
        }} />
      </head>
      <body className="bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100">
        <ClientLayout navItems={navItems}>{children}</ClientLayout>
      </body>
    </html>
  );
}
