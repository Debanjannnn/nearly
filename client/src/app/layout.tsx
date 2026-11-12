import type { Metadata, Viewport } from "next";
import { Geist_Mono, Inter } from "next/font/google";
import "./globals.css";
import { SessionProvider } from "@/lib/session";
import { Toaster } from "sonner";
import { MotionRoot } from "@/components/motion";
import { DESCRIPTION, OPEN_GRAPH, SITE_URL, TITLE } from "@/lib/site";

const inter = Inter({ variable: "--font-inter", subsets: ["latin"] });
const mono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"] });

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: { default: TITLE, template: "%s | nearly" },
  description: DESCRIPTION,
  applicationName: "nearly",
  keywords: ["nearly", "NEAR", "yield basket", "DeFi", "self custodial", "Rhea", "LiNEAR", "NEAR Intents", "lending", "staking", "DCA"],
  openGraph: OPEN_GRAPH,
  twitter: { card: "summary", title: TITLE, description: DESCRIPTION },
  robots: { index: true, follow: true, googleBot: { index: true, follow: true, "max-image-preview": "large" } },
};

export const viewport: Viewport = { themeColor: "#101010", colorScheme: "dark" };

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="en"
      className={`dark ${inter.variable} ${mono.variable} h-full`}
    >
      <body className={`min-h-full font-sans bg-background text-foreground antialiased`}>
        <MotionRoot>
          <SessionProvider>{children}</SessionProvider>
          <Toaster theme="dark" position="bottom-right" />
        </MotionRoot>
      </body>
    </html>
  );
}
