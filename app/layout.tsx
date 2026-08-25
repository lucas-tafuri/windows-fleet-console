import type { Metadata } from "next";
import { IBM_Plex_Mono, IBM_Plex_Sans } from "next/font/google";
import "./globals.css";
import { Shell } from "@/components/shell";
import { PinGate } from "@/components/pin-gate";

const ibmSans = IBM_Plex_Sans({
  variable: "--font-ibm-sans",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

const ibmMono = IBM_Plex_Mono({
  variable: "--font-ibm-mono",
  subsets: ["latin"],
  weight: ["400", "500"],
});

export const metadata: Metadata = {
  title: "Fleet Console",
  description: "Remote control for your Windows machines",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="en"
      className={`dark ${ibmSans.variable} ${ibmMono.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <PinGate>
          <Shell>{children}</Shell>
        </PinGate>
      </body>
    </html>
  );
}
