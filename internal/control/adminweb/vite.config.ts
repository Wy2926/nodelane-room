import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "./",
  build: {
    outDir: "dist",
    sourcemap: false,
    rollupOptions: {
      output: {
        entryFileNames: "assets/app.js",
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/style[extname]",
      },
    },
  },
  server: {
    proxy: {
      "/v2": { target: "http://127.0.0.1:8080" },
      "/install": { target: "http://127.0.0.1:8080" },
    },
  },
});
