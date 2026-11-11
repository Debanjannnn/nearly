import type { Metadata, Viewport } from "next";
import { Geist_Mono, Inter } from "next/font/google";
import "./globals.css";
import { MotionRoot } from "@/components/motion";
import { SessionProvider } from "@/lib/session";

const inter = Inter({ variable: "--font-inter", subsets: ["latin"] });
const mono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"] });

export const metadata: Metadata = {
  title: { default: "nearly: non-custodial yield baskets on NEAR", template: "%s | nearly" },
  description: "Build and buy yield baskets across lending, liquidity and staking on NEAR, from your own account contract.",
  applicationName: "nearly",
};

export const viewport: Viewport = { themeColor: "#101010", colorScheme: "dark" };

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`dark ${inter.variable} ${mono.variable} h-full`}>
      <body className="min-h-full font-sans bg-background text-foreground antialiased">
        <MotionRoot>
          <SessionProvider>{children}</SessionProvider>
        </MotionRoot>
      </body>
    </html>
  );
}
