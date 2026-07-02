import "./globals.css";
import { TooltipProvider } from "@/components/ui/tooltip";

const navItems = [
  { href: "/workspace", label: "⚡ Workspace", desc: "AI Agent Hub" },
  { href: "/workflow", label: "🔀 Workflow", desc: "Visual Graph" },
  { href: "/enterprise", label: "🏢 Enterprise", desc: "Business" },
  { href: "/system", label: "🔬 System", desc: "Monitor" },
];

import ClientLayout from "./client-layout";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `
            try {
              let theme = localStorage.getItem('theme');
              if (!theme) theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
              if (theme === 'dark') document.documentElement.classList.add('dark');
            } catch(e) {}
          `}}
        />
      </head>
      <body className="bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100">
        <TooltipProvider>
          <ClientLayout navItems={navItems}>{children}</ClientLayout>
        </TooltipProvider>
      </body>
    </html>
  );
}
