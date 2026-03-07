import type { Metadata } from "next";
import { TooltipProvider } from "@/components/ui/tooltip";
import { themeInitScript } from "@/lib/theme";
import "./globals.css";

export const metadata: Metadata = {
  title: "Bored",
  description: "An LLM-powered issue tracker",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="antialiased">
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
        <TooltipProvider>{children}</TooltipProvider>
      </body>
    </html>
  );
}
