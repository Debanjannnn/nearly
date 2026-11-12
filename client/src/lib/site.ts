import type { Metadata } from "next";

// public origin for metadata, robots and sitemap; set per environment
export const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? "http://localhost:3000";

export const TITLE = "nearly: Self Custodial Yield Baskets on NEAR";
export const DESCRIPTION =
  "Build and buy yield baskets across Rhea Lending, LiNEAR staking and Rhea DEX, all from your own NEAR account. Fund from any chain with NEAR Intents and automate recurring buys and rebalances.";

export const OG_IMAGE = { url: "/og.png", width: 1600, height: 896, alt: "Nearly: The Yield Bearing Basket, live on NEAR" };

// link preview card; a page setting its own openGraph replaces the parent's, so spread this
export const OPEN_GRAPH: NonNullable<Metadata["openGraph"]> = {
  type: "website",
  siteName: "nearly",
  title: TITLE,
  description: DESCRIPTION,
  locale: "en_US",
  images: [OG_IMAGE],
};
