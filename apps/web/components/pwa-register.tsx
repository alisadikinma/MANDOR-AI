"use client";

import { useEffect } from "react";

// Registers the PWA service worker (/sw.js). Production-only: a SW in dev
// caches the shell and fights HMR. Renders nothing.
export function PwaRegister() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") return;
    if (!("serviceWorker" in navigator)) return;
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  }, []);
  return null;
}
