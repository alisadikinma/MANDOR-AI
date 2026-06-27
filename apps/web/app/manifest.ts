import type { MetadataRoute } from "next";

// PWA manifest — Next.js App Router serves this at /manifest.webmanifest and
// auto-links it in <head>. ponytail: native MetadataRoute, no next-pwa dep.
// theme/background mirror the dark themeColor in app/layout.tsx (#05070b).
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Multica",
    short_name: "Multica",
    description:
      "Project management for human + agent teams. Monitor inbox, issues, and runs on the go.",
    start_url: "/",
    display: "standalone",
    background_color: "#05070b",
    theme_color: "#05070b",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
      // ponytail: reuse the square icon as maskable. Add a padded safe-zone
      // variant if Android launchers crop the logo.
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
